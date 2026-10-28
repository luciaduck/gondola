package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/frontmatter"
)

func TestNewTemplateDirs(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		dirs      config.DirList
		positions []config.Position
		want      string // "" when New succeeds
	}{
		{"default exists", map[string]string{defaultTemplateDir + "/page.jet": ""}, defaultDirs(), nil, ""},
		{"default missing is skipped", nil, defaultDirs(), nil, ""},
		{"no template directories", nil, config.DirList{Dirs: []string{}}, nil, ""},
		{"written exists", map[string]string{"layouts/page.jet": ""}, config.DirList{Dirs: []string{"layouts"}}, []config.Position{{Line: 6, Col: 14}}, ""},
		{"written missing", nil, config.DirList{Dirs: []string{"layouts"}}, []config.Position{{Line: 6, Col: 14}},
			`gondola.yaml:6:14: input.templates "layouts" does not exist`},
		{"second written missing", map[string]string{"a/x.jet": ""}, config.DirList{Dirs: []string{"a", "b"}}, []config.Position{{Line: 6, Col: 15}, {Line: 6, Col: 18}},
			`gondola.yaml:6:18: input.templates "b" does not exist`},
		{"written is a file", map[string]string{"layouts": "x"}, config.DirList{Dirs: []string{"layouts"}}, []config.Position{{Line: 6, Col: 14}},
			`gondola.yaml:6:14: input.templates "layouts" is a file, not a directory`},
		{"default is a file", map[string]string{defaultTemplateDir: "x"}, defaultDirs(), []config.Position{{}},
			`gondola.yaml: input.templates "_components/includes" (the default) is a file, not a directory`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			writeFiles(t, projectDir, test.files)
			_, err := New(Options{ProjectDir: projectDir, TemplateDirs: test.dirs, TemplateDirPositions: test.positions})
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

func TestNewRefusesLinkedTemplateDir(t *testing.T) {
	projectDir := t.TempDir()
	writeFiles(t, projectDir, map[string]string{"real/page.jet": ""})
	if err := os.Symlink(filepath.Join(projectDir, "real"), filepath.Join(projectDir, "layouts")); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	_, err := New(Options{ProjectDir: projectDir, TemplateDirs: config.DirList{Dirs: []string{"layouts"}}, TemplateDirPositions: []config.Position{{Line: 6, Col: 14}}})
	want := "layouts is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %s", err, want)
	}
}

// chainFiles names the files of a layout chain.
func chainFiles(chain []*Layout) string {
	files := []string{}
	for _, layout := range chain {
		files = append(files, layout.File)
	}
	return strings.Join(files, " ")
}

func TestLayoutChain(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	tests := []struct {
		name   string
		files  map[string]string
		layout string
		want   string // the files of the chain, nearest first
	}{
		{"nil means no layout", map[string]string{dir + "nil.jet": ""}, "nil", ""},
		{"empty means no layout", nil, "", ""},
		{"one layout without front matter", map[string]string{dir + "page.jet": "{{ copy|raw }}"}, "page", dir + "page.jet"},
		{"parent", map[string]string{dir + "page.jet": "---\nlayout: base\n---\nP", dir + "base.jet": "B"}, "page", dir + "page.jet " + dir + "base.jet"},
		{"three levels", map[string]string{dir + "a.jet": "---\nlayout: b\n---\n", dir + "b.jet": "---\nlayout: c\n---\n", dir + "c.jet": ""}, "a", dir + "a.jet " + dir + "b.jet " + dir + "c.jet"},
		{"parent nil ends the chain", map[string]string{dir + "page.jet": "---\nlayout: nil\n---\n", dir + "nil.jet": ""}, "page", dir + "page.jet"},
		{"parent empty ends the chain", map[string]string{dir + "page.jet": "---\nlayout:\n---\n"}, "page", dir + "page.jet"},
		{"Nil is a layout name", map[string]string{dir + "Nil.jet": ""}, "Nil", dir + "Nil.jet"},
		{"html layout", map[string]string{dir + "page.html": ""}, "page", dir + "page.html"},
		{".jet comes before .html", map[string]string{dir + "page.html": "", dir + "page.jet": ""}, "page", dir + "page.jet"},
		{"name with its extension", map[string]string{dir + "page.html": "", dir + "page.jet": ""}, "page.html", dir + "page.html"},
		{"a file without extension is not a layout", map[string]string{dir + "page": "", dir + "page.html": ""}, "page", dir + "page.html"},
		{"subdirectory", map[string]string{dir + "blog/post.jet": "---\nlayout: base\n---\n", dir + "base.jet": ""}, "blog/post", dir + "blog/post.jet " + dir + "base.jet"},
		{"a directory is not a layout", map[string]string{dir + "page.jet/x": "", dir + "page.html": ""}, "page", dir + "page.html"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := newProject(t, test.files)
			chain, err := engine.LayoutChain("blog/post.md", test.layout)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := chainFiles(chain); got != test.want {
				t.Errorf("chain:\n got %q\nwant %q", got, test.want)
			}
		})
	}
}

func TestLayoutChainSearchesDirectoriesInOrder(t *testing.T) {
	files := map[string]string{
		"first/page.html":  "",
		"second/page.jet":  "",
		"second/base.html": "",
		"first/only.jet":   "",
	}
	options := Options{TemplateDirs: config.DirList{Dirs: []string{"first", "second"}}}
	tests := []struct {
		layout string
		want   string
	}{
		// page.jet in any directory comes before page.html, as for include.
		{"page", "second/page.jet"},
		{"page.html", "first/page.html"},
		{"base", "second/base.html"},
		{"only", "first/only.jet"},
	}
	engine := newProjectWith(t, files, options)
	for _, test := range tests {
		chain, err := engine.LayoutChain("index.md", test.layout)
		if err != nil {
			t.Fatalf("%s: %v", test.layout, err)
		}
		if got := chainFiles(chain); got != test.want {
			t.Errorf("%s: got %q, want %q", test.layout, got, test.want)
		}
	}
}

func TestLayoutChainErrors(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	tests := []struct {
		name   string
		files  map[string]string
		layout string
		want   string
	}{
		{"not found", map[string]string{dir + "base.jet": ""}, "page",
			`blog/post.md: layout "page" was not found; looked for page.jet and page.html in _components/includes (the default)`},
		{"not found with extension", map[string]string{dir + "page.jet": ""}, "page.html",
			`blog/post.md: layout "page.html" was not found; looked for page.html in _components/includes (the default)`},
		{"parent not found", map[string]string{dir + "page.jet": "---\nlayout: base\n---\n"}, "page",
			`_components/includes/page.jet: layout "base" was not found; looked for base.jet and base.html in _components/includes (the default) (while rendering blog/post.md)`},
		{"default directory missing", nil, "page",
			`blog/post.md: layout "page" was not found, because the project has no template directory; please create _components/includes or set input.templates in gondola.yaml`},
		{"cycle with itself", map[string]string{dir + "page.jet": "---\nlayout: page\n---\n"}, "page",
			`blog/post.md: the layouts form a cycle: page.jet → page.jet; please remove one of the layout keys`},
		{"cycle of two", map[string]string{dir + "page.jet": "---\nlayout: base\n---\n", dir + "base.jet": "---\nlayout: page.jet\n---\n"}, "page",
			`blog/post.md: the layouts form a cycle: page.jet → base.jet → page.jet; please remove one of the layout keys`},
		{"cycle below the first layout", map[string]string{dir + "a.jet": "---\nlayout: b\n---\n", dir + "b.jet": "---\nlayout: c\n---\n", dir + "c.jet": "---\nlayout: b\n---\n"}, "a",
			`blog/post.md: the layouts form a cycle: b.jet → c.jet → b.jet; please remove one of the layout keys`},
		{"name escaping the directories", nil, "../secret",
			`blog/post.md: layout "../secret" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"parent name escaping", map[string]string{dir + "page.jet": "---\nlayout: ../x\n---\n"}, "page",
			`_components/includes/page.jet:2:9: layout "../x" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"absolute name", nil, "/etc/passwd",
			`blog/post.md: layout "/etc/passwd" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"malformed front matter", map[string]string{dir + "page.jet": "---\ntitle: [\n---\n"}, "page",
			`_components/includes/page.jet:2:1: invalid YAML: did not find expected node content`},
		{"layout saved as UTF-16", map[string]string{dir + "page.jet": "\xff\xfex\x00"}, "page",
			`_components/includes/page.jet: the file is saved as UTF-16; please save it as UTF-8`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := newProject(t, test.files)
			_, err := engine.LayoutChain("blog/post.md", test.layout)
			checkDiagError(t, err, test.want)
		})
	}
}

func TestLayoutChainWithoutTemplateDirectories(t *testing.T) {
	engine := newProjectWith(t, nil, Options{TemplateDirs: config.DirList{Dirs: []string{}}})
	_, err := engine.LayoutChain("index.md", "page")
	want := `index.md: layout "page" was not found, because the project has no template directory; please set input.templates in gondola.yaml`
	checkDiagError(t, err, want)
}

func TestLayoutChainRefusesLinks(t *testing.T) {
	tests := []struct {
		name string
		link string // a link to real/, relative to the template directory
		want string
	}{
		{"linked layout", "page.jet", "_components/includes/page.jet is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"},
		{"linked directory", "blog", "_components/includes/blog is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			writeFiles(t, projectDir, map[string]string{"real/post.jet": "", "real.jet": "", defaultTemplateDir + "/x.jet": ""})
			target := filepath.Join(projectDir, "real")
			if test.link == "page.jet" {
				target = filepath.Join(projectDir, "real.jet")
			}
			if err := os.Symlink(target, filepath.Join(projectDir, defaultTemplateDir, test.link)); err != nil {
				t.Skipf("cannot create a symbolic link: %v", err)
			}
			engine, err := New(Options{ProjectDir: projectDir, TemplateDirs: defaultDirs()})
			if err != nil {
				t.Fatal(err)
			}
			name := "page"
			if test.link == "blog" {
				name = "blog/post"
			}
			_, err = engine.LayoutChain("index.md", name)
			if err == nil || err.Error() != test.want {
				t.Errorf("got %v, want %s", err, test.want)
			}
		})
	}
}

// TestLayoutChainMergesDefaults checks the precedence of SPEC §3.2 with the
// documents that LayoutChain returns: default, farthest layout, …, nearest
// layout, page.
func TestLayoutChainMergesDefaults(t *testing.T) {
	const dir = defaultTemplateDir + "/"
	engine := newProject(t, map[string]string{
		dir + "page.jet": "---\nlayout: base\ntitle: from page.jet\nauthor:\nsection: page\n---\n",
		dir + "base.jet": "---\ntitle: from base.jet\nauthor: base author\nsection: base\nfooter: base footer\n---\n",
	})
	chain, err := engine.LayoutChain("blog/post.md", "page")
	if err != nil {
		t.Fatal(err)
	}
	page, err := frontmatter.Split("blog/post.md", []byte("---\nlayout: page\nsection: post\n---\nbody"))
	if err != nil {
		t.Fatal(err)
	}
	defaults := frontmatter.NewDefaults(map[string]any{"layout": "nil", "title": "default title", "author": "", "footer": "default footer", "extra": "default extra"}, "2006-01-02")
	merged := frontmatter.Merge(defaults, Documents(chain), page)
	want := map[string]string{
		"title":   "from page.jet",
		"author":  "base author", // empty in page.jet, so base.jet's
		"section": "post",
		"footer":  "base footer",
		"extra":   "default extra",
		"layout":  "page", // the page's own, never a layout's parent
	}
	for name, value := range want {
		if got := merged.Variables[name]; got != value {
			t.Errorf("%s: got %v, want %q", name, got, value)
		}
	}
}

// TestLayoutChainReadsEachLayoutOnce checks that a layout is read once per
// build: a change on disk is seen only by the next Jet.
func TestLayoutChainReadsEachLayoutOnce(t *testing.T) {
	projectDir := t.TempDir()
	layoutFile := filepath.Join(projectDir, defaultTemplateDir, "page.jet")
	writeFiles(t, projectDir, map[string]string{defaultTemplateDir + "/page.jet": "---\ntitle: one\n---\n"})
	engine, err := New(Options{ProjectDir: projectDir, TemplateDirs: defaultDirs()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.LayoutChain("a.md", "page"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layoutFile, []byte("---\ntitle: two\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chain, err := engine.LayoutChain("b.md", "page")
	if err != nil {
		t.Fatal(err)
	}
	if got := chain[0].Document.Variables["title"]; got != "one" {
		t.Errorf("same build: got %v, want one", got)
	}
	next, err := New(Options{ProjectDir: projectDir, TemplateDirs: defaultDirs()})
	if err != nil {
		t.Fatal(err)
	}
	chain, err = next.LayoutChain("b.md", "page")
	if err != nil {
		t.Fatal(err)
	}
	if got := chain[0].Document.Variables["title"]; got != "two" {
		t.Errorf("next build: got %v, want two", got)
	}
}
