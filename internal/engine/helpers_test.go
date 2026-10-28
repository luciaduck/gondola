package engine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/luciaduck/gondola/internal/collections"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/parser"
)

// defaultTemplateDir is config's default input.templates.
const defaultTemplateDir = "_components/includes"

// writeFiles creates files (paths with "/" relative to dir) with their
// contents.
func writeFiles(t testing.TB, dir string, files map[string]string) {
	t.Helper()
	for name, contents := range files {
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// defaultDirs is input.templates when gondola.yaml leaves it out.
func defaultDirs() config.DirList {
	return config.DirList{Dirs: []string{defaultTemplateDir}, FromDefault: true}
}

// newProject writes files into a new project directory and returns a Jet
// for it with the default template directory and fenced blocks on.
func newProject(t *testing.T, files map[string]string) *Jet {
	t.Helper()
	return newProjectWith(t, files, Options{TemplateDirs: defaultDirs(), FencedBlocks: parser.New(parser.AliasFile{})})
}

// newProjectWith is newProject with options; ProjectDir is filled in.
func newProjectWith(t *testing.T, files map[string]string, options Options) *Jet {
	t.Helper()
	options.ProjectDir = t.TempDir()
	writeFiles(t, options.ProjectDir, files)
	engine, err := New(options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return engine
}

// testGlobals are the shared variables of the render tests.
func testGlobals() Globals {
	settings := config.Settings{SiteName: "Lindos", SiteVersion: "0.10", Description: "Notes & news", CanonicalURL: "https://example.com"}
	data := map[string]any{"siteInfo": map[string]any{"owner": "Lucia", "first-name": "L"}}
	entry := map[string]any{"url": "/a/", "title": "A"}
	pages := collections.Collections{
		Fleet: []map[string]any{entry},
		Tags:  map[string][]map[string]any{"web-dev": {entry}},
	}
	return NewGlobals(settings, data, pages)
}

// testPage is a page at blog/post.md with a few variables.
func testPage() Page {
	return Page{
		SourcePath: "blog/post.md",
		URLPath:    "/blog/post.html",
		Variables:  map[string]any{"title": "Fish & chips", "layout": "page", "tags": []string{"news"}},
	}
}

// checkDiagError fails unless err is a *diag.Error whose text is want.
func checkDiagError(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %s", want)
	}
	var diagErr *diag.Error
	if !errors.As(err, &diagErr) {
		t.Errorf("error is a %T, not a *diag.Error: %v", err, err)
	}
	if err.Error() != want {
		t.Errorf("error:\n got %s\nwant %s", err, want)
	}
}
