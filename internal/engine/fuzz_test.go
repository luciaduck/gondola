package engine

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/parser"
)

// fuzzBodyLineOffset is the number of front matter lines in front of the
// fuzzed body, so that the fuzzer checks the shift of page lines too.
const fuzzBodyLineOffset = 2

// fuzzTemplates are the templates that fuzzed page bodies can include, with
// the errors they hold.
var fuzzTemplates = map[string]string{
	defaultTemplateDir + "/a.jet":            "A {{ title }}\n",
	defaultTemplateDir + "/nested/b.jet":     `B {{ include "../a.jet" }}`,
	defaultTemplateDir + "/fragment.md":      "::: +note\n{{ site.name }}\n:::\n",
	defaultTemplateDir + "/broken.md":        "line 1\n::: +note\n",
	defaultTemplateDir + "/bad-syntax.jet":   "one\n{{ if }}\n",
	defaultTemplateDir + "/undefined.jet":    "one\ntwo\n{{ nope }}\n",
	defaultTemplateDir + "/extends-self.jet": `{{ extends "extends-self.jet" }}`,
	defaultTemplateDir + "/macros.jet":       "{{ block card(text) }}<i>{{ text }}</i>{{ end }}",
}

// FuzzRenderBody renders arbitrary page bodies. It checks that every error
// is a *diag.Error naming the page or a template at a line that the file
// has, that no internal name or Jet position prefix leaks into a message,
// that a panic inside Jet never escapes, and that rendering is
// deterministic. A panic that the engine recovered is an accepted result:
// the first one the fuzzer found is a bug of Jet
// (TestJetPanicsOnSignedIncludeName), which Gondola cannot fix.
func FuzzRenderBody(f *testing.F) {
	seeds := []string{
		"",
		"plain text\n",
		"{{ title }}",
		"{{ title|raw }} & {{ page.url }} {{ site.name }} {{ data.siteInfo.owner }}",
		`{{ range _, entry := fleet }}{{ entry.url }}{{ end }}`,
		`{{ collections["web-dev"][0].title }} {{ collections["missing"] }}`,
		`{{ include "a.jet" }}`,
		`{{ include "nested/b.jet" }}`,
		`{{ include "/../../a.jet" }}`,
		`{{ include "fragment.md" }}`,
		`{{ include "broken.md" }}`,
		`{{ include "bad-syntax.jet" }}`,
		`{{ include "undefined.jet" }}`,
		`{{ include "missing" }}`,
		`{{ import "extends-self.jet" }}`,
		`{{ import "macros.jet" }}{{ yield card(text="x") }}`,
		`{{ includeIfExists("broken.md") }}`,
		`{{ include "a\\b.jet" }}`,
		"line 1\n\n{{ titel }}",
		"{{ if }}",
		"{{ if true }}x{{ else }}y{{ end }}",
		"{{ 1/0 }}",
		`{{ len(collections["missing"]) }}`,
		`{{ "{{" }} title }}`,
		"{* comment *} {{ x := 1 }}{{ x }}",
		"{{ try }}{{ nope }}{{ catch err }}caught{{ end }}",
		"\r\n{{ titel }}\r\n",
		"{{",
		"}}",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	projectDir := f.TempDir()
	writeFiles(f, projectDir, fuzzTemplates)
	fileLines := map[string]int{}
	for name, contents := range fuzzTemplates {
		fileLines[name] = strings.Count(contents, "\n") + 1
	}
	f.Fuzz(func(t *testing.T, body string) {
		first, firstErr := renderFuzzBody(t, projectDir, body)
		second, secondErr := renderFuzzBody(t, projectDir, body)
		if !bytes.Equal(first, second) || errorText(firstErr) != errorText(secondErr) {
			t.Fatalf("two renders differ:\n%q, %v\n%q, %v", first, firstErr, second, secondErr)
		}
		if firstErr == nil {
			return
		}
		checkFuzzError(t, firstErr, body, fileLines)
	})
}

// renderFuzzBody renders body with a new Jet, as a new build would.
func renderFuzzBody(t *testing.T, projectDir, body string) ([]byte, error) {
	t.Helper()
	engine, err := New(Options{ProjectDir: projectDir, TemplateDirs: defaultDirs(), FencedBlocks: parser.New(parser.AliasFile{})})
	if err != nil {
		t.Fatal(err)
	}
	return engine.RenderBody(testPage(), testGlobals(), []byte(body), fuzzBodyLineOffset)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// checkFuzzError checks the shape of an error from rendering body.
func checkFuzzError(t *testing.T, err error, body string, fileLines map[string]int) {
	t.Helper()
	var diagErr *diag.Error
	if !errors.As(err, &diagErr) {
		t.Fatalf("error is a %T, not a *diag.Error: %v", err, err)
	}
	pageLines := fuzzBodyLineOffset + strings.Count(body, "\n") + 1
	maxLine, known := fileLines[diagErr.File]
	if diagErr.File == testPage().SourcePath {
		maxLine, known = pageLines, true
	}
	if !known {
		t.Fatalf("error names an unknown file %q: %v", diagErr.File, err)
	}
	if diagErr.Line < 0 || diagErr.Line > maxLine {
		t.Fatalf("error at line %d, but %s has %d lines: %v", diagErr.Line, diagErr.File, maxLine, err)
	}
	if diagErr.File == testPage().SourcePath && diagErr.Line > 0 && diagErr.Line <= fuzzBodyLineOffset {
		t.Fatalf("error at line %d, inside the front matter: %v", diagErr.Line, err)
	}
	// Jet quotes the offending text of a syntax error, so a leak counts
	// only when the body itself does not hold that text.
	message := err.Error()
	for _, leak := range []string{"/" + internalNameMarker, jetRuntimePrefix} {
		if strings.Contains(message, leak) && !strings.Contains(body, leak) {
			t.Fatalf("message contains %q: %q", leak, message)
		}
	}
}
