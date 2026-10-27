package routing

import (
	"errors"
	"io/fs"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/frontmatter"
)

// FuzzRoute checks what must hold for any page: Route does not panic; a
// path that tries to leave the output directory is refused; and on success
// the output path is a clean path inside the output directory, and the URL
// path is a valid, encoded URL path that reaches exactly that file.
func FuzzRoute(f *testing.F) {
	seeds := []struct {
		contentPath, permalink, title string
		isDirectoryRouting, hasDate   bool
		unixSeconds                   int64
	}{
		{"about.md", "path", "", false, false, 0},
		{"blog/index.md", "path", "", true, false, 0},
		{"Index.HTML", "path", "", true, false, 0},
		{"città/my page.md", "path", "", false, false, 0},
		{"post.md", "slug", "Città di mare", true, false, 0},
		{"post.md", "slug", "../../etc/passwd", false, false, 0},
		{"post.md", "slug", "!!!", false, false, 0},
		{"post.md", "slug", "Index", true, false, 0},
		{"post.md", "date", "Hello", false, true, 1714559400},
		{"post.md", "date", "Hello", true, true, -62135596800},
		{"post.md", "date", "Hello", false, false, 0},
		{"x.md", "/", "", false, false, 0},
		{"x.md", "/about-us/", "", true, false, 0},
		{"x.md", "/feed.xml", "", false, false, 0},
		{"x.md", "//a/./b/.", "", false, false, 0},
		{"x.md", "/../secret", "", false, false, 0},
		{"x.md", "/a/../../b/", "", false, false, 0},
		{"x.md", `/..\..\x`, "", false, false, 0},
		{"x.md", "/C:/x", "", false, false, 0},
		{"x.md", "/a?b#c", "", false, false, 0},
		{"x.md", "/index.html", "", false, false, 0},
		{"../x.md", "path", "", false, false, 0},
		{"/etc/passwd.md", "path", "", false, false, 0},
		{"a/../../x.md", "path", "", false, false, 0},
		{"a\\..\\..\\x.md", "path", "", false, false, 0},
		{".md", "path", "", false, false, 0},
		{"100%?#.md", "path", "", false, false, 0},
	}
	for _, seed := range seeds {
		f.Add(seed.contentPath, seed.permalink, seed.title, seed.isDirectoryRouting, seed.hasDate, seed.unixSeconds)
	}
	f.Fuzz(func(t *testing.T, contentPath, permalink, title string, isDirectoryRouting, hasDate bool, unixSeconds int64) {
		routing := config.RoutingFile
		if isDirectoryRouting {
			routing = config.RoutingDirectory
		}
		page := Page{
			SourcePath:  "src/" + contentPath,
			ContentPath: contentPath,
			Permalink:   permalink,
			Title:       title,
			Date:        time.Unix(unixSeconds, 0).UTC(),
			HasDate:     hasDate,
		}
		location, err := Route(page, routing)
		if err != nil {
			checkError(t, page, err)
			return
		}
		if frontmatter.CheckPermalink(permalink) != nil {
			t.Fatalf("permalink %q is refused by front matter but routed to %+v", permalink, location)
		}
		if !fs.ValidPath(contentPath) {
			t.Fatalf("content path %q is not valid but routed to %+v", contentPath, location)
		}
		checkOutputPath(t, page, location.OutputPath)
		checkURLPath(t, location)
	})
}

// checkError fails t unless err is a *diag.Error naming the page's source
// file, or one of the errors for a content path that scan never produces.
func checkError(t *testing.T, page Page, err error) {
	t.Helper()
	if strings.HasPrefix(err.Error(), "routing "+page.SourcePath+": content path ") {
		return
	}
	var diagErr *diag.Error
	if !errors.As(err, &diagErr) || diagErr.File != page.SourcePath || diagErr.Line != 0 {
		t.Fatalf("error %q is not a diag error naming %q without a position", err, page.SourcePath)
	}
}

func checkOutputPath(t *testing.T, page Page, outputPath string) {
	t.Helper()
	if !fs.ValidPath(outputPath) || outputPath == "." || path.Clean(outputPath) != outputPath {
		t.Fatalf("output path %q is not a clean relative path", outputPath)
	}
	if !filepath.IsLocal(filepath.FromSlash(outputPath)) {
		t.Fatalf("output path %q leaves the output directory", outputPath)
	}
	if !frontmatter.IsCustomPermalink(page.Permalink) && !strings.HasSuffix(outputPath, htmlExtension) {
		t.Fatalf("output path %q of permalink %q does not end in .html", outputPath, page.Permalink)
	}
}

// checkURLPath fails t unless the URL path is an encoded path, with no
// query or fragment, that reaches the output file: its own path, or its
// directory's for an index.html.
func checkURLPath(t *testing.T, location Location) {
	t.Helper()
	parsed, err := url.Parse(location.URLPath)
	if err != nil {
		t.Fatalf("URL path %q does not parse: %v", location.URLPath, err)
	}
	if parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.EscapedPath() != location.URLPath {
		t.Fatalf("URL path %q is not only a path: %#v", location.URLPath, parsed)
	}
	decoded := parsed.Path
	if !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") {
		t.Fatalf("URL path %q does not begin with a single \"/\"", location.URLPath)
	}
	want := strings.TrimPrefix(decoded, "/")
	if strings.HasSuffix(decoded, "/") {
		want += indexFileName
	} else if path.Base(want) == indexFileName {
		t.Fatalf("URL path %q names index.html instead of its directory", location.URLPath)
	}
	if want != location.OutputPath {
		t.Fatalf("URL path %q reaches %q, not the output path %q", location.URLPath, want, location.OutputPath)
	}
}
