package engine

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/collections"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/gondola/internal/parser"
)

// update rewrites the golden files instead of comparing with them:
// go test ./internal/engine -run TestGolden -update. Read the diff of the
// rewritten files before keeping them.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden/want")

// The golden project: pages under pagesDir, templates in the default
// template directory next to it, and the expected pages under wantDir with
// the same paths and ".html".
const (
	goldenProjectDir = "testdata/golden"
	goldenPagesDir   = "pages"
	goldenWantDir    = "want"
)

func TestGolden(t *testing.T) {
	engine, err := New(Options{ProjectDir: goldenProjectDir, TemplateDirs: defaultDirs(), FencedBlocks: parser.New(parser.AliasFile{})})
	if err != nil {
		t.Fatal(err)
	}
	pageFiles := goldenPageFiles(t)
	defaults := frontmatter.NewDefaults(map[string]any{
		"layout": "page", "title": "", "author": "", "date": "", "permalink": "path", "section": "Site",
	}, "2006-01-02")
	// Prepare every page first, as build does (SPEC §3.1), because
	// templates list the other pages.
	type preparedPage struct {
		document frontmatter.Document
		chain    []*Layout
		page     Page
	}
	prepared := []preparedPage{}
	collectionPages := []collections.Page{}
	for _, file := range pageFiles {
		src, err := os.ReadFile(filepath.Join(goldenProjectDir, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		document, err := frontmatter.Split(file, src)
		if err != nil {
			t.Fatal(err)
		}
		layout, _ := document.Variables["layout"].(string)
		if layout == "" {
			layout = "page"
		}
		chain, err := engine.LayoutChain(file, layout)
		if err != nil {
			t.Fatal(err)
		}
		merged := frontmatter.Merge(defaults, Documents(chain), document)
		urlPath := "/" + strings.TrimSuffix(strings.TrimPrefix(file, goldenPagesDir+"/"), path.Ext(file)) + ".html"
		page := Page{SourcePath: file, URLPath: urlPath, Variables: merged.Variables}
		prepared = append(prepared, preparedPage{document: document, chain: chain, page: page})
		collectionPages = append(collectionPages, collections.Page{SourcePath: file, URLPath: urlPath, Merged: merged})
	}
	settings := config.Settings{SiteName: "Lindos", SiteVersion: "1.0", CanonicalURL: "https://example.com"}
	data := map[string]any{"siteInfo": map[string]any{"owner": "Lucia", "since": 2024}}
	globals := NewGlobals(settings, data, collections.Build(collectionPages, settings.CanonicalURL))
	markdown := parser.New(parser.AliasFile{})
	for _, page := range prepared {
		file := page.page.SourcePath
		t.Run(file, func(t *testing.T) {
			got := renderGoldenPage(t, engine, markdown, globals, page.page, page.document, page.chain)
			wantPath := filepath.Join(goldenProjectDir, goldenWantDir, filepath.FromSlash(strings.TrimPrefix(page.page.URLPath, "/")))
			if *update {
				if err := os.MkdirAll(filepath.Dir(wantPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(wantPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(wantPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("page differs from %s:\n got:\n%s\nwant:\n%s", wantPath, got, want)
			}
		})
	}
}

// goldenPageFiles returns the pages of the golden project, relative to it
// with "/", sorted.
func goldenPageFiles(t *testing.T) []string {
	t.Helper()
	files := []string{}
	// fs.WalkDir visits the entries in lexical order, so the list is stable.
	err := fs.WalkDir(os.DirFS(goldenProjectDir), goldenPagesDir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			files = append(files, name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no pages in %s", goldenProjectDir)
	}
	return files
}

// renderGoldenPage runs the steps of SPEC §3.2 on one page: Squero, the
// first Jet round, Goldmark and the layouts, skipping Jet for layout: nil.
func renderGoldenPage(t *testing.T, engine *Jet, markdown *parser.Parser, globals Globals, page Page, document frontmatter.Document, chain []*Layout) []byte {
	t.Helper()
	isMarkdown := path.Ext(page.SourcePath) == ".md"
	usesJet := page.Variables["layout"] != frontmatter.NoLayout
	body := document.Body
	var err error
	if isMarkdown {
		if body, err = markdown.Squero(page.SourcePath, body, document.BodyLineOffset); err != nil {
			t.Fatalf("Squero: %v", err)
		}
	}
	if usesJet {
		if body, err = engine.RenderBody(page, globals, body, document.BodyLineOffset); err != nil {
			t.Fatalf("RenderBody: %v", err)
		}
	}
	if isMarkdown {
		if body, err = markdown.Markdown(body); err != nil {
			t.Fatalf("Markdown: %v", err)
		}
	}
	if !usesJet {
		return body
	}
	result, err := engine.WrapInLayouts(page, globals, chain, body)
	if err != nil {
		t.Fatalf("WrapInLayouts: %v", err)
	}
	return result
}
