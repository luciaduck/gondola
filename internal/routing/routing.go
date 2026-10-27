// Package routing works out where each page is written in the output and
// the URL that reaches it, from the page's path in the content directory,
// its permalink and the setting output.routing (SPEC §5.7).
//
// Two pages that would write the same file are not caught here: the build
// claims every output path on a scan.OutputPaths, together with the Sass
// and passthrough outputs, so that one check covers all of them.
package routing

import (
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosimple/slug"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/frontmatter"
)

const (
	// indexStem is the file name, without extension, of the page that a
	// directory's URL reaches: blog/index.md is the page at /blog/. It is
	// matched in small letters only, because a web server on Linux answers
	// /blog/ with index.html, never with Index.html.
	indexStem = "index"
	// indexFileName is the output file that a URL ending in "/" reaches.
	indexFileName = indexStem + htmlExtension
	// htmlExtension ends every output path that routing makes itself;
	// only a custom permalink such as /feed.xml can name another file.
	htmlExtension = ".html"
	// dateDirectoriesLayout is the Go time layout of the directories that
	// the permalink "date" puts in front of the slug: 2024/05/01.
	dateDirectoriesLayout = "2006/01/02"
)

// Page is what routing needs to know about one page. The build fills it
// from scan.Page and the page's merged front matter (frontmatter.Merged).
type Page struct {
	// SourcePath is relative to the project root, written with "/". It
	// names the page in messages.
	SourcePath string
	// ContentPath is relative to the content directory, written with "/",
	// with the letters as on disk, such as "blog/Post.md".
	ContentPath string
	// Permalink is the page's merged permalink: "path", "slug", "date", or
	// a custom permalink such as "/about-us/".
	Permalink string
	// Title is the page's merged title, used by the permalinks "slug" and
	// "date". It is "" when no level sets it.
	Title string
	// Date is the page's date, used by the permalink "date". It is
	// meaningful only when HasDate is true.
	Date    time.Time
	HasDate bool
}

// Location is where a page is written and how it is reached.
type Location struct {
	// OutputPath is the page's output file, relative to the output
	// directory and written with "/", such as "blog/post/index.html".
	OutputPath string
	// URLPath is the path of the page's URL, percent-encoded, such as
	// "/blog/post/" or "/citt%C3%A0.html". It always begins with "/". A
	// page's full URL is the canonical URL followed by URLPath.
	URLPath string
}

// Route works out the location of page, with routing as the shape of the
// paths that are not custom permalinks (output.routing):
//
//   - "path" keeps the content path; "slug" replaces the file name with the
//     slug of the title, keeping the directories; "date" makes
//     YYYY/MM/DD/<slug> from the date and the title.
//   - With routing "file", about.md becomes about.html; with "directory",
//     about/index.html.
//   - An index page (index.md, blog/index.html, …) keeps its path under
//     every permalink and every routing: it is the page of its directory.
//   - A custom permalink ignores routing: one that ends in "/" becomes an
//     index.html in that directory, any other is the output file itself.
//
// An output file called index.html is reached by its directory's URL
// ("/blog/"); any other by its own path ("/blog/post.html").
//
// Errors about the page are *diag.Error values naming its source file,
// without a position: the values may come from a layout or from the
// defaults, and front matter has already checked what it could at its own
// positions.
func Route(page Page, routing config.Routing) (Location, error) {
	if !isPagePath(page.ContentPath) {
		return Location{}, fmt.Errorf("routing %s: content path %q is not the path of a file inside the content directory", page.SourcePath, page.ContentPath)
	}
	// Front matter checks permalinks when it reads them; checking again
	// here keeps every caller from making a page escape the output.
	if err := frontmatter.CheckPermalink(page.Permalink); err != nil {
		return Location{}, &diag.Error{File: page.SourcePath, Err: err}
	}
	outputPath, err := outputPathOf(page, routing)
	if err != nil {
		return Location{}, err
	}
	// The rules above cannot produce such a path; this is the last line of
	// defense against writing outside the output directory.
	if !fs.ValidPath(outputPath) || !filepath.IsLocal(filepath.FromSlash(outputPath)) {
		return Location{}, fmt.Errorf("routing %s: output path %q is not inside the output directory", page.SourcePath, outputPath)
	}
	return Location{OutputPath: outputPath, URLPath: urlPathOf(outputPath)}, nil
}

// isPagePath reports whether contentPath can be the content path of a page:
// a clean relative path with "/" (as package scan writes it) whose file name
// without the extension is a name of its own. path.Ext takes everything from
// the last dot, so "..." would leave "..", and "..md" would leave ".": both
// would name a directory instead of a file. Package scan skips such names,
// because they begin with ".".
func isPagePath(contentPath string) bool {
	if !fs.ValidPath(contentPath) || contentPath == "." {
		return false
	}
	_, stem := splitContentPath(contentPath)
	return stem != "" && stem != "." && stem != ".."
}

// splitContentPath returns the directory of contentPath ("." at the top of
// the content directory) and its file name without the extension.
func splitContentPath(contentPath string) (dir, stem string) {
	dir, fileName := path.Split(contentPath)
	stem = strings.TrimSuffix(fileName, path.Ext(fileName))
	return path.Clean(dir), stem
}

func outputPathOf(page Page, routing config.Routing) (string, error) {
	if frontmatter.IsCustomPermalink(page.Permalink) {
		return customOutputPath(page.Permalink), nil
	}
	dir, stem := splitContentPath(page.ContentPath)
	if stem == indexStem {
		return path.Join(dir, indexFileName), nil
	}
	var name string
	switch config.Permalink(page.Permalink) {
	case config.PermalinkPath:
		name = path.Join(dir, stem)
	case config.PermalinkSlug:
		titleSlug, err := slugOfTitle(page)
		if err != nil {
			return "", err
		}
		name = path.Join(dir, titleSlug)
	case config.PermalinkDate:
		if !page.HasDate {
			return "", pageError(page, "permalink %q needs a date, but the page has none; please set date or choose another permalink", page.Permalink)
		}
		titleSlug, err := slugOfTitle(page)
		if err != nil {
			return "", err
		}
		// The date's own zone: the offset written with it, or UTC.
		name = path.Join(page.Date.Format(dateDirectoriesLayout), titleSlug)
	}
	return shapedOutputPath(name, routing)
}

// shapedOutputPath turns name, an output path without extension, into the
// output file that routing asks for.
func shapedOutputPath(name string, routing config.Routing) (string, error) {
	// A title that slugs to "index" makes an index page too.
	if path.Base(name) == indexStem {
		return path.Join(path.Dir(name), indexFileName), nil
	}
	switch routing {
	case config.RoutingFile:
		return name + htmlExtension, nil
	case config.RoutingDirectory:
		return path.Join(name, indexFileName), nil
	}
	return "", fmt.Errorf("unknown routing %q", routing)
}

// customOutputPath turns a custom permalink, already checked, into an
// output path. "./" and doubled "/" are cleaned, so "//a/./b/" is a/b/.
func customOutputPath(permalink string) string {
	// Cleaning removes a trailing "/" and a final ".", which both name a
	// directory, so the question is asked before.
	namesDirectory := strings.HasSuffix(permalink, "/") || path.Base(permalink) == "."
	cleaned := strings.TrimPrefix(path.Clean(permalink), "/")
	if namesDirectory || cleaned == "" {
		return path.Join(cleaned, indexFileName)
	}
	return cleaned
}

// slugOfTitle returns the slug of the page's title, made by gosimple/slug
// as heading IDs are: "Città di mare" becomes "citta-di-mare".
func slugOfTitle(page Page) (string, error) {
	if page.Title == "" {
		return "", pageError(page, "permalink %q needs a title, but the page has none; please set title or choose another permalink", page.Permalink)
	}
	titleSlug := slug.Make(page.Title)
	if titleSlug == "" {
		return "", pageError(page, "the title %q has no letters or digits, so permalink %q cannot make a file name from it; please change the title or choose another permalink", page.Title, page.Permalink)
	}
	return titleSlug, nil
}

// urlPathOf returns the percent-encoded URL path that reaches outputPath.
func urlPathOf(outputPath string) string {
	urlPath := "/" + outputPath
	if path.Base(outputPath) == indexFileName {
		urlPath = strings.TrimSuffix(urlPath, indexFileName)
	}
	// EscapedPath percent-encodes what a URL path cannot hold as it is,
	// such as spaces, "%", "?", "#" and letters outside ASCII, so the URL
	// is valid in an href and in the sitemap.
	return (&url.URL{Path: urlPath}).EscapedPath()
}

func pageError(page Page, format string, args ...any) error {
	return &diag.Error{File: page.SourcePath, Msg: fmt.Sprintf(format, args...)}
}
