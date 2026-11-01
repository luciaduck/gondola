// Package sitemap writes sitemap.xml, the list of the site's pages for
// search engines, and reads robots.txt to leave out the pages that crawlers
// may not visit (SPEC §5.12). The robots.txt matcher is written by hand,
// following RFC 9309.
package sitemap

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// FileName is the name of the sitemap in the output directory. It sits at
// the root, where it covers every URL of the site.
const FileName = "sitemap.xml"

// MaxURLs is the most URLs that one sitemap may hold, by the sitemap
// protocol (sitemaps.org). A larger site would need a sitemap index, which
// Gondola does not write.
const MaxURLs = 50_000

// lastmodLayout writes the date of a page as a W3C date without the time
// (M10, Q5): the date as written, in its own offset or in UTC, as for
// permalink: date.
const lastmodLayout = "2006-01-02"

// Page is what the sitemap needs to know about one page.
type Page struct {
	// OutputPath is the page's file, relative to the output directory and
	// written with "/". Only HTML files are listed.
	OutputPath string
	// URLPath is the page's URL path from routing: it begins with "/" and
	// is percent-encoded.
	URLPath string
	// Listed is the page's merged sitemap variable: false leaves it out,
	// and true never overrides robots.txt.
	Listed bool
	// Date is the page's date, written as lastmod; it is meaningful only
	// when HasDate is true.
	Date    time.Time
	HasDate bool
}

// Build returns the text of sitemap.xml and the number of URLs in it.
// canonicalURL has no trailing slash, as config gives it. A page is listed
// when its output is an HTML file (.html or .htm, in any letter case), its
// sitemap variable is not false, and rules allow its URL path. The URLs are
// sorted by URL path, byte by byte.
//
// More than MaxURLs listed pages is an error. With no listed page the
// sitemap is written anyway, with an empty urlset, so that a Sitemap line
// in robots.txt never leads to a missing file (M10, Q5).
func Build(pages []Page, canonicalURL string, rules Rules) ([]byte, int, error) {
	listed := make([]Page, 0, len(pages))
	for _, page := range pages {
		if page.Listed && isHTMLOutput(page.OutputPath) && rules.Allows(page.URLPath) {
			listed = append(listed, page)
		}
	}
	if len(listed) > MaxURLs {
		return nil, 0, fmt.Errorf("the sitemap would list %s pages, more than the %s a sitemap may hold; please exclude some with sitemap: false or turn config.sitemap off",
			withThousandsSeparators(len(listed)), withThousandsSeparators(MaxURLs))
	}
	// Routing gives every page its own URL path, so the order is complete;
	// the sort is stable anyway, to stay deterministic whatever the input.
	sort.SliceStable(listed, func(i, j int) bool { return listed[i].URLPath < listed[j].URLPath })

	var xml strings.Builder
	xml.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	xml.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, page := range listed {
		xml.WriteString("  <url>\n")
		fmt.Fprintf(&xml, "    <loc>%s</loc>\n", escapeXML(canonicalURL+page.URLPath))
		if page.HasDate {
			fmt.Fprintf(&xml, "    <lastmod>%s</lastmod>\n", page.Date.Format(lastmodLayout))
		}
		xml.WriteString("  </url>\n")
	}
	xml.WriteString("</urlset>\n")
	return []byte(xml.String()), len(listed), nil
}

// isHTMLOutput reports whether outputPath is an HTML file, by its
// extension. A page with a custom permalink may write another kind of file,
// such as /feed.xml, or a file without an extension, such as /about, which
// Gondola cannot know will be served as HTML (M10, Q1).
func isHTMLOutput(outputPath string) bool {
	switch strings.ToLower(path.Ext(outputPath)) {
	case ".html", ".htm":
		return true
	}
	return false
}

// xmlEscaper replaces the five characters that the sitemap protocol asks to
// write as entities. Routing has already percent-encoded everything else
// that a URL path cannot hold, but "&" and "'" are valid in a URL path, and
// the canonical URL is kept as written. One pass, so the "&" that it
// writes is never escaped again.
var xmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"'", "&apos;",
	`"`, "&quot;",
	">", "&gt;",
	"<", "&lt;",
)

func escapeXML(text string) string {
	return xmlEscaper.Replace(text)
}

// withThousandsSeparators writes a non-negative number with a "," every
// three digits, as in 50,000.
func withThousandsSeparators(number int) string {
	digits := strconv.Itoa(number)
	var grouped strings.Builder
	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}
	return grouped.String()
}
