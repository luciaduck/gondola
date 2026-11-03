package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
)

// minimal is the smallest valid gondola.yaml. Lines 1 to 3 are its own, so
// a section added after it starts on line 4.
const minimal = "config:\n  site-name: S\n  version: 1\n  canonical-url: https://example.com\n"

// parseConfig returns the configuration of minimal followed by more.
func parseConfig(t *testing.T, more string) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(minimal + more))
	if err != nil {
		t.Fatalf("config.Parse: %v", err)
	}
	return cfg
}

// makeTree creates the files in a new temporary directory and returns it.
// A path that ends in "/" is an empty directory.
func makeTree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, slashed := range paths {
		full := filepath.Join(root, filepath.FromSlash(slashed))
		if strings.HasSuffix(slashed, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(slashed), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// describe lists what Scan found, one line per file, so that tables can
// state it briefly:
//
//	html blog/post.html as blog/post.html
//	markdown src/index.md as index.md
//	sass style/main.scss to style/main.css
//	passthrough robots.txt
func describe(result Result) []string {
	lines := []string{}
	for _, page := range result.Pages {
		kind := "html"
		if page.IsMarkdown {
			kind = "markdown"
		}
		lines = append(lines, fmt.Sprintf("%s %s as %s", kind, page.SourcePath, page.ContentPath))
	}
	for _, entryPoint := range result.SassEntryPoints {
		lines = append(lines, fmt.Sprintf("sass %s to %s", entryPoint.SourcePath, entryPoint.OutputPath))
	}
	for _, file := range result.PassthroughFiles {
		if file.OutputPath != file.SourcePath {
			lines = append(lines, fmt.Sprintf("passthrough %s to %s (output path differs)", file.SourcePath, file.OutputPath))
			continue
		}
		lines = append(lines, "passthrough "+file.SourcePath)
	}
	return lines
}

type scanTest struct {
	name   string
	config string
	files  []string
	want   []string
}

func runScanTests(t *testing.T, tests []scanTest) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := makeTree(t, test.files...)
			result, err := Scan(projectDir, parseConfig(t, test.config))
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got := describe(result); !reflect.DeepEqual(got, test.want) {
				t.Errorf("found:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(test.want, "\n  "))
			}
		})
	}
}

// siteFiles is a site with the default directories, holding a file of
// every kind.
var siteFiles = []string{
	"gondola.yaml", "alias.squero",
	"index.md", "about.html", "old.htm", "notes.markdown", "README.md",
	"photo.jpg", "layout.jet", "blog/post.md", "blog/_draft.md", "blog/.secret.html",
	"style/main.scss", "style/print.sass", "style/_variables.scss",
	"_components/includes/base.jet", "_components/includes/page.html",
	"_components/sass/_mixins.scss", "_data/site.json",
	"_site/old.html", "_site.temp/x.html", ".git/config", ".git/x.md",
	"_drafts/a.md", ".well-known/security.txt",
}

func TestScanClassifies(t *testing.T) {
	runScanTests(t, []scanTest{
		{"default directories", "", siteFiles, []string{
			// os.ReadDir sorts by bytes: capitals come before "_" and small
			// letters.
			"markdown README.md as README.md",
			"html about.html as about.html",
			"markdown blog/post.md as blog/post.md",
			"markdown index.md as index.md",
			"markdown notes.markdown as notes.markdown",
			"html old.htm as old.htm",
			"sass style/main.scss to style/main.css",
			"sass style/print.sass to style/print.css",
		}},
		{"markdown off", "  markdown: false\n", siteFiles, []string{
			"html about.html as about.html",
			"html old.htm as old.htm",
			"sass style/main.scss to style/main.css",
			"sass style/print.sass to style/print.css",
		}},
		{"sass off", "  sass: false\n", siteFiles, []string{
			"markdown README.md as README.md",
			"html about.html as about.html",
			"markdown blog/post.md as blog/post.md",
			"markdown index.md as index.md",
			"markdown notes.markdown as notes.markdown",
			"html old.htm as old.htm",
		}},
		{"extensions in capitals", "", []string{"INDEX.HTML", "Post.MD", "Guide.Markdown", "Old.HTM", "Main.SCSS", "Theme.Sass"}, []string{
			"markdown Guide.Markdown as Guide.Markdown",
			"html INDEX.HTML as INDEX.HTML",
			"html Old.HTM as Old.HTM",
			"markdown Post.MD as Post.MD",
			"sass Main.SCSS to Main.css",
			"sass Theme.Sass to Theme.css",
		}},
		{"names that only look like extensions", "", []string{"md", "html.txt", "x.md.bak", "x.", "page.html/"}, []string{}},
		{"content in a subdirectory", "input:\n  content: src\n", []string{
			"top.md", "docs/guide.html", "src/index.md", "src/_partial.md", "src/blog/post.html",
			"src/assets/site.scss", "assets/outside.scss",
		}, []string{
			"html src/blog/post.html as blog/post.html",
			"markdown src/index.md as index.md",
			"sass src/assets/site.scss to assets/site.css",
		}},
		{"content directory whose name starts with an underscore", "input:\n  content: _src\n", []string{
			"_src/index.md", "_src/_skip.md", "_src/.skip/x.md", "index.md",
		}, []string{
			"markdown _src/index.md as index.md",
		}},
		{"content directory two levels down", "input:\n  content: .site/src\n", []string{
			".site/top.md", ".site/src/index.md", ".site/other/x.md",
		}, []string{
			"markdown .site/src/index.md as index.md",
		}},
		{"role directories inside content", "input:\n  templates: layouts\n  data: data\n  partials: [styles, shared/sass]\noutput:\n  directory: public\n", []string{
			"layouts/base.html", "layouts/post.md", "data/menu.md", "styles/main.scss",
			"shared/sass/x.scss", "shared/page.html", "public/old.html", "public.temp/x.html",
			"layoutsx/page.html", "index.md",
		}, []string{
			"markdown index.md as index.md",
			"html layoutsx/page.html as layoutsx/page.html",
			"html shared/page.html as shared/page.html",
		}},
		{"role directory named by its file", "input:\n  data: data.md\n", []string{"data.md", "index.md"}, []string{
			"markdown index.md as index.md",
		}},
		{"empty project", "", []string{}, []string{}},
	})
}

func TestScanPassthrough(t *testing.T) {
	runScanTests(t, []scanTest{
		{"leading slash is optional", "output:\n  passthrough: [/robots.txt, humans.txt]\n",
			[]string{"robots.txt", "humans.txt", "sub/robots.txt"},
			[]string{"passthrough humans.txt", "passthrough robots.txt"}},
		{"star stays within one level, at the root", "output:\n  passthrough: [\"*.ico\"]\n",
			[]string{"favicon.ico", "apple.ico", "img/x.ico"},
			[]string{"passthrough apple.ico", "passthrough favicon.ico"}},
		{"star in a directory", "output:\n  passthrough: [\"img/*.png\"]\n",
			[]string{"img/a.png", "img/b.jpg", "img/deep/c.png"},
			[]string{"passthrough img/a.png"}},
		{"trailing slash copies everything inside", "output:\n  passthrough: [assets/]\n",
			[]string{"assets/logo.png", "assets/fonts/a.woff2", "assets/fonts/deep/b.woff2", "assetsx/no.png", "assets.png"},
			[]string{"passthrough assets/fonts/a.woff2", "passthrough assets/fonts/deep/b.woff2", "passthrough assets/logo.png"}},
		{"star before a trailing slash", "output:\n  passthrough: [\"docs/*/\"]\n",
			[]string{"docs/a/x.txt", "docs/b/c/y.txt", "docs/z.txt"},
			[]string{"passthrough docs/a/x.txt", "passthrough docs/b/c/y.txt"}},
		{"character set", "output:\n  passthrough: [\"icon-[0-9].png\"]\n",
			[]string{"icon-1.png", "icon-a.png", "icon-12.png"},
			[]string{"passthrough icon-1.png"}},
		{"dot directory below content", "output:\n  passthrough: [.well-known/]\n",
			[]string{".well-known/security.txt", ".well-known/a/b.txt", ".git/config"},
			[]string{"passthrough .well-known/a/b.txt", "passthrough .well-known/security.txt"}},
		{"underscore file", "output:\n  passthrough: [_redirects, _headers]\n",
			[]string{"_redirects", "_headers", "_other"},
			[]string{"passthrough _headers", "passthrough _redirects"}},
		{"inside a skipped directory", "output:\n  passthrough: [_static/keep.txt]\n",
			[]string{"_static/keep.txt", "_static/page.html"},
			[]string{"passthrough _static/keep.txt"}},
		{"a match wins over a page or a Sass file", "output:\n  passthrough: [static/]\n",
			[]string{"static/google123.html", "static/raw.scss", "static/_x.md", "index.md"},
			[]string{"markdown index.md as index.md", "passthrough static/_x.md", "passthrough static/google123.html", "passthrough static/raw.scss"}},
		{"outside the content directory", "input:\n  content: src\noutput:\n  passthrough: [/assets/, src/robots.txt, CNAME]\n",
			[]string{"assets/logo.png", "src/robots.txt", "src/index.md", "CNAME"},
			[]string{"markdown src/index.md as index.md", "passthrough CNAME", "passthrough assets/logo.png", "passthrough src/robots.txt"}},
		{"star at the root takes files only", "output:\n  passthrough: [\"*\"]\n",
			[]string{"a.txt", "index.md", "blog/post.md"},
			[]string{"markdown blog/post.md as blog/post.md", "passthrough a.txt", "passthrough index.md"}},
		{"patterns are cleaned", "output:\n  passthrough: [./a.txt, \"b//c.txt\", d/./e/]\n",
			[]string{"a.txt", "b/c.txt", "d/e/f.txt"},
			[]string{"passthrough a.txt", "passthrough b/c.txt", "passthrough d/e/f.txt"}},
		{"two patterns match one file", "output:\n  passthrough: [docs/, docs/a.txt]\n",
			[]string{"docs/a.txt"},
			[]string{"passthrough docs/a.txt"}},
		{"letter case counts", "output:\n  passthrough: [Robots.txt]\n",
			[]string{"Robots.txt", "robots.txt"},
			[]string{"passthrough Robots.txt"}},
	})
}

type scanErrorTest struct {
	name   string
	config string
	files  []string
	want   string
}

func runScanErrorTests(t *testing.T, tests []scanErrorTest) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := makeTree(t, test.files...)
			_, err := Scan(projectDir, parseConfig(t, test.config))
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestScanErrors(t *testing.T) {
	// The passthrough list starts at column 17 of line 6:
	//   output:
	//     passthrough: [
	const at = "gondola.yaml:6:17: "
	notAllowed := `is not allowed: patterns are relative to the project directory and may not contain ".." or be absolute`
	wholeProject := "is the whole project directory; please name the files or directories to copy"
	doubleStar := `uses "**", which Gondola does not support; to copy a directory and everything inside it, end the pattern with "/", as in "assets/"`
	invalid := `is not a valid pattern; please check the characters written between "[" and "]"`
	noFile := "matches no file (role directories and symbolic links are never copied)"
	runScanErrorTests(t, []scanErrorTest{
		{"backslash", "output:\n  passthrough: ['assets\\img\\']\n", nil,
			at + `output.passthrough "assets\\img\\" contains "\"; please separate directories with "/"`},
		{"tab from double quotes", "output:\n  passthrough: [\"assets\\tmp/\"]\n", nil,
			at + `output.passthrough "assets\tmp/" contains a tab; inside double quotes YAML reads "\t" as a tab; please separate directories with "/"`},
		{"control character", "output:\n  passthrough: [\"a\\x7fb\"]\n", nil,
			at + `output.passthrough "a\x7fb" contains a control character (U+007F); please remove it`},
		{"parent directory", "output:\n  passthrough: [../secrets/]\n", nil, at + `output.passthrough "../secrets/" ` + notAllowed},
		{"parent directory after the root", "output:\n  passthrough: [/../x]\n", nil, at + `output.passthrough "/../x" ` + notAllowed},
		{"parent directory that stays inside", "output:\n  passthrough: [a/../b]\n", nil, at + `output.passthrough "a/../b" ` + notAllowed},
		{"two leading slashes", "output:\n  passthrough: [//server/x]\n", nil, at + `output.passthrough "//server/x" ` + notAllowed},
		{"drive letter", "output:\n  passthrough: [\"C:/site\"]\n", nil, at + `output.passthrough "C:/site" ` + notAllowed},
		{"drive letter without a slash", "output:\n  passthrough: [\"c:site\"]\n", nil, at + `output.passthrough "c:site" ` + notAllowed},
		{"drive letter after a dot", "output:\n  passthrough: [\"./C:x\"]\n", nil, at + `output.passthrough "./C:x" ` + notAllowed},
		{"colon deeper down is a name", "output:\n  passthrough: [\"a/C:x\"]\n", nil, at + `output.passthrough "a/C:x" ` + noFile},
		{"root alone", "output:\n  passthrough: [/]\n", nil, at + `output.passthrough "/" ` + wholeProject},
		{"dot", "output:\n  passthrough: [.]\n", nil, at + `output.passthrough "." ` + wholeProject},
		{"dot slash", "output:\n  passthrough: [/./]\n", nil, at + `output.passthrough "/./" ` + wholeProject},
		{"double star", "output:\n  passthrough: [\"assets/**\"]\n", nil, at + `output.passthrough "assets/**" ` + doubleStar},
		{"double star inside a name", "output:\n  passthrough: [\"a**b\"]\n", nil, at + `output.passthrough "a**b" ` + doubleStar},
		{"unclosed set", "output:\n  passthrough: [\"a[\"]\n", nil, at + `output.passthrough "a[" ` + invalid},
		{"empty set", "output:\n  passthrough: [\"x/[]/y\"]\n", nil, at + `output.passthrough "x/[]/y" ` + invalid},
		{"set with an open range", "output:\n  passthrough: [\"[a-]\"]\n", nil, at + `output.passthrough "[a-]" ` + invalid},
		{"the first bad pattern is reported", "output:\n  passthrough: [ok.txt, \"a[\"]\n", []string{"ok.txt"},
			`gondola.yaml:6:25: output.passthrough "a[" ` + invalid},
		{"pattern errors come before the walk", "input:\n  content: missing\noutput:\n  passthrough: [\"**\"]\n", nil,
			`gondola.yaml:8:17: output.passthrough "**" ` + doubleStar},

		{"no file", "output:\n  passthrough: [robot.txt]\n", []string{"robots.txt"}, at + `output.passthrough "robot.txt" ` + noFile},
		{"only a directory", "output:\n  passthrough: [.well-known]\n", []string{".well-known/security.txt"},
			at + `output.passthrough ".well-known" matches no file but matches the directory ".well-known"; to copy a directory and everything inside it, end the pattern with "/"`},
		{"only a deeper directory", "output:\n  passthrough: [\"docs/*\"]\n", []string{"docs/a/x.txt"},
			at + `output.passthrough "docs/*" matches no file but matches the directory "docs/a"; to copy a directory and everything inside it, end the pattern with "/"`},
		{"an empty directory", "output:\n  passthrough: [empty/]\n", []string{"empty/"}, at + `output.passthrough "empty/" ` + noFile},
		{"a file named as a directory", "output:\n  passthrough: [notes/]\n", []string{"notes"}, at + `output.passthrough "notes/" ` + noFile},
		{"inside a role directory", "input:\n  templates: layouts\noutput:\n  passthrough: [layouts/]\n", []string{"layouts/base.html"},
			`gondola.yaml:8:17: output.passthrough "layouts/" ` + noFile},
		{"inside the output directory", "output:\n  passthrough: [_site/]\n", []string{"_site/index.html"}, at + `output.passthrough "_site/" ` + noFile},
		{"inside the temp directory", "output:\n  passthrough: [_site.temp/x.html]\n", []string{"_site.temp/x.html"},
			at + `output.passthrough "_site.temp/x.html" ` + noFile},
		{"the first unmatched pattern is reported", "output:\n  passthrough: [a.txt, b.txt, c.txt]\n", []string{"a.txt"},
			`gondola.yaml:6:24: output.passthrough "b.txt" ` + noFile},

		{"content directory missing", "input:\n  content: src\n", []string{"index.md"}, `gondola.yaml:6:12: input.content "src" does not exist`},
		{"content directory is a file", "input:\n  content: src\n", []string{"src"}, `gondola.yaml:6:12: input.content "src" is a file, not a directory`},
	})
}

// TestScanContentDirectoryLetterCase checks that input.content is found as
// the file system finds it: "Src" names "src" only where letter case is
// ignored, as on Windows and macOS.
func TestScanContentDirectoryLetterCase(t *testing.T) {
	const config = "input:\n  content: Src\n"
	if !fileSystemIgnoresCase(t) {
		runScanErrorTests(t, []scanErrorTest{
			{"differs in letter case", config, []string{"src/index.md"}, `gondola.yaml:6:12: input.content "Src" does not exist`},
		})
		return
	}
	projectDir := makeTree(t, "src/index.md")
	if _, err := Scan(projectDir, parseConfig(t, config)); err != nil {
		t.Errorf("Scan returned error: %v", err)
	}
}

func TestScanOutputCollisions(t *testing.T) {
	runScanErrorTests(t, []scanErrorTest{
		{"two Sass files", "", []string{"style/a.sass", "style/a.scss"},
			`Sass entry point "style/a.sass" and Sass entry point "style/a.scss" both write "style/a.css" in the output; please rename or remove one`},
		{"passthrough and Sass", "output:\n  passthrough: [\"*.css\"]\n", []string{"a.css", "a.scss"},
			`passthrough file "a.css" and Sass entry point "a.scss" both write "a.css" in the output; please rename or remove one`},
		{"Sass in a content directory and passthrough at the root", "input:\n  content: src\noutput:\n  passthrough: [main.css]\n", []string{"main.css", "src/main.scss"},
			`passthrough file "main.css" and Sass entry point "src/main.scss" both write "main.css" in the output; please rename or remove one`},
		{"Sass extensions in capitals", "", []string{"a.SCSS", "a.sass"},
			`Sass entry point "a.SCSS" and Sass entry point "a.sass" both write "a.css" in the output; please rename or remove one`},
	})
}

// TestScanReturnsOutputPaths checks that the build can claim the pages'
// paths on the record that Scan filled, so that a page colliding with a
// Sass or passthrough output is caught.
func TestScanReturnsOutputPaths(t *testing.T) {
	cfg := parseConfig(t, "output:\n  passthrough: [\"*.txt\"]\n")
	root := makeTree(t, "robots.txt", "style/main.scss", "index.html")
	result, err := Scan(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		outputPath string
		wantErr    string
	}{
		{"robots.txt", `passthrough file "robots.txt" and page "robots.md" both write "robots.txt" in the output; please rename or remove one`},
		{"style/Main.css", `Sass entry point "style/main.scss" and page "robots.md" both write "style/main.css" and "style/Main.css", which differ only in letter case, in the output; please rename or remove one`},
		{"index.html", ""},
	}
	for _, test := range tests {
		err := result.OutputPaths.Claim(test.outputPath, PageSource, "robots.md")
		switch {
		case test.wantErr == "" && err != nil:
			t.Errorf("Claim(%q): %v", test.outputPath, err)
		case test.wantErr != "" && (err == nil || err.Error() != test.wantErr):
			t.Errorf("Claim(%q) = %v, want %s", test.outputPath, err, test.wantErr)
		}
	}
}

func TestScanReturnsOutputPathsWithoutOutputs(t *testing.T) {
	result, err := Scan(makeTree(t, "index.html"), parseConfig(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	if result.OutputPaths == nil {
		t.Fatal("OutputPaths is nil")
	}
	if err := result.OutputPaths.Claim("index.html", PageSource, "index.html"); err != nil {
		t.Fatal(err)
	}
}

func TestScanOutputCollisionsInLetterCase(t *testing.T) {
	if fileSystemIgnoresCase(t) {
		t.Skip("this file system cannot hold two names that differ only in letter case")
	}
	runScanErrorTests(t, []scanErrorTest{
		{"two passthrough files", "output:\n  passthrough: [\"*.png\"]\n", []string{"Logo.png", "logo.png"},
			`passthrough file "Logo.png" and passthrough file "logo.png" both write "Logo.png" and "logo.png", which differ only in letter case, in the output; please rename or remove one`},
		{"two Sass files", "", []string{"Main.scss", "main.sass"},
			`Sass entry point "Main.scss" and Sass entry point "main.sass" both write "Main.css" and "main.css", which differ only in letter case, in the output; please rename or remove one`},
	})
}

// fileSystemIgnoresCase reports whether the file system of the temporary
// directories ignores letter case.
func fileSystemIgnoresCase(t *testing.T) bool {
	t.Helper()
	dir := makeTree(t, "probe")
	_, err := os.Stat(filepath.Join(dir, "PROBE"))
	return err == nil
}

// TestScanIgnoringLetterCase runs on any file system: the tree is written in
// small letters and gondola.yaml in capitals, as a user on Windows or macOS
// might write it.
func TestScanIgnoringLetterCase(t *testing.T) {
	tests := []struct {
		name   string
		config string
		files  []string
		want   []string
	}{
		{"pattern", "output:\n  passthrough: [/Robots.TXT, Assets/]\n", []string{"robots.txt", "assets/a.png"},
			[]string{"passthrough assets/a.png", "passthrough robots.txt"}},
		{"pattern with a character set", "output:\n  passthrough: [\"icon-[A-C].png\"]\n", []string{"icon-b.png", "icon-d.png"},
			[]string{"passthrough icon-b.png"}},
		{"role directory", "input:\n  templates: Layouts\n  partials: STYLES\n", []string{"layouts/base.html", "styles/a.scss", "index.html"},
			[]string{"html index.html as index.html"}},
		{"output directory", "output:\n  directory: Public\n", []string{"public/old.html", "public.temp/x.html", "index.html"},
			[]string{"html index.html as index.html"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := makeTree(t, test.files...)
			cfg := parseConfig(t, test.config)
			cfg.IgnoresLetterCase = true
			result, err := Scan(projectDir, cfg)
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if got := describe(result); !reflect.DeepEqual(got, test.want) {
				t.Errorf("found:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(test.want, "\n  "))
			}
		})
	}
}

// TestWalkerFindsContentDirIgnoringLetterCase covers a content directory
// written in other letters than on disk. Scan itself cannot be run for it
// on a file system that tells letters apart, because the directory would not
// be found.
func TestWalkerFindsContentDirIgnoringLetterCase(t *testing.T) {
	cfg := parseConfig(t, "input:\n  content: Site/SRC\n")
	cfg.IgnoresLetterCase = true
	w := newWalker(t.TempDir(), cfg, nil)
	if !w.leadsToContentDir([]string{"site"}) {
		t.Errorf("site does not lead to the content directory")
	}
	if !w.isContentDir([]string{"site", "src"}) {
		t.Errorf("site/src is not the content directory")
	}
	cfg.IgnoresLetterCase = false
	w = newWalker(t.TempDir(), cfg, nil)
	if w.isContentDir([]string{"site", "src"}) {
		t.Errorf("site/src is the content directory when letters count")
	}
}

// makeLink creates a symbolic link at linkPath (relative to root) pointing
// at target, or skips the test where the system does not allow it (Windows
// without developer mode).
func makeLink(t *testing.T, root, target, linkPath string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(linkPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash(target), full); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
}

func TestScanSymbolicLinks(t *testing.T) {
	const refused = " is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"
	tests := []struct {
		name   string
		config string
		// links maps a link's path to its target, relative to the link's
		// directory; an empty target makes an ordinary file instead.
		links map[string]string
		want  string // the error; empty when the link is skipped
	}{
		{"page", "", map[string]string{"post.md": "_real/elsewhere.txt"}, "post.md" + refused},
		{"page in capitals", "", map[string]string{"POST.HTML": "_real/elsewhere.txt"}, "POST.HTML" + refused},
		{"Sass entry point", "", map[string]string{"main.scss": "_real/elsewhere.txt"}, "main.scss" + refused},
		{"broken link with a page name", "", map[string]string{"gone.html": "nowhere.html"}, "gone.html" + refused},
		{"directory inside content", "", map[string]string{"blog": "_real"}, "blog" + refused},
		{"content directory", "input:\n  content: src\n", map[string]string{"src": "_real"}, "src" + refused},
		{"directory above the content directory", "input:\n  content: site/src\n", map[string]string{"site": "_real"}, "site" + refused},
		{"passthrough file", "output:\n  passthrough: [robots.txt]\n", map[string]string{"robots.txt": "_real/elsewhere.txt"}, "robots.txt" + refused},
		{"file inside a passthrough directory", "output:\n  passthrough: [static/]\n", map[string]string{"static/x.txt": "../_real/elsewhere.txt"}, "static/x.txt" + refused},
		{"directory a pattern goes through", "output:\n  passthrough: [vendor/lib/x.js]\n", map[string]string{"vendor": "_real"}, "vendor" + refused},
		{"directory a pattern covers", "output:\n  passthrough: [static/]\n", map[string]string{"static/fonts": "../_real"}, "static/fonts" + refused},

		{"file with another extension", "", map[string]string{"notes.txt": "_real/elsewhere.txt"}, ""},
		{"page under a skipped directory", "", map[string]string{"_drafts/x.md": "../_real/elsewhere.txt"}, ""},
		{"page with a skipped name", "", map[string]string{"_x.md": "_real/elsewhere.txt"}, ""},
		{"directory with a skipped name", "", map[string]string{".cache": "_real"}, ""},
		{"directory outside content", "input:\n  content: src\n", map[string]string{"vendor": "_real", "src/index.md": ""}, ""},
		{"Sass file with Sass off", "  sass: false\n", map[string]string{"a.scss": "_real/elsewhere.txt"}, ""},
		{"page with Markdown off", "  markdown: false\n", map[string]string{"a.md": "_real/elsewhere.txt"}, ""},
		{"inside a role directory", "input:\n  templates: layouts\n", map[string]string{"layouts/base.html": "../_real/elsewhere.txt"}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// _real holds the targets; its name keeps it out of the scan.
			// _real/src lets a link named site stand for site/src.
			root := makeTree(t, "_real/elsewhere.txt", "_real/page.md", "_real/src/", "layouts/", "static/")
			for linkPath, target := range test.links {
				if target == "" {
					// An ordinary file, so that the scan finds something.
					full := filepath.Join(root, filepath.FromSlash(linkPath))
					if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(full, nil, 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				makeLink(t, root, target, linkPath)
			}
			_, err := Scan(root, parseConfig(t, test.config))
			switch {
			case test.want == "" && err != nil:
				t.Errorf("Scan: %v, want the link skipped", err)
			case test.want != "" && (err == nil || err.Error() != test.want):
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

// TestScanEntersOnlyNeededDirectories proves that the walk keeps out of a
// skipped directory that no pattern points into: it cannot read .git here,
// so entering it would fail.
func TestScanEntersOnlyNeededDirectories(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory permissions do not stop reading on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := makeTree(t, ".git/config", "index.md")
	locked := filepath.Join(root, ".git")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0o755) })

	result, err := Scan(root, parseConfig(t, "output:\n  passthrough: [\".w*/\"]\n  # .well-known is absent, so this pattern fails after the walk\n"))
	want := `gondola.yaml:6:17: output.passthrough ".w*/" matches no file (role directories and symbolic links are never copied)`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
	if result, err = Scan(root, parseConfig(t, "")); err != nil {
		t.Errorf("Scan without patterns: %v", err)
	} else if got := describe(result); !reflect.DeepEqual(got, []string{"markdown index.md as index.md"}) {
		t.Errorf("found %v", got)
	}

	_, err = Scan(root, parseConfig(t, "output:\n  passthrough: [.git/]\n"))
	if err == nil || !strings.HasPrefix(err.Error(), `reading directory ".git": `) {
		t.Errorf("error = %v, want one about reading .git", err)
	}
}

// skipUnlessNamesMayBeAnyBytes skips the test on a file system that refuses
// file names that are not valid UTF-8, as APFS on macOS and NTFS do.
func skipUnlessNamesMayBeAnyBytes(t *testing.T) {
	t.Helper()
	const notUTF8Name = "caf\xe9"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, notUTF8Name), nil, 0o644); err != nil {
		t.Skipf("the file system refuses names that are not UTF-8: %v", err)
	}
	// Windows stores names as UTF-16, and Go writes U+FFFD there for each
	// byte that is not UTF-8 instead of failing.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != notUTF8Name {
		t.Skip("the file system does not keep names that are not UTF-8")
	}
}

func TestScanNamesNotUTF8(t *testing.T) {
	skipUnlessNamesMayBeAnyBytes(t)
	// "caf\xe9" is "café" in Latin-1: the byte 0xE9 alone is not UTF-8.
	const notUTF8 = ": the file name is not valid UTF-8; please rename it"
	runScanErrorTests(t, []scanErrorTest{
		{"page", "", []string{"index.md", "caf\xe9.md"}, `caf\xe9.md` + notUTF8},
		{"HTML page", "", []string{"caf\xe9.HTML"}, `caf\xe9.HTML` + notUTF8},
		{"Sass entry point", "", []string{"style/caf\xe9.scss"}, `style/caf\xe9.scss` + notUTF8},
		{"passthrough file", "output:\n  passthrough: [static/]\n", []string{"static/caf\xe9.png"}, `static/caf\xe9.png` + notUTF8},
		{"passthrough file in a skipped directory", "output:\n  passthrough: [.well-known/]\n", []string{".well-known/caf\xe9"}, `.well-known/caf\xe9` + notUTF8},
		{"several bytes", "", []string{"a\xff\xfeb.md"}, `a\xff\xfeb.md` + notUTF8},
		{"directory", "", []string{"caf\xe9/post.md"},
			`caf\xe9/post.md: the name of the directory "caf\xe9" is not valid UTF-8; please rename it`},
		{"directory and file", "", []string{"caf\xe9/caf\xe9.md"},
			`caf\xe9/caf\xe9.md: the name of the directory "caf\xe9" is not valid UTF-8; please rename it`},
	})
	// Files that Gondola would ignore anyway are left alone. A real U+FFFD
	// in a name is valid UTF-8.
	runScanTests(t, []scanTest{
		{"ignored files", "", []string{"caf\xe9.txt", "caf\xe9.jet", "_drafts/caf\xe9.md", ".git/caf\xe9.md", "caf\xe9/notes.txt", "a\uFFFD.md"},
			[]string{"markdown a\uFFFD.md as a\uFFFD.md"}},
	})
}

func TestEscapeInvalidUTF8(t *testing.T) {
	tests := []struct{ text, want string }{
		{"", ""},
		{"café/città.md", "café/città.md"},
		{"caf\xe9.md", `caf\xe9.md`},
		{"\xff", `\xff`},
		{"a\xe2\x82b", `a\xe2\x82b`}, // the start of "€" without its last byte
		{"\uFFFD", "\uFFFD"},
		{`a"b\c`, `a"b\c`},
	}
	for _, test := range tests {
		if got := escapeInvalidUTF8(test.text); got != test.want {
			t.Errorf("escapeInvalidUTF8(%q) = %q, want %q", test.text, got, test.want)
		}
	}
}
