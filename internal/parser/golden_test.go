package parser

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/frontmatter"
)

// update rewrites the golden files instead of comparing with them:
// go test ./internal/parser -run TestGolden -update. Read the diff of the
// rewritten files before keeping them.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// goldenDir holds pairs of files: a page (name.md) and the HTML that Squero
// and Goldmark make of its body (name.html), with the aliases of the
// alias.squero file in the same directory.
const goldenDir = "testdata/golden"

func TestGolden(t *testing.T) {
	aliasFile, err := LoadAliasFile(goldenDir)
	if err != nil {
		t.Fatalf("LoadAliasFile: %v", err)
	}
	p := New(aliasFile)
	// filepath.Glob returns the names sorted, so the order is stable.
	pages, err := filepath.Glob(filepath.Join(goldenDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatalf("no pages in %s", goldenDir)
	}
	for _, pagePath := range pages {
		name := strings.TrimSuffix(filepath.Base(pagePath), ".md")
		t.Run(name, func(t *testing.T) {
			got := renderGoldenPage(t, p, pagePath)
			htmlPath := filepath.Join(goldenDir, name+".html")
			if *update {
				if err := os.WriteFile(htmlPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("HTML differs from %s:\n got:\n%s\nwant:\n%s", htmlPath, got, want)
			}
		})
	}
}

// renderGoldenPage runs the steps of SPEC §3.2 that belong to the parser on
// the page at pagePath: split, Squero, Goldmark.
func renderGoldenPage(t *testing.T, p *Parser, pagePath string) []byte {
	t.Helper()
	src, err := os.ReadFile(pagePath)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.ToSlash(pagePath)
	document, err := frontmatter.Split(file, src)
	if err != nil {
		t.Fatalf("frontmatter.Split: %v", err)
	}
	fenced, err := p.Squero(file, document.Body, document.BodyLineOffset)
	if err != nil {
		t.Fatalf("Squero: %v", err)
	}
	html, err := p.Markdown(fenced)
	if err != nil {
		t.Fatalf("Markdown: %v", err)
	}
	return html
}
