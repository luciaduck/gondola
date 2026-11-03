package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/parser"
)

// The variables that every page has besides the test page's own, as the
// message about an undefined variable lists them.
const (
	testPageNames       = "collections, data, fleet, layout, page, site, tags, title"
	testPageNamesInWrap = "collections, copy, data, fleet, layout, page, site, tags, title"
)

func TestRenderBody(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	files := map[string]string{
		dir + "snippets/note.md":       "Note for {{ title }}.",
		dir + "snippets/fenced.md":     "::: +note\nInside **{{ site.name }}**\n:::\n",
		dir + "snippets/bom.md":        "\xEF\xBB\xBFno mark",
		dir + "header.jet":             "<header>{{ site.name }}</header>",
		dir + "footer.html":            "<footer>",
		dir + "both.jet":               "both.jet",
		dir + "both.html":              "both.html",
		dir + "parts/a.jet":            `A{{ include "b.jet" }}{{ include "/header.jet" }}`,
		dir + "parts/b.jet":            "B",
		dir + "macros.jet":             `{{ block card(text) }}<i>{{ text }}</i>{{ end }}`,
		dir + "blog/snippets/note.md":  "the blog's note",
		dir + "unicode/città.md":       "accented",
		"blog/snippets/not-a-template": "content directory",
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"plain text", "Hello", "Hello"},
		{"front matter variable is escaped", "{{ title }}", "Fish &amp; chips"},
		{"raw", "{{ title|raw }}", "Fish & chips"},
		{"site", "{{ site.name }} {{ site.version }} {{ site.description }} {{ site.canonicalUrl }}", "Lindos 0.10 Notes &amp; news https://example.com"},
		{"data", `{{ data.siteInfo.owner }} {{ data.siteInfo["first-name"] }}`, "Lucia L"},
		{"fleet", "{{ range _, entry := fleet }}{{ entry.url }}{{ end }}", "/a/"},
		{"collections", `{{ collections["web-dev"][0].title }}`, "A"},
		{"page", "{{ page.url }} {{ page.canonical }} {{ page.source }} {{ page.meta.title }}", "/blog/post.html https://example.com/blog/post.html blog/post.md Fish &amp; chips"},
		{"tags", "{{ tags[0] }}", "news"},
		{"literal braces", `{{ "{{" }} title }}`, "{{ title }}"},
		{"include from a page starts at the root", `{{ include "snippets/note.md" }}`, "Note for Fish &amp; chips."},
		{"include with a leading slash", `{{ include "/snippets/note.md" }}`, "Note for Fish &amp; chips."},
		{"dot dot stays inside", `{{ include "../../snippets/note.md" }}`, "Note for Fish &amp; chips."},
		{"blog directory of the templates", `{{ include "blog/snippets/note.md" }}`, "the blog's note"},
		{"fenced block in a fragment", `{{ include "snippets/fenced.md" }}`, "<div class=\"note\">\n\nInside **Lindos**\n\n</div>\n"},
		{"byte order mark removed", `{{ include "snippets/bom.md" }}`, "no mark"},
		{"extension jet", `{{ include "header" }}`, "<header>Lindos</header>"},
		{"extension html", `{{ include "footer" }}`, "<footer>"},
		{"jet before html", `{{ include "both" }}`, "both.jet"},
		{"siblings in a template", `{{ include "parts/a.jet" }}`, "AB<header>Lindos</header>"},
		{"import", `{{ import "macros.jet" }}{{ yield card(text="x") }}`, "<i>x</i>"},
		{"unicode name", `{{ include "unicode/città.md" }}`, "accented"},
	}
	engine := newProject(t, files)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := engine.RenderBody(testPage(), testGlobals(), []byte(test.body), 3)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(got) != test.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, test.want)
			}
		})
	}
}

func TestRenderBodyFencedBlocksOff(t *testing.T) {
	files := map[string]string{defaultTemplateDir + "/fenced.md": "::: +note\nx\n:::\n"}
	engine := newProjectWith(t, files, Options{TemplateDirs: defaultDirs()})
	got, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "fenced.md" }}`), 0)
	if err != nil {
		t.Fatal(err)
	}
	if want := "::: +note\nx\n:::\n"; string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderBodyWithAliases(t *testing.T) {
	aliases, err := parser.ParseAliasFile([]byte(":::\\box . aside +box\n"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{defaultTemplateDir + "/box.md": ":::\\box\nx\n:::\n"}
	engine := newProjectWith(t, files, Options{TemplateDirs: defaultDirs(), FencedBlocks: parser.New(aliases)})
	got, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "box.md" }}`), 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), `<aside class="box">`) {
		t.Errorf("got %q, want an aside", got)
	}
}

func TestRenderBodyErrors(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	files := map[string]string{
		dir + "broken.md":       "line 1\n::: +note\nunclosed\n",
		dir + "undefined.jet":   "one\n{{ nope }}",
		dir + "bad-syntax.jet":  "one\ntwo\n{{ if }}",
		dir + "nested.jet":      `{{ include "undefined.jet" }}`,
		dir + "cycle-a.jet":     `{{ extends "cycle-b.jet" }}`,
		dir + "cycle-b.jet":     `{{ extends "cycle-a.jet" }}`,
		dir + "self-import.jet": `{{ import "self-import.jet" }}`,
		// The name differs from the one Jet opened it under, but it is the
		// same file.
		dir + "self-import-dots.jet": `{{ import "/a/../self-import-dots.jet" }}`,
		dir + "utf16.jet":            "\xff\xfex\x00",
	}
	tests := []struct {
		name string
		body string
		want string
	}{
		{"undefined variable in the page", "line 1\n\n{{ titel }}",
			`blog/post.md:6: variable "titel" is not defined; the available variables are ` + testPageNames},
		{"copy is not defined in the first round", "{{ copy }}",
			`blog/post.md:4: variable "copy" is not defined; the available variables are ` + testPageNames},
		{"syntax error in the page", "x\n{{ if }}",
			`blog/post.md:5: parsing if: unexpected token '}}' (expected term)`},
		{"error without a line", "{{ 1/0 }}",
			`blog/post.md: division by zero`},
		{"error without a position", `{{ len(collections["missing"]) }}`,
			`blog/post.md: len(): argument is not a valid value`},
		// Jet prints the address of its own variable here; it is removed so
		// that the message is the same on every run.
		{"message with an address", `{{ ints("") }}`,
			`blog/post.md: could not parse  (string) into *int64`},
		{"undefined variable in an include", `{{ include "undefined.jet" }}`,
			`_components/includes/undefined.jet:2: variable "nope" is not defined; the available variables are ` + testPageNames + ` (while rendering blog/post.md)`},
		{"error two includes down", "\n{{ include \"nested.jet\" }}",
			`_components/includes/undefined.jet:2: variable "nope" is not defined; the available variables are ` + testPageNames + ` (while rendering blog/post.md)`},
		{"syntax error in an include", `{{ include "bad-syntax.jet" }}`,
			`_components/includes/bad-syntax.jet:3: parsing if: unexpected token '}}' (expected term)` + ` (while rendering blog/post.md)`},
		{"fenced block error in a fragment", `{{ include "broken.md" }}`,
			`_components/includes/broken.md:2:1: unclosed fence: no matching ":::" closing marker (while rendering blog/post.md)`},
		{"include not found", `{{ include "missing.md" }}`,
			`blog/post.md:4: template "missing.md" was not found in _components/includes (the default)`},
		{"include without extension not found", "\n{{ include \"snippets/missing\" }}",
			`blog/post.md:5: template "snippets/missing" was not found; looked for snippets/missing, snippets/missing.jet and snippets/missing.html in _components/includes (the default)`},
		{"a directory is not a template", `{{ include "dir" }}`,
			`blog/post.md:4: template "dir" was not found; looked for dir, dir.jet and dir.html in _components/includes (the default)`},
		{"import not found", `{{ import "missing.jet" }}`,
			`blog/post.md:4: template "missing.jet" was not found in _components/includes (the default)`},
		{"extends cycle", `{{ include "cycle-a.jet" }}`,
			`_components/includes/cycle-b.jet:1: the templates form a cycle: cycle-a.jet → cycle-b.jet → cycle-a.jet; please remove one of the extends or import statements (while rendering blog/post.md)`},
		{"cycle through an absolute name with dot dot", `{{ import "/../../self-import-dots.jet" }}`,
			`_components/includes/self-import-dots.jet:1: the templates form a cycle: self-import-dots.jet → self-import-dots.jet; please remove one of the extends or import statements (while rendering blog/post.md)`},
		{"import of itself", `{{ import "self-import.jet" }}`,
			`_components/includes/self-import.jet:1: the templates form a cycle: self-import.jet → self-import.jet; please remove one of the extends or import statements (while rendering blog/post.md)`},
		{"control character", `{{ include "a` + "\t" + `b" }}`,
			`blog/post.md:4: template name "a\tb" contains a control character (U+0009); please remove it`},
		{"template saved as UTF-16", `{{ include "utf16.jet" }}`,
			`_components/includes/utf16.jet: the file is saved as UTF-16; please save it as UTF-8 (while rendering blog/post.md)`},
	}
	engine := newProject(t, files)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := engine.RenderBody(testPage(), testGlobals(), []byte(test.body), 3)
			checkDiagError(t, err, test.want)
		})
	}
}

// TestRenderBodyBackslashInName checks "\" in an included name (M14, Q5).
// The loader refuses it, but on Windows Jet turns it into "/" before the
// loader sees the name, so there it works as "/" would.
func TestRenderBodyBackslashInName(t *testing.T) {
	engine := newProject(t, map[string]string{
		defaultTemplateDir + "/dir/x.jet": "X",
		defaultTemplateDir + "/a/b.md":    "B",
	})
	tests := []struct {
		name string
		body string
		// want is the error, or on Windows the rendered body.
		want string
	}{
		{"include", `{{ include "dir\\x.jet" }}`,
			`blog/post.md:4: template name "dir\\x.jet" contains "\\"; please separate directories with "/"`},
		{"includeIfExists", `{{ includeIfExists("a\\b.md") }}`,
			`blog/post.md: template name "a\\b.md" contains "\\"; please separate directories with "/"`},
	}
	// Goldmark runs later, on the whole page, so the fragment is still Markdown.
	windowsBodies := map[string]string{"include": "X", "includeIfExists": "B"}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := engine.RenderBody(testPage(), testGlobals(), []byte(test.body), 3)
			if runtime.GOOS != "windows" {
				checkDiagError(t, err, test.want)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got, want := string(body), windowsBodies[test.name]; got != want {
				t.Errorf("body %q, want %q", got, want)
			}
		})
	}
}

func TestRenderBodyIncludeWithoutTemplateDirectories(t *testing.T) {
	tests := []struct {
		dirs config.DirList
		want string
	}{
		{defaultDirs(), `blog/post.md:1: template "x.md" was not found, because the project has no template directory; please create _components/includes or set input.templates in gondola.yaml`},
		{config.DirList{Dirs: []string{}}, `blog/post.md:1: template "x.md" was not found, because the project has no template directory; please set input.templates in gondola.yaml`},
	}
	for _, test := range tests {
		engine := newProjectWith(t, nil, Options{TemplateDirs: test.dirs})
		_, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "x.md" }}`), 0)
		checkDiagError(t, err, test.want)
	}
}

// TestRenderBodyNeverReadsContent checks that an include never reaches a
// file outside the template directories, here a page in the content
// directory and a file above the project.
func TestRenderBodyNeverReadsContent(t *testing.T) {
	files := map[string]string{
		"about.md":                        "secret page",
		defaultTemplateDir + "/about.jet": "template",
	}
	engine := newProject(t, files)
	// Jet cleans relative names itself but passes absolute ones as
	// written, so "/../../about.md" reaches the loader with its "..".
	bodies := []string{
		`{{ include "about.md" }}`,
		`{{ include "../../about.md" }}`,
		`{{ include "/../about.md" }}`,
		`{{ include "/../../about.md" }}`,
		`{{ import "/../../about.md" }}`,
		`{{ extends "/../../about.md" }}`,
	}
	for _, body := range bodies {
		_, err := engine.RenderBody(testPage(), testGlobals(), []byte(body), 0)
		if err == nil || !strings.Contains(err.Error(), `template "about.md" was not found`) {
			t.Errorf("%s: got %v, want not found", body, err)
		}
	}
}

func TestRenderBodyRefusesLinkedTemplate(t *testing.T) {
	projectDir := t.TempDir()
	writeFiles(t, projectDir, map[string]string{"outside.md": "secret", defaultTemplateDir + "/x.jet": ""})
	if err := os.Symlink(filepath.Join(projectDir, "outside.md"), filepath.Join(projectDir, defaultTemplateDir, "link.md")); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	engine, err := New(Options{ProjectDir: projectDir, TemplateDirs: defaultDirs()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "link.md" }}`), 0)
	want := "blog/post.md:1: _components/includes/link.md is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"
	checkDiagError(t, err, want)
}

func TestWrapInLayouts(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	files := map[string]string{
		dir + "page.jet":         "---\nlayout: base\n---\n<article>{{ copy|raw }}</article>",
		dir + "base.jet":         "---\nlayout: nil\n---\n<title>{{ title }}</title><body>{{ copy|raw }}</body>",
		dir + "escaped.jet":      "{{ copy }}",
		dir + "nested/inner.jet": `{{ include "part.jet" }}|{{ copy|raw }}`,
		dir + "nested/part.jet":  "nested part",
		dir + "part.jet":         "root part",
	}
	tests := []struct {
		name   string
		layout string
		copy   string
		want   string
	}{
		{"chain of two", "page", "<p>Hi</p>", "<article><p>Hi</p></article>"},
		{"copy is escaped without raw", "escaped", "<p>", "&lt;p&gt;"},
		{"relative include in a layout starts next to it", "nested/inner", "x", "nested part|x"},
	}
	engine := newProject(t, files)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chain, err := engine.LayoutChain("blog/post.md", test.layout)
			if err != nil {
				t.Fatal(err)
			}
			got, err := engine.WrapInLayouts(testPage(), testGlobals(), chain, []byte(test.copy))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.name == "chain of two" {
				test.want = "<title>Fish &amp; chips</title><body>" + test.want + "</body>"
			}
			if string(got) != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestWrapInLayoutsNoLayouts(t *testing.T) {
	engine := newProject(t, nil)
	got, err := engine.WrapInLayouts(testPage(), testGlobals(), nil, []byte("<p>as is</p>"))
	if err != nil || string(got) != "<p>as is</p>" {
		t.Errorf("got %q, %v", got, err)
	}
}

func TestWrapInLayoutsErrors(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	files := map[string]string{
		dir + "undefined.jet": "---\ntitle: layout\n---\n\n{{ titel }}",
		dir + "syntax.jet":    "---\na: 1\n---\n{{ if }}",
		dir + "outer.jet":     "{{ copy|raw }}\n{{ missing }}",
		dir + "inner.jet":     "---\nlayout: outer\n---\n{{ copy|raw }}",
		dir + "fragment.jet":  "---\n---\n{{ include \"broken.md\" }}",
		dir + "broken.md":     "::: +x\n",
		dir + "includes.jet":  "\n{{ include \"broken.jet\" }}",
		dir + "broken.jet":    "{{ nope }}",
		// A layout that is also opened through the loader, verbatim, by
		// another template: errors in each are counted in their own way.
		dir + "both.jet":      "---\na: 1\n---\n{{ nope }}",
		dir + "uses-both.jet": `{{ include "both.jet" }}`,
		// The layout includes its own file, which Jet then reads verbatim,
		// front matter included, with a context: the error is in the
		// verbatim copy, at line 4 of the file, not 4 + 3.
		dir + "self.jet": "---\na: 1\n---\n{{ if . }}{{ nope }}{{ else }}{{ include \"self.jet\" 1 }}{{ end }}",
	}
	tests := []struct {
		layout string
		want   string
	}{
		{"undefined", `_components/includes/undefined.jet:5: variable "titel" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
		{"syntax", `_components/includes/syntax.jet:4: parsing if: unexpected token '}}' (expected term) (while rendering blog/post.md)`},
		{"inner", `_components/includes/outer.jet:2: variable "missing" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
		{"fragment", `_components/includes/broken.md:1:1: unclosed fence: no matching ":::" closing marker (while rendering blog/post.md)`},
		{"includes", `_components/includes/broken.jet:1: variable "nope" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
		{"both", `_components/includes/both.jet:4: variable "nope" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
		{"self", `_components/includes/self.jet:4: variable "nope" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
		{"uses-both", `_components/includes/both.jet:4: variable "nope" is not defined; the available variables are ` + testPageNamesInWrap + ` (while rendering blog/post.md)`},
	}
	engine := newProject(t, files)
	for _, test := range tests {
		t.Run(test.layout, func(t *testing.T) {
			chain, err := engine.LayoutChain("blog/post.md", test.layout)
			if err != nil {
				t.Fatal(err)
			}
			_, err = engine.WrapInLayouts(testPage(), testGlobals(), chain, []byte("x"))
			checkDiagError(t, err, test.want)
		})
	}
}

// TestRenderReportsIgnoredFailures checks that a template that exists but
// cannot be opened is an error even where Jet would ignore it.
func TestRenderReportsIgnoredFailures(t *testing.T) {
	files := map[string]string{
		defaultTemplateDir + "/broken.md": "::: +x\n",
		defaultTemplateDir + "/fine.md":   "fine",
	}
	tests := []struct {
		name string
		body string
		want string // "" when the render succeeds
	}{
		{"includeIfExists of a missing template", `{{ includeIfExists("missing.md") }}`, ""},
		{"includeIfExists of a template", `{{ includeIfExists("fine.md") }}`, ""},
		{"includeIfExists of a broken fragment", `{{ includeIfExists("broken.md") }}`,
			`_components/includes/broken.md:1:1: unclosed fence: no matching ":::" closing marker (while rendering blog/post.md)`},
		{"try around a broken fragment", `{{ try }}{{ include "broken.md" }}{{ end }}`,
			`_components/includes/broken.md:1:1: unclosed fence: no matching ":::" closing marker (while rendering blog/post.md)`},
	}
	engine := newProject(t, files)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := engine.RenderBody(testPage(), testGlobals(), []byte(test.body), 0)
			if test.want == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			checkDiagError(t, err, test.want)
		})
	}
}

// TestRenderErrorDoesNotLeak checks that an error in one render, kept by the
// loader, is not reported again by the next render.
func TestRenderErrorDoesNotLeak(t *testing.T) {
	files := map[string]string{defaultTemplateDir + "/broken.md": "::: +x\n"}
	engine := newProject(t, files)
	if _, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "broken.md" }}`), 0); err == nil {
		t.Fatal("no error for the broken fragment")
	}
	_, err := engine.RenderBody(testPage(), testGlobals(), []byte("{{ nope }}"), 0)
	checkDiagError(t, err, `blog/post.md:1: variable "nope" is not defined; the available variables are `+testPageNames)
}

// TestRenderTriesAgainAfterAnError checks that a template whose first use
// failed is read again by the next page instead of being remembered as
// broken or as missing.
func TestRenderTriesAgainAfterAnError(t *testing.T) {
	files := map[string]string{defaultTemplateDir + "/broken.md": "::: +x\n"}
	engine := newProject(t, files)
	for range 2 {
		_, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include "broken.md" }}`), 0)
		checkDiagError(t, err, "_components/includes/broken.md:1:1: unclosed fence: no matching \":::\" closing marker (while rendering blog/post.md)")
	}
}

// TestRenderBodyRecoversJetPanic checks that a panic inside Jet (see
// TestJetPanicsOnSignedIncludeName) is an error naming the page.
func TestRenderBodyRecoversJetPanic(t *testing.T) {
	engine := newProject(t, nil)
	_, err := engine.RenderBody(testPage(), testGlobals(), []byte(`{{ include +"x" }}`), 0)
	want := "blog/post.md: Jet stopped unexpectedly (runtime error: invalid memory address or nil pointer dereference); this is a bug in Jet or in Gondola, please report it"
	checkDiagError(t, err, want)
	// The next render works: Jet's state was not left broken.
	got, err := engine.RenderBody(testPage(), testGlobals(), []byte("{{ title }}"), 0)
	if err != nil || string(got) != "Fish &amp; chips" {
		t.Errorf("next render: %q, %v", got, err)
	}
}

func TestRecoverPanic(t *testing.T) {
	engine := newProject(t, nil)
	run := engine.newRun(testPage(), newVariables(testGlobals(), testPage()))
	run.addSource(pageTemplateName, "blog/post.md", 0)
	err := func() (err error) {
		defer run.recoverPanic(pageTemplateName, &err)
		var values map[string]int
		values["x"] = 1 // a Go runtime error: assignment to a nil map
		return nil
	}()
	want := "blog/post.md: Jet stopped unexpectedly (assignment to entry in nil map); this is a bug in Jet or in Gondola, please report it"
	checkDiagError(t, err, want)
}
