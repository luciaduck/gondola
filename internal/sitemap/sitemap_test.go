package sitemap

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// goldenSitemapPath is the sitemap that TestBuildGolden must write. It is
// written by hand, not generated, so that it states what the output should
// be.
const goldenSitemapPath = "testdata/sitemap.xml"

const testCanonicalURL = "https://example.com"

// date returns the time written as text in RFC 3339, or the date alone.
func date(t *testing.T, text string) time.Time {
	t.Helper()
	layout := time.RFC3339
	if len(text) == len(time.DateOnly) {
		layout = time.DateOnly
	}
	parsed, err := time.Parse(layout, text)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func mustParseRobots(t *testing.T, text string) Rules {
	t.Helper()
	rules, err := ParseRobots("robots.txt", []byte(text), "*")
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestBuildGolden(t *testing.T) {
	pages := []Page{
		// Given out of order: the sitemap sorts them by URL path.
		{OutputPath: "fish&chips's.html", URLPath: "/fish&chips's.html", Listed: true},
		{OutputPath: "città.htm", URLPath: "/citt%C3%A0.htm", Listed: true, Date: date(t, "2024-05-01T23:30:00-05:00"), HasDate: true},
		{OutputPath: "index.html", URLPath: "/", Listed: true},
		{OutputPath: "about-us/index.html", URLPath: "/about-us/", Listed: true},
		{OutputPath: "2024/05/01/fish-and-chips.html", URLPath: "/2024/05/01/fish-and-chips.html", Listed: true, Date: date(t, "2024-05-01"), HasDate: true},
		// Left out: not HTML, sitemap: false, and disallowed by robots.txt
		// although sitemap is true.
		{OutputPath: "feed.xml", URLPath: "/feed.xml", Listed: true},
		{OutputPath: "about", URLPath: "/about", Listed: true},
		{OutputPath: "secret.html", URLPath: "/secret.html", Listed: false},
		{OutputPath: "private/page.html", URLPath: "/private/page.html", Listed: true},
	}
	rules := mustParseRobots(t, "User-agent: *\nDisallow: /private/\n")
	got, count, err := Build(pages, testCanonicalURL, rules)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(goldenSitemapPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("sitemap differs:\n got:\n%s\nwant:\n%s", got, want)
	}
	if count != 5 {
		t.Errorf("count %d, want 5", count)
	}
}

func TestBuildEmpty(t *testing.T) {
	want := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n" +
		"</urlset>\n"
	tests := []struct {
		name  string
		pages []Page
		rules Rules
	}{
		{"no pages", nil, Rules{}},
		{"every page excluded", []Page{{OutputPath: "a.html", URLPath: "/a.html", Listed: false}}, Rules{}},
		{"every page disallowed", []Page{{OutputPath: "a.html", URLPath: "/a.html", Listed: true}}, mustParseRobots(t, "User-agent: *\nDisallow: /\n")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, count, err := Build(test.pages, testCanonicalURL, test.rules)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want || count != 0 {
				t.Errorf("got %d URLs:\n%s\nwant 0:\n%s", count, got, want)
			}
		})
	}
}

// locs returns the text of every <loc> in a sitemap, in order.
func locs(sitemap []byte) []string {
	var found []string
	for _, line := range strings.Split(string(sitemap), "\n") {
		line = strings.TrimSpace(line)
		if loc, ok := strings.CutPrefix(line, "<loc>"); ok {
			found = append(found, strings.TrimSuffix(loc, "</loc>"))
		}
	}
	return found
}

func TestBuildListsOnly(t *testing.T) {
	tests := []struct {
		name     string
		page     Page
		robots   string
		wantLocs []string
	}{
		{"an HTML page", Page{OutputPath: "a.html", URLPath: "/a.html", Listed: true}, "", []string{"https://example.com/a.html"}},
		{"an .htm page", Page{OutputPath: "a.htm", URLPath: "/a.htm", Listed: true}, "", []string{"https://example.com/a.htm"}},
		{"an extension in capitals", Page{OutputPath: "A.HTML", URLPath: "/A.HTML", Listed: true}, "", []string{"https://example.com/A.HTML"}},
		{"an index page", Page{OutputPath: "blog/index.html", URLPath: "/blog/", Listed: true}, "", []string{"https://example.com/blog/"}},
		{"an XML page", Page{OutputPath: "feed.xml", URLPath: "/feed.xml", Listed: true}, "", nil},
		{"a page without extension", Page{OutputPath: "about", URLPath: "/about", Listed: true}, "", nil},
		{"a name that only ends in html", Page{OutputPath: "xhtml", URLPath: "/xhtml", Listed: true}, "", nil},
		{"an HTML directory", Page{OutputPath: "a.html/b", URLPath: "/a.html/b", Listed: true}, "", nil},
		{"sitemap: false", Page{OutputPath: "a.html", URLPath: "/a.html", Listed: false}, "", nil},
		{"sitemap: true does not override robots.txt", Page{OutputPath: "a.html", URLPath: "/a.html", Listed: true}, "User-agent: *\nDisallow: /a\n", nil},
		{"robots.txt is matched against the URL path, not the file", Page{OutputPath: "blog/index.html", URLPath: "/blog/", Listed: true}, "User-agent: *\nDisallow: /blog/index.html\n", []string{"https://example.com/blog/"}},
		{"robots.txt allows the page", Page{OutputPath: "a.html", URLPath: "/a.html", Listed: true}, "User-agent: *\nDisallow: /b\n", []string{"https://example.com/a.html"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, count, err := Build([]Page{test.page}, testCanonicalURL, mustParseRobots(t, test.robots))
			if err != nil {
				t.Fatal(err)
			}
			gotLocs := locs(got)
			if fmt.Sprint(gotLocs) != fmt.Sprint(test.wantLocs) || count != len(test.wantLocs) {
				t.Errorf("listed %q (count %d), want %q", gotLocs, count, test.wantLocs)
			}
		})
	}
}

func TestBuildOrder(t *testing.T) {
	// Byte order of the URL paths: "/" before letters, capitals before
	// small letters, and a percent-encoding by its "%".
	urlPaths := []string{"/b.html", "/a/", "/", "/B.html", "/%C3%A0.html", "/a-b.html", "/a.html"}
	pages := make([]Page, 0, len(urlPaths))
	for _, urlPath := range urlPaths {
		pages = append(pages, Page{OutputPath: strings.TrimPrefix(urlPath, "/") + "index.html", URLPath: urlPath, Listed: true})
	}
	got, _, err := Build(pages, testCanonicalURL, Rules{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/", "/%C3%A0.html", "/B.html", "/a-b.html", "/a.html", "/a/", "/b.html"}
	for i := range want {
		want[i] = testCanonicalURL + want[i]
	}
	if gotLocs := locs(got); fmt.Sprint(gotLocs) != fmt.Sprint(want) {
		t.Errorf("order %q, want %q", gotLocs, want)
	}
}

func TestBuildEscaping(t *testing.T) {
	tests := []struct {
		canonicalURL string
		urlPath      string
		wantLoc      string
	}{
		{"https://example.com", "/fish&chips.html", "https://example.com/fish&amp;chips.html"},
		{"https://example.com", "/it's.html", "https://example.com/it&apos;s.html"},
		{"https://example.com/a&b", "/c.html", "https://example.com/a&amp;b/c.html"},
		{"https://example.com", `/"<>.html`, "https://example.com/&quot;&lt;&gt;.html"},
		{"https://example.com", "/&amp;.html", "https://example.com/&amp;amp;.html"},
	}
	for _, test := range tests {
		got, _, err := Build([]Page{{OutputPath: "x.html", URLPath: test.urlPath, Listed: true}}, test.canonicalURL, Rules{})
		if err != nil {
			t.Fatal(err)
		}
		if gotLocs := locs(got); len(gotLocs) != 1 || gotLocs[0] != test.wantLoc {
			t.Errorf("%q + %q: loc %q, want %q", test.canonicalURL, test.urlPath, gotLocs, test.wantLoc)
		}
	}
}

func TestBuildLastmod(t *testing.T) {
	tests := []struct {
		name string
		date string
		want string
	}{
		{"a date alone", "2024-05-01", "2024-05-01"},
		{"a time in UTC", "2024-05-01T23:30:00Z", "2024-05-01"},
		{"the date as written in its own offset", "2024-05-01T23:30:00-05:00", "2024-05-01"},
		{"a positive offset", "2024-05-02T00:30:00+02:00", "2024-05-02"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, err := Build([]Page{{OutputPath: "a.html", URLPath: "/a.html", Listed: true, Date: date(t, test.date), HasDate: true}}, testCanonicalURL, Rules{})
			if err != nil {
				t.Fatal(err)
			}
			want := "<lastmod>" + test.want + "</lastmod>"
			if !strings.Contains(string(got), want) {
				t.Errorf("no %s in:\n%s", want, got)
			}
		})
	}
}

func TestBuildLimit(t *testing.T) {
	manyPages := func(count int) []Page {
		pages := make([]Page, count)
		for i := range pages {
			pages[i] = Page{OutputPath: fmt.Sprintf("p%d.html", i), URLPath: fmt.Sprintf("/p%d.html", i), Listed: true}
		}
		return pages
	}
	if _, count, err := Build(manyPages(MaxURLs), testCanonicalURL, Rules{}); err != nil || count != MaxURLs {
		t.Errorf("%d pages: count %d, error %v", MaxURLs, count, err)
	}
	_, _, err := Build(manyPages(MaxURLs+1), testCanonicalURL, Rules{})
	want := "the sitemap would list 50,001 pages, more than the 50,000 a sitemap may hold; please exclude some with sitemap: false or turn config.sitemap off"
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %q", err, want)
	}
	// Only the listed pages count.
	pages := manyPages(MaxURLs + 1)
	pages[0].Listed = false
	if _, count, err := Build(pages, testCanonicalURL, Rules{}); err != nil || count != MaxURLs {
		t.Errorf("%d listed pages of %d: count %d, error %v", MaxURLs, len(pages), count, err)
	}
}

func TestWithThousandsSeparators(t *testing.T) {
	tests := []struct {
		number int
		want   string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1,000"},
		{50000, "50,000"},
		{50001, "50,001"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
	}
	for _, test := range tests {
		if got := withThousandsSeparators(test.number); got != test.want {
			t.Errorf("withThousandsSeparators(%d) = %q, want %q", test.number, got, test.want)
		}
	}
}
