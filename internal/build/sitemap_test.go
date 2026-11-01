package build

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// withSitemap is minimalConfig with the sitemap on.
const withSitemap = minimalConfig + "  sitemap: true\n"

// sitemapOf returns a sitemap.xml that lists urls, without lastmod.
func sitemapOf(urls ...string) string {
	text := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n" +
		"<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n"
	for _, url := range urls {
		text += "  <url>\n    <loc>" + url + "</loc>\n  </url>\n"
	}
	return text + "</urlset>\n"
}

func TestBuildSitemap(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantSitemap string
		// wantLine is the sitemap line, or "" when the log must not have
		// one.
		wantLine string
	}{
		{
			name:     "the sitemap is off by default",
			files:    map[string]string{"index.html": "home"},
			wantLine: "",
		},
		{
			name:        "no robots.txt",
			files:       map[string]string{"gondola.yaml": withSitemap, "index.html": "home", "b.html": "b"},
			wantSitemap: sitemapOf("https://example.com/", "https://example.com/b.html"),
			wantLine:    "(sitemap) 2 pages were listed (there is no robots.txt)",
		},
		{
			name: "a passthrough robots.txt",
			files: map[string]string{
				"gondola.yaml": withSitemap + "output:\n  passthrough: [robots.txt]\n",
				"robots.txt":   "User-agent: *\nDisallow: /private/\n",
				"index.html":   "home", "private/a.html": "a",
			},
			wantSitemap: sitemapOf("https://example.com/"),
			wantLine:    "(sitemap) 1 page was allowed by robots.txt",
		},
		{
			name: "a robots.txt made by a page",
			files: map[string]string{
				"gondola.yaml": withSitemap,
				"robots.html":  "---\nlayout: nil\npermalink: /robots.txt\n---\nUser-agent: *\nDisallow: /b\n",
				"index.html":   "home", "b.html": "b",
			},
			wantSitemap: sitemapOf("https://example.com/"),
			wantLine:    "(sitemap) 1 page was allowed by robots.txt",
		},
		{
			name: "config.robots-agent chooses the group",
			files: map[string]string{
				"gondola.yaml": withSitemap + "  robots-agent: Gondolabot\noutput:\n  passthrough: [robots.txt]\n",
				"robots.txt":   "User-agent: *\nDisallow: /\n\nUser-agent: gondolabot\nDisallow: /b\n",
				"index.html":   "home", "b.html": "b",
			},
			wantSitemap: sitemapOf("https://example.com/"),
			wantLine:    "(sitemap) 1 page was allowed by robots.txt",
		},
		{
			name: "sitemap: true does not override robots.txt",
			files: map[string]string{
				"gondola.yaml": withSitemap + "output:\n  passthrough: [robots.txt]\n",
				"robots.txt":   "User-agent: *\nDisallow: /b\n",
				"index.html":   "home", "b.html": "---\nsitemap: true\n---\nb",
			},
			wantSitemap: sitemapOf("https://example.com/"),
			wantLine:    "(sitemap) 1 page was allowed by robots.txt",
		},
		{
			name: "sitemap: false in a page, a layout and the defaults",
			files: map[string]string{
				"gondola.yaml":                    withSitemap + "default:\n  sitemap: false\n",
				"index.html":                      "---\nsitemap: true\n---\nhome",
				"b.html":                          "b",
				"c.html":                          "---\nlayout: hidden\nsitemap: true\n---\nc",
				"d.html":                          "---\nlayout: hidden\n---\nd",
				"_components/includes/hidden.jet": "---\nsitemap: false\n---\n{{ copy|raw }}",
			},
			wantSitemap: sitemapOf("https://example.com/", "https://example.com/c.html"),
			wantLine:    "(sitemap) 2 pages were listed (there is no robots.txt)",
		},
		{
			name: "only HTML pages, never passthrough files",
			files: map[string]string{
				"gondola.yaml":      withSitemap + "output:\n  passthrough: [static/]\n",
				"index.html":        "home",
				"feed.html":         "---\nlayout: nil\npermalink: /feed.xml\n---\n<feed/>",
				"about.html":        "---\nlayout: nil\npermalink: /about\n---\nabout",
				"old.htm":           "---\npermalink: /old.htm\n---\nold",
				"static/guide.html": "guide",
			},
			wantSitemap: sitemapOf("https://example.com/", "https://example.com/old.htm"),
			wantLine:    "(sitemap) 2 pages were listed (there is no robots.txt)",
		},
		{
			name:        "a site without pages",
			files:       map[string]string{"gondola.yaml": withSitemap},
			wantSitemap: sitemapOf(),
			wantLine:    "(sitemap) 0 pages were listed (there is no robots.txt)",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, test.files)
			result := runBuild(t, projectDir)
			if result.failure != nil {
				t.Fatalf("build failed: %q", result.errorTexts())
			}
			output := readTree(t, filepath.Join(projectDir, "_site"))
			gotSitemap, written := output["sitemap.xml"]
			if test.wantSitemap == "" {
				if written || strings.Contains(result.stdout, "(sitemap)") {
					t.Errorf("a sitemap was written:\n%s\n%s", gotSitemap, result.stdout)
				}
				return
			}
			if gotSitemap != test.wantSitemap {
				t.Errorf("sitemap.xml:\n got:\n%s\nwant:\n%s", gotSitemap, test.wantSitemap)
			}
			// The line comes after every page and before the output is
			// published (SPEC §5.14).
			lines := strings.Split(strings.TrimSuffix(result.stdout, "\n"), "\n")
			if len(lines) < 3 || lines[len(lines)-3] != test.wantLine || !strings.HasPrefix(lines[len(lines)-2], `\gondola/ Shipping`) {
				t.Errorf("log:\n%s\nwant the line %q before Shipping", result.stdout, test.wantLine)
			}
		})
	}
}

func TestBuildSitemapErrors(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantErrors []string
	}{
		{
			name: "a page writes sitemap.xml",
			files: map[string]string{
				"gondola.yaml": withSitemap,
				"map.html":     "---\npermalink: /sitemap.xml\n---\n",
				"other.html":   "---\npermalink: /a\\b\n---\n",
			},
			// A collected error of stage 3, reported with the others.
			wantErrors: []string{
				`page "map.html" writes "sitemap.xml", where Gondola writes the sitemap because config.sitemap is true; please rename or remove it, or turn config.sitemap off`,
				`other.html:2:12: permalink "/a\\b" contains "\\"; please separate directories with "/"`,
			},
		},
		{
			name: "a passthrough file writes sitemap.xml in other letters",
			files: map[string]string{
				"gondola.yaml": withSitemap + "output:\n  passthrough: [Sitemap.xml]\n",
				"Sitemap.xml":  "<urlset/>",
				"map.html":     "---\npermalink: /sitemap.xml\n---\n",
			},
			// It stops the build at once, like every error of scan.
			wantErrors: []string{
				`passthrough file "Sitemap.xml" writes "Sitemap.xml", where Gondola writes the sitemap because config.sitemap is true; please rename or remove it, or turn config.sitemap off`,
			},
		},
		{
			name: "robots.txt saved as UTF-16",
			files: map[string]string{
				"gondola.yaml": withSitemap + "output:\n  passthrough: [robots.txt]\n",
				"robots.txt":   "\xFF\xFEU\x00",
				"index.html":   "home",
			},
			wantErrors: []string{"robots.txt: the file is saved as UTF-16; please save it as UTF-8"},
		},
		{
			name: "robots.txt is a directory",
			files: map[string]string{
				"gondola.yaml": withSitemap,
				"robots.html":  "---\npermalink: /robots.txt/\n---\n",
			},
			wantErrors: []string{"reading _site.temp/robots.txt: is a directory"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, test.files)
			result := runBuild(t, projectDir)
			if got := result.errorTexts(); !reflect.DeepEqual(got, test.wantErrors) {
				t.Errorf("errors:\n got %q\nwant %q", got, test.wantErrors)
			}
			checkNotPublished(t, projectDir, result)
		})
	}
}

func TestBuildSitemapOffAllowsSitemapFile(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"map.html": "---\nlayout: nil\npermalink: /sitemap.xml\n---\n<urlset/>",
	})
	result := runBuild(t, projectDir)
	if result.failure != nil {
		t.Fatalf("build failed: %q", result.errorTexts())
	}
	if got := readTree(t, filepath.Join(projectDir, "_site"))["sitemap.xml"]; got != "<urlset/>" {
		t.Errorf("sitemap.xml is %q, want the page's", got)
	}
}
