package routing

import (
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/scan"
)

// routings are both values of output.routing, for tests whose result does
// not depend on it.
var routings = []config.Routing{config.RoutingFile, config.RoutingDirectory}

// may1 is the date of the pages in the tests of the permalink "date".
var may1 = time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)

// routeTest is one case of the table tests: a page and, for each routing,
// the location it gets.
type routeTest struct {
	name      string
	page      Page
	file      Location
	directory Location
}

func runRouteTests(t *testing.T, tests []routeTest) {
	t.Helper()
	for _, test := range tests {
		for _, routing := range routings {
			want := test.file
			if routing == config.RoutingDirectory {
				want = test.directory
			}
			t.Run(test.name+"/"+string(routing), func(t *testing.T) {
				page := test.page
				if page.SourcePath == "" {
					page.SourcePath = page.ContentPath
				}
				got, err := Route(page, routing)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != want {
					t.Errorf("got  %+v\nwant %+v", got, want)
				}
			})
		}
	}
}

func pathPage(contentPath string) Page {
	return Page{ContentPath: contentPath, Permalink: "path"}
}

func TestRoutePath(t *testing.T) {
	runRouteTests(t, []routeTest{
		// The table of SPEC §5.7.
		{"about", pathPage("about.md"), Location{"about.html", "/about.html"}, Location{"about/index.html", "/about/"}},
		{"nested", pathPage("blog/post.md"), Location{"blog/post.html", "/blog/post.html"}, Location{"blog/post/index.html", "/blog/post/"}},
		{"index", pathPage("index.md"), Location{"index.html", "/"}, Location{"index.html", "/"}},
		{"nested index", pathPage("blog/index.md"), Location{"blog/index.html", "/blog/"}, Location{"blog/index.html", "/blog/"}},
		// Every page extension becomes .html.
		{"html", pathPage("about.html"), Location{"about.html", "/about.html"}, Location{"about/index.html", "/about/"}},
		{"htm", pathPage("pag/geo/orders.htm"), Location{"pag/geo/orders.html", "/pag/geo/orders.html"}, Location{"pag/geo/orders/index.html", "/pag/geo/orders/"}},
		{"markdown", pathPage("notes.markdown"), Location{"notes.html", "/notes.html"}, Location{"notes/index.html", "/notes/"}},
		{"extension in capitals", pathPage("About.HTML"), Location{"About.html", "/About.html"}, Location{"About/index.html", "/About/"}},
		{"index extension in capitals", pathPage("index.HTML"), Location{"index.html", "/"}, Location{"index.html", "/"}},
		{"deep index", pathPage("a/b/c/index.htm"), Location{"a/b/c/index.html", "/a/b/c/"}, Location{"a/b/c/index.html", "/a/b/c/"}},
		// Letters are kept as on disk; only "index" in small letters is an
		// index page.
		{"Index is not an index page", pathPage("Index.md"), Location{"Index.html", "/Index.html"}, Location{"Index/index.html", "/Index/"}},
		{"INDEX is not an index page", pathPage("blog/INDEX.md"), Location{"blog/INDEX.html", "/blog/INDEX.html"}, Location{"blog/INDEX/index.html", "/blog/INDEX/"}},
		{"directory named index", pathPage("index/post.md"), Location{"index/post.html", "/index/post.html"}, Location{"index/post/index.html", "/index/post/"}},
		{"404", pathPage("404.html"), Location{"404.html", "/404.html"}, Location{"404/index.html", "/404/"}},
		// Only the last extension is replaced.
		{"two dots", pathPage("v1.2.md"), Location{"v1.2.html", "/v1.2.html"}, Location{"v1.2/index.html", "/v1.2/"}},
		{"index with two dots", pathPage("index.en.md"), Location{"index.en.html", "/index.en.html"}, Location{"index.en/index.html", "/index.en/"}},
		{"name that ends in a dot", pathPage("a."), Location{"a.html", "/a.html"}, Location{"a/index.html", "/a/"}},
		// Names made of dots are still names when something is left.
		{"three dots and an extension", pathPage("....md"), Location{"....html", "/....html"}, Location{".../index.html", "/.../"}},
	})
}

func TestRouteURLEncoding(t *testing.T) {
	// Output files keep their names; URLs are percent-encoded.
	runRouteTests(t, []routeTest{
		{"accents", pathPage("città/è.md"), Location{"città/è.html", "/citt%C3%A0/%C3%A8.html"}, Location{"città/è/index.html", "/citt%C3%A0/%C3%A8/"}},
		{"space", pathPage("my page.md"), Location{"my page.html", "/my%20page.html"}, Location{"my page/index.html", "/my%20page/"}},
		{"percent", pathPage("100%.md"), Location{"100%.html", "/100%25.html"}, Location{"100%/index.html", "/100%25/"}},
		{"question mark and hash", pathPage("a?b#c.md"), Location{"a?b#c.html", "/a%3Fb%23c.html"}, Location{"a?b#c/index.html", "/a%3Fb%23c/"}},
		{"characters kept in URLs", pathPage("a+b,c;d=e@f$g&h~i_j-k.md"), Location{"a+b,c;d=e@f$g&h~i_j-k.html", "/a+b,c;d=e@f$g&h~i_j-k.html"}, Location{"a+b,c;d=e@f$g&h~i_j-k/index.html", "/a+b,c;d=e@f$g&h~i_j-k/"}},
		{"other script", pathPage("你好.md"), Location{"你好.html", "/%E4%BD%A0%E5%A5%BD.html"}, Location{"你好/index.html", "/%E4%BD%A0%E5%A5%BD/"}},
	})
}

func TestRouteSlug(t *testing.T) {
	slugPage := func(contentPath, title string) Page {
		return Page{ContentPath: contentPath, Permalink: "slug", Title: title}
	}
	runRouteTests(t, []routeTest{
		{"top level", slugPage("post.md", "My Title"), Location{"my-title.html", "/my-title.html"}, Location{"my-title/index.html", "/my-title/"}},
		{"directories stay", slugPage("blog/2024/post.md", "My Title"), Location{"blog/2024/my-title.html", "/blog/2024/my-title.html"}, Location{"blog/2024/my-title/index.html", "/blog/2024/my-title/"}},
		{"accents", slugPage("post.md", "Città di mare"), Location{"citta-di-mare.html", "/citta-di-mare.html"}, Location{"citta-di-mare/index.html", "/citta-di-mare/"}},
		{"other script", slugPage("post.md", "你好"), Location{"ni-hao.html", "/ni-hao.html"}, Location{"ni-hao/index.html", "/ni-hao/"}},
		{"English substitutions", slugPage("post.md", "Q&A"), Location{"qanda.html", "/qanda.html"}, Location{"qanda/index.html", "/qanda/"}},
		{"slash in the title", slugPage("post.md", "AC/DC"), Location{"ac-dc.html", "/ac-dc.html"}, Location{"ac-dc/index.html", "/ac-dc/"}},
		{"dots in the title", slugPage("post.md", "../../etc"), Location{"etc.html", "/etc.html"}, Location{"etc/index.html", "/etc/"}},
		{"title number", slugPage("post.md", "2024"), Location{"2024.html", "/2024.html"}, Location{"2024/index.html", "/2024/"}},
		{"source directory letters kept", slugPage("Blog/post.md", "Hello"), Location{"Blog/hello.html", "/Blog/hello.html"}, Location{"Blog/hello/index.html", "/Blog/hello/"}},
		// Index pages keep their path.
		{"index page", slugPage("index.md", "Home"), Location{"index.html", "/"}, Location{"index.html", "/"}},
		{"nested index page", slugPage("blog/index.md", "Blog"), Location{"blog/index.html", "/blog/"}, Location{"blog/index.html", "/blog/"}},
		{"index page without title", slugPage("index.md", ""), Location{"index.html", "/"}, Location{"index.html", "/"}},
		// A title whose slug is "index" makes an index page.
		{"title Index", slugPage("blog/post.md", "Index"), Location{"blog/index.html", "/blog/"}, Location{"blog/index.html", "/blog/"}},
	})
}

func TestRouteDate(t *testing.T) {
	datePage := func(contentPath, title string, date time.Time) Page {
		return Page{ContentPath: contentPath, Permalink: "date", Title: title, Date: date, HasDate: true}
	}
	lateEvening := time.Date(2024, time.May, 1, 23, 30, 0, 0, time.FixedZone("", -5*60*60))
	runRouteTests(t, []routeTest{
		{"top level", datePage("post.md", "My Title", may1), Location{"2024/05/01/my-title.html", "/2024/05/01/my-title.html"}, Location{"2024/05/01/my-title/index.html", "/2024/05/01/my-title/"}},
		{"source directories dropped", datePage("blog/2023/post.md", "My Title", may1), Location{"2024/05/01/my-title.html", "/2024/05/01/my-title.html"}, Location{"2024/05/01/my-title/index.html", "/2024/05/01/my-title/"}},
		// 2024-05-01T23:30:00-05:00 is May 2 in UTC; the date as written wins.
		{"date in its own zone", datePage("post.md", "Late", lateEvening), Location{"2024/05/01/late.html", "/2024/05/01/late.html"}, Location{"2024/05/01/late/index.html", "/2024/05/01/late/"}},
		{"early year", datePage("post.md", "Old", time.Date(812, time.December, 9, 0, 0, 0, 0, time.UTC)), Location{"0812/12/09/old.html", "/0812/12/09/old.html"}, Location{"0812/12/09/old/index.html", "/0812/12/09/old/"}},
		// Index pages keep their path, even without a date.
		{"index page", datePage("blog/index.md", "Blog", may1), Location{"blog/index.html", "/blog/"}, Location{"blog/index.html", "/blog/"}},
		{"index page without date", Page{ContentPath: "index.md", Permalink: "date"}, Location{"index.html", "/"}, Location{"index.html", "/"}},
		{"title Index", datePage("post.md", "Index", may1), Location{"2024/05/01/index.html", "/2024/05/01/"}, Location{"2024/05/01/index.html", "/2024/05/01/"}},
	})
}

func TestRouteCustomPermalink(t *testing.T) {
	custom := func(contentPath, permalink string) Page {
		return Page{ContentPath: contentPath, Permalink: permalink, Title: "Ignored", Date: may1, HasDate: true}
	}
	same := func(name string, page Page, location Location) routeTest {
		return routeTest{name, page, location, location}
	}
	// Custom permalinks ignore routing.
	runRouteTests(t, []routeTest{
		same("directory", custom("about.md", "/about-us/"), Location{"about-us/index.html", "/about-us/"}),
		same("root", custom("home.md", "/"), Location{"index.html", "/"}),
		same("file", custom("feed.html", "/feed.xml"), Location{"feed.xml", "/feed.xml"}),
		same("html file", custom("about.md", "/about-us.html"), Location{"about-us.html", "/about-us.html"}),
		same("file without extension", custom("about.md", "/about"), Location{"about", "/about"}),
		same("nested", custom("x.md", "/a/b/c/"), Location{"a/b/c/index.html", "/a/b/c/"}),
		same("source directories dropped", custom("blog/post.md", "/post/"), Location{"post/index.html", "/post/"}),
		same("index.html has its directory's URL", custom("x.md", "/blog/index.html"), Location{"blog/index.html", "/blog/"}),
		same("Index.html is a file", custom("x.md", "/blog/Index.html"), Location{"blog/Index.html", "/blog/Index.html"}),
		same("on an index page", custom("index.md", "/home/"), Location{"home/index.html", "/home/"}),
		same("doubled slashes cleaned", custom("x.md", "//a//b/"), Location{"a/b/index.html", "/a/b/"}),
		same("dot elements cleaned", custom("x.md", "/./a/./b.html"), Location{"a/b.html", "/a/b.html"}),
		same("final dot is a directory", custom("x.md", "/a/."), Location{"a/index.html", "/a/"}),
		same("root dot", custom("x.md", "/."), Location{"index.html", "/"}),
		same("dots inside names", custom("x.md", "/a..b/...html"), Location{"a..b/...html", "/a..b/...html"}),
		same("letters kept", custom("x.md", "/About/Us.HTML"), Location{"About/Us.HTML", "/About/Us.HTML"}),
		same("encoded", custom("x.md", "/città/my page/"), Location{"città/my page/index.html", "/citt%C3%A0/my%20page/"}),
		same("dot directory", custom("x.md", "/.well-known/security.txt"), Location{".well-known/security.txt", "/.well-known/security.txt"}),
	})
}

func TestRouteErrors(t *testing.T) {
	tests := []struct {
		name string
		page Page
		want string
	}{
		{"slug without title", Page{SourcePath: "src/blog/post.md", ContentPath: "blog/post.md", Permalink: "slug"},
			`src/blog/post.md: permalink "slug" needs a title, but the page has none; please set title or choose another permalink`},
		{"slug of symbols", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "slug", Title: "!!!"},
			`post.md: the title "!!!" has no letters or digits, so permalink "slug" cannot make a file name from it; please change the title or choose another permalink`},
		{"slug of spaces", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "slug", Title: "   "},
			`post.md: the title "   " has no letters or digits, so permalink "slug" cannot make a file name from it; please change the title or choose another permalink`},
		{"date without date", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "date", Title: "Hello"},
			`post.md: permalink "date" needs a date, but the page has none; please set date or choose another permalink`},
		{"date without anything", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "date"},
			`post.md: permalink "date" needs a date, but the page has none; please set date or choose another permalink`},
		{"date without title", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "date", Date: may1, HasDate: true},
			`post.md: permalink "date" needs a title, but the page has none; please set title or choose another permalink`},
		// Permalinks that try to leave the output directory, in case one
		// reaches routing without going through front matter.
		{"dot dot", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "/../secret.html"},
			`post.md: permalink "/../secret.html" is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"dot dot deeper", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "/a/../../b/"},
			`post.md: permalink "/a/../../b/" is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"backslash", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: `/..\..\x`},
			`post.md: permalink "/..\\..\\x" contains "\\"; please separate directories with "/"`},
		{"drive", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "/C:/Windows/x.html"},
			`post.md: permalink "/C:/Windows/x.html" contains ":"; Windows does not allow it in file names; please remove it`},
		{"relative", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "../x/"},
			`post.md: permalink "../x/" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"empty", Page{SourcePath: "post.md", ContentPath: "post.md"},
			`post.md: permalink "" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"permalink not UTF-8", Page{SourcePath: "post.md", ContentPath: "post.md", Permalink: "/\x8d"},
			`post.md: permalink "/\x8d" is not valid UTF-8; please write it in UTF-8`},
		// Content paths that package scan never produces.
		{"content path not UTF-8", Page{SourcePath: "caf\xe9.md", ContentPath: "caf\xe9.md", Permalink: "path"},
			"routing caf\xe9.md: content path \"caf\\xe9.md\" is not the path of a file inside the content directory"},
		{"content path with dot dot", Page{SourcePath: "x.md", ContentPath: "../x.md", Permalink: "path"},
			`routing x.md: content path "../x.md" is not the path of a file inside the content directory`},
		{"content path inside with dot dot", Page{SourcePath: "x.md", ContentPath: "a/../../x.md", Permalink: "path"},
			`routing x.md: content path "a/../../x.md" is not the path of a file inside the content directory`},
		{"absolute content path", Page{SourcePath: "x.md", ContentPath: "/etc/x.md", Permalink: "path"},
			`routing x.md: content path "/etc/x.md" is not the path of a file inside the content directory`},
		{"empty content path", Page{SourcePath: "x.md", Permalink: "path"},
			`routing x.md: content path "" is not the path of a file inside the content directory`},
		{"content path dot", Page{SourcePath: "x.md", ContentPath: ".", Permalink: "path"},
			`routing x.md: content path "." is not the path of a file inside the content directory`},
		{"content path that is only an extension", Page{SourcePath: "x.md", ContentPath: "blog/.md", Permalink: "path"},
			`routing x.md: content path "blog/.md" is not the path of a file inside the content directory`},
		{"content path whose name is dot dot", Page{SourcePath: "x.md", ContentPath: "...", Permalink: "path"},
			`routing x.md: content path "..." is not the path of a file inside the content directory`},
		{"content path whose name is dot dot deeper", Page{SourcePath: "x.md", ContentPath: "a/...", Permalink: "path"},
			`routing x.md: content path "a/..." is not the path of a file inside the content directory`},
		{"content path whose name is dot", Page{SourcePath: "x.md", ContentPath: "..md", Permalink: "path"},
			`routing x.md: content path "..md" is not the path of a file inside the content directory`},
		{"content path not clean", Page{SourcePath: "x.md", ContentPath: "a//x.md", Permalink: "path"},
			`routing x.md: content path "a//x.md" is not the path of a file inside the content directory`},
	}
	for _, test := range tests {
		for _, routing := range routings {
			t.Run(test.name+"/"+string(routing), func(t *testing.T) {
				_, err := Route(test.page, routing)
				if err == nil || err.Error() != test.want {
					t.Errorf("error:\n got %v\nwant %s", err, test.want)
				}
			})
		}
	}
}

func TestRouteUnknownRouting(t *testing.T) {
	_, err := Route(pathPage("about.md"), config.Routing("folder"))
	if err == nil || err.Error() != `unknown routing "folder"` {
		t.Errorf("error: got %v", err)
	}
}

// TestRouteCollisions routes pages and claims their output paths on a
// scan.OutputPaths, as the build does, to check that two pages that write
// the same file, or files that differ only in letter case, are caught.
func TestRouteCollisions(t *testing.T) {
	tests := []struct {
		name    string
		routing config.Routing
		pages   []Page
		want    string // "" when no pages collide
	}{
		{"markdown and HTML", config.RoutingFile,
			[]Page{pathPage("about.md"), pathPage("about.html")},
			`page "about.md" and page "about.html" both write "about.html" in the output; please rename or remove one`},
		{"file and directory index", config.RoutingDirectory,
			[]Page{pathPage("about.md"), pathPage("about/index.md")},
			`page "about.md" and page "about/index.md" both write "about/index.html" in the output; please rename or remove one`},
		{"no collision with file routing", config.RoutingFile,
			[]Page{pathPage("about.md"), pathPage("about/index.md")}, ""},
		{"letter case", config.RoutingFile,
			[]Page{pathPage("Index.md"), pathPage("index.md")},
			`page "Index.md" and page "index.md" both write "Index.html" and "index.html", which differ only in letter case, in the output; please rename or remove one`},
		{"letter case of directories", config.RoutingDirectory,
			[]Page{pathPage("Blog/post.md"), pathPage("blog/post.md")},
			`page "Blog/post.md" and page "blog/post.md" both write "Blog/post/index.html" and "blog/post/index.html", which differ only in letter case, in the output; please rename or remove one`},
		{"same title", config.RoutingFile,
			[]Page{{ContentPath: "a.md", Permalink: "slug", Title: "Hello"}, {ContentPath: "b.md", Permalink: "slug", Title: "hello!"}},
			`page "a.md" and page "b.md" both write "hello.html" in the output; please rename or remove one`},
		{"same title in two directories", config.RoutingFile,
			[]Page{{ContentPath: "a/x.md", Permalink: "slug", Title: "Hello"}, {ContentPath: "b/x.md", Permalink: "slug", Title: "Hello"}}, ""},
		{"same day and title", config.RoutingDirectory,
			[]Page{{ContentPath: "a/x.md", Permalink: "date", Title: "Hello", Date: may1, HasDate: true}, {ContentPath: "b/y.md", Permalink: "date", Title: "Hello", Date: may1, HasDate: true}},
			`page "a/x.md" and page "b/y.md" both write "2024/05/01/hello/index.html" in the output; please rename or remove one`},
		{"custom permalink and a page", config.RoutingDirectory,
			[]Page{pathPage("about.md"), {ContentPath: "x.md", Permalink: "/about/"}},
			`page "about.md" and page "x.md" both write "about/index.html" in the output; please rename or remove one`},
		{"custom permalink in other letters", config.RoutingFile,
			[]Page{pathPage("index.md"), {ContentPath: "x.md", Permalink: "/INDEX.HTML"}},
			`page "index.md" and page "x.md" both write "index.html" and "INDEX.HTML", which differ only in letter case, in the output; please rename or remove one`},
		{"two roots", config.RoutingFile,
			[]Page{{ContentPath: "home.md", Permalink: "/"}, pathPage("index.html")},
			`page "home.md" and page "index.html" both write "index.html" in the output; please rename or remove one`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var outputPaths scan.OutputPaths
			var err error
			for _, page := range test.pages {
				page.SourcePath = page.ContentPath
				location, routeErr := Route(page, test.routing)
				if routeErr != nil {
					t.Fatalf("unexpected error: %v", routeErr)
				}
				if err = outputPaths.Claim(location.OutputPath, scan.PageSource, page.SourcePath); err != nil {
					break
				}
			}
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != test.want {
				t.Errorf("error:\n got %s\nwant %s", got, test.want)
			}
		})
	}
}
