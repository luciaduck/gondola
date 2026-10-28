package engine

// These tests record how jet/v6 v6.3.3 itself behaves, without the engine,
// where Gondola relies on it (SPEC §8: V2, V3, V4) or reads its error texts.
// If an upgrade of Jet changes one of them, the test says which assumption
// broke.

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/CloudyKit/jet/v6"
)

// memoryLoader serves templates from a map and records the paths that Jet
// asked about.
type memoryLoader struct {
	files map[string]string
	asked []string
}

func (m *memoryLoader) Exists(templatePath string) bool {
	m.asked = append(m.asked, templatePath)
	_, ok := m.files[templatePath]
	return ok
}

func (m *memoryLoader) Open(templatePath string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.files[templatePath])), nil
}

// runJet parses src under name and executes it with vars.
func runJet(set *jet.Set, name, src string, vars jet.VarMap) (string, error) {
	template, err := set.Parse(name, src)
	if err != nil {
		return "", err
	}
	var buffer bytes.Buffer
	err = template.Execute(&buffer, vars, nil)
	return buffer.String(), err
}

func jetVariables() jet.VarMap {
	vars := jet.VarMap{}
	vars.Set("title", "A & B")
	vars.Set("social", map[string]any{"name": "Lu", "first-name": "L", "nested": map[string]any{"k": 1}})
	vars.Set("fleet", []map[string]any{{"url": "/a/"}, {"url": "/b/"}})
	vars.Set("collections", map[string][]map[string]any{"web-dev": {{"url": "/w/"}}})
	return vars
}

// TestJetReadsNestedMaps is V2: templates read the maps that front matter,
// data files and collections hold.
func TestJetReadsNestedMaps(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`{{ social.name }}`, "Lu"},
		{`{{ social["first-name"] }}`, "L"},
		{`{{ social.nested.k }}`, "1"},
		// A key that the map does not hold is empty, without an error.
		{`{{ social.missing }}`, ""},
		{`{{ collections["web-dev"][0].url }}`, "/w/"},
		{`{{ len(fleet) }}`, "2"},
		// With one variable, range gives the index; the entry needs two.
		{`{{ range i := fleet }}{{ i }},{{ end }}`, "0,1,"},
		{`{{ range _, entry := fleet }}{{ entry.url }},{{ end }}`, "/a/,/b/,"},
		{`{{ range fleet }}{{ .url }},{{ end }}`, "/a/,/b/,"},
		// A tag that no page has is not in the map: reading it is empty.
		{`{{ collections["missing"] }}`, ""},
		{`{{ isset(collections["missing"]) }}`, "false"},
	}
	set := jet.NewSet(&memoryLoader{})
	for _, test := range tests {
		got, err := runJet(set, "/p", test.src, jetVariables())
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v; want %q", test.src, got, err, test.want)
		}
	}
}

// TestJetMissingCollection records that a template cannot loop over or
// count a tag that no page has: it must check with isset first.
func TestJetMissingCollection(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`{{ range _, e := collections["missing"] }}x{{ end }}`, `Jet Runtime Error ("/p":1): can't range over invalid value`},
		// This error has no position at all.
		{`{{ len(collections["missing"]) }}`, `len(): argument is not a valid value`},
	}
	set := jet.NewSet(&memoryLoader{})
	for _, test := range tests {
		_, err := runJet(set, "/p", test.src, jetVariables())
		if err == nil || err.Error() != test.want {
			t.Errorf("%s: got %v, want %s", test.src, err, test.want)
		}
	}
}

// TestJetLiteralBraces is V4: a literal "{{" is written {{ "{{" }}, also in
// a Markdown code span; "}}" outside an action is text. "{*" starts a Jet
// comment, which a Markdown text could contain by accident.
func TestJetLiteralBraces(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`{{ "{{" }}`, "{{"},
		{`{{ "}}" }}`, "}}"},
		{"`{{ \"{{\" }} name }}`", "`{{ name }}`"},
		{`a }} b`, "a }} b"},
		{`{* a comment *}text`, "text"},
	}
	set := jet.NewSet(&memoryLoader{})
	for _, test := range tests {
		got, err := runJet(set, "/p", test.src, nil)
		if err != nil || got != test.want {
			t.Errorf("%s: got %q, %v; want %q", test.src, got, err, test.want)
		}
	}
}

// TestJetResolvesNames is V3: Jet makes an include name absolute against
// the directory of the template that holds it, cleans it (so ".." cannot
// leave its root), then tries the extensions in order.
func TestJetResolvesNames(t *testing.T) {
	tests := []struct {
		name      string // the name the including template is parsed under
		src       string
		wantAsked []string
	}{
		{"/blog/post.md", `{{ include "snippets/x.md" }}`, []string{"/blog/snippets/x.md", "/blog/snippets/x.md.jet", "/blog/snippets/x.md.html"}},
		{"/post.md", `{{ include "snippets/x.md" }}`, []string{"/snippets/x.md"}},
		{"/blog/post.md", `{{ include "/snippets/x.md" }}`, []string{"/snippets/x.md"}},
		{"/blog/post.md", `{{ include "../../../snippets/x.md" }}`, []string{"/snippets/x.md"}},
		{"/blog/post.md", `{{ include "./../snippets//x.md" }}`, []string{"/snippets/x.md"}},
		{"/post.md", `{{ include "header" }}`, []string{"/header", "/header.jet", "/header.html"}},
		// An absolute name is passed as written, ".." included: the loader
		// must clean it.
		{"/blog/post.md", `{{ include "/../x.md" }}`, []string{"/../x.md", "/../x.md.jet", "/../x.md.html"}},
		// "\" is not a separator on Linux; on Windows Jet turns it into "/".
		{"/post.md", `{{ include "a\\x.md" }}`, []string{"/a\\x.md", "/a\\x.md.jet", "/a\\x.md.html"}},
	}
	for _, test := range tests {
		loader := &memoryLoader{files: map[string]string{"/snippets/x.md": "X", "/header.html": "H"}}
		set := jet.NewSet(loader, jet.WithTemplateNameExtensions(templateExtensions))
		_, _ = runJet(set, test.name, test.src, nil)
		if strings.Join(loader.asked, " ") != strings.Join(test.wantAsked, " ") {
			t.Errorf("%s in %s: asked %q, want %q", test.src, test.name, loader.asked, test.wantAsked)
		}
	}
}

// TestJetIncludesSiblings records that a template opened through the loader
// resolves its own relative names against its own directory.
func TestJetIncludesSiblings(t *testing.T) {
	loader := &memoryLoader{files: map[string]string{"/layouts/a.jet": `A{{ include "b.jet" }}`, "/layouts/b.jet": "B"}}
	set := jet.NewSet(loader)
	got, err := runJet(set, "/p", `{{ include "layouts/a.jet" }}`, nil)
	if err != nil || got != "AB" {
		t.Errorf("got %q, %v; want %q", got, err, "AB")
	}
}

// TestJetErrorTexts records the error texts that the engine reads
// (errors.go).
func TestJetErrorTexts(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"line 1\n\n{{ titel }}", `Jet Runtime Error ("/p":3): identifier "titel" not available in current (map[title:A & B]) or parent scope, global, or default variables`},
		{`{{ if }}`, `template: /p:1: parsing if: unexpected token '}}' (expected term)`},
		{`{{ include "nope.jet" }}`, `Jet Runtime Error ("/p":1): template /nope.jet could not be found`},
		{`{{ import "nope.jet" }}`, `template: /p:1: template /nope.jet could not be found`},
		{`{{ include "bad.jet" }}`, `Jet Runtime Error ("/p":1): template: /bad.jet:2: unexpected {{end}}`},
		// Jet reports line 0 for a division by zero.
		{`{{ 1/0 }}`, `Jet Runtime Error ("/p":0): division by zero`},
	}
	loader := &memoryLoader{files: map[string]string{"/bad.jet": "x\n{{ end }}"}}
	set := jet.NewSet(loader)
	for _, test := range tests {
		vars := jet.VarMap{}
		vars.Set("title", "A & B")
		_, err := runJet(set, "/p", test.src, vars)
		if err == nil || err.Error() != test.want {
			t.Errorf("%q:\n got %v\nwant %s", test.src, err, test.want)
		}
	}
}

// TestJetIncludeDepth records that Jet stops a template that includes
// itself, with an error. A cycle of extends or import is not stopped by Jet
// (it overflows the stack), which is why the loader detects it.
func TestJetIncludeDepth(t *testing.T) {
	loader := &memoryLoader{files: map[string]string{"/self.jet": `{{ include "self.jet" }}`}}
	set := jet.NewSet(loader)
	_, err := runJet(set, "/p", `{{ include "self.jet" }}`, nil)
	want := `Jet Runtime Error ("/self.jet":1): maximum 'include' depth (100000) exceeded`
	if err == nil || err.Error() != want {
		t.Errorf("got %v, want %s", err, want)
	}
}

// TestJetPanicsOnSignedIncludeName records a bug of Jet found by
// FuzzRenderBody: a "+" or "-" in front of an include name parses, then
// panics with a nil pointer while the template runs. The engine turns the
// panic into an error (TestRenderBodyRecoversJetPanic).
func TestJetPanicsOnSignedIncludeName(t *testing.T) {
	for _, src := range []string{`{{ include +"x" }}`, `{{ include -"x" }}`} {
		template, err := jet.NewSet(&memoryLoader{}).Parse("/p", src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		recovered := func() (recovered any) {
			defer func() { recovered = recover() }()
			_ = template.Execute(io.Discard, nil, nil)
			return nil
		}()
		if recovered == nil {
			t.Errorf("%s: Jet no longer panics; the engine's recover may be kept anyway", src)
		}
	}
}
