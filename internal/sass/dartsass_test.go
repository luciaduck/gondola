package sass

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bep/godartsass/v2"
)

// These tests record what godartsass v2.5.0 and Dart Sass do on their own,
// without Gondola's code (SPEC §8, V6). If one fails after an update of
// either, the code of this package that relies on it must be checked.

// startTranspiler starts godartsass with a handler that keeps log events.
func startTranspiler(t *testing.T) (*godartsass.Transpiler, func() []godartsass.LogEvent) {
	t.Helper()
	executablePath := requireSass(t)
	var mutex sync.Mutex
	var events []godartsass.LogEvent
	transpiler, err := godartsass.Start(godartsass.Options{
		DartSassEmbeddedFilename: executablePath,
		LogEventHandler: func(event godartsass.LogEvent) {
			mutex.Lock()
			defer mutex.Unlock()
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { transpiler.Close() })
	return transpiler, func() []godartsass.LogEvent {
		mutex.Lock()
		defer mutex.Unlock()
		return events
	}
}

// V6: a source given as text, with URL set to the file: URL of its file,
// resolves relative imports next to that file without a custom
// ImportResolver, and a file next to it wins over the load paths.
func TestDartSassRelativeImportsFromTextSource(t *testing.T) {
	transpiler, _ := startTranspiler(t)
	projectDir := writeSite(t, map[string]string{
		"assets/_near.scss":   "$v: near;\n",
		"assets/_same.scss":   "$v: next-to-the-file;\n",
		"partials/_same.scss": "$v: load-path;\n",
		"partials/_far.scss":  "$v: far;\n",
	})
	tests := []struct {
		source string
		want   string
	}{
		{"@use 'near';\na { b: near.$v; }", "a {\n  b: near;\n}"},
		{"@use 'same';\na { b: same.$v; }", "a {\n  b: next-to-the-file;\n}"},
		{"@use 'far';\na { b: far.$v; }", "a {\n  b: far;\n}"},
	}
	for _, test := range tests {
		result, err := transpiler.Execute(godartsass.Args{
			Source:       test.source,
			URL:          fileURL(filepath.Join(projectDir, "assets", "main.scss")),
			IncludePaths: []string{filepath.Join(projectDir, "partials")},
		})
		if err != nil {
			t.Fatalf("%q: %v", test.source, err)
		}
		// No trailing line break and no @charset: writeCSS adds the line
		// break that the sass command writes.
		if result.CSS != test.want {
			t.Errorf("%q compiled to %q, want %q", test.source, result.CSS, test.want)
		}
	}
}

// Without a URL, Dart Sass cannot resolve a relative import.
func TestDartSassTextSourceWithoutURL(t *testing.T) {
	transpiler, _ := startTranspiler(t)
	_, err := transpiler.Execute(godartsass.Args{Source: "@use 'near';\n"})
	var sassErr godartsass.SassError
	if !errors.As(err, &sassErr) || sassErr.Message != "Can't find stylesheet to import." {
		t.Errorf("error %v, want Can't find stylesheet to import.", err)
	}
}

// A SassError has an offset and a column from 0, counted in UTF-16 units,
// and no line: sourcePosition computes it from the offset. The offset of a
// file that Dart Sass read itself does not count its byte order mark.
func TestDartSassErrorSpans(t *testing.T) {
	transpiler, _ := startTranspiler(t)
	projectDir := writeSite(t, map[string]string{
		"assets/_emoji.scss": "a {}\n/* 😀 */ b { c: $nope; }\n",
		"assets/_bom.scss":   "\xEF\xBB\xBFa { b: $nope; }\n",
	})
	tests := []struct {
		use        string
		wantOffset int
		wantColumn int
		wantFile   string
	}{
		// "a {}\n" is 5 units, "/* " 3, "😀" 2, " */ b { c: " 11.
		{"emoji", 21, 16, "_emoji.scss"},
		// The mark is not counted: "a { b: " is 7 units.
		{"bom", 7, 7, "_bom.scss"},
	}
	for _, test := range tests {
		_, err := transpiler.Execute(godartsass.Args{
			Source: "@use '" + test.use + "';\n",
			URL:    fileURL(filepath.Join(projectDir, "assets", "main.scss")),
		})
		var sassErr godartsass.SassError
		if !errors.As(err, &sassErr) {
			t.Fatalf("%s: error %v, want a SassError", test.use, err)
		}
		span := sassErr.Span
		if span.Start.Offset != test.wantOffset || span.Start.Column != test.wantColumn || !strings.HasSuffix(span.Url, "/assets/"+test.wantFile) {
			t.Errorf("%s: span at offset %d, column %d, in %s; want %d, %d, in .../assets/%s",
				test.use, span.Start.Offset, span.Start.Column, span.Url, test.wantOffset, test.wantColumn, test.wantFile)
		}
	}
}

// godartsass writes a log event with a position as "<url>:<line>:<col>:
// <text>", line and column from 0, the column in UTF-16 units; @warn has
// no position.
func TestDartSassLogEvents(t *testing.T) {
	transpiler, events := startTranspiler(t)
	projectDir := t.TempDir()
	mainURL := fileURL(filepath.Join(projectDir, "main.scss"))
	_, err := transpiler.Execute(godartsass.Args{
		Source: "@warn 'careful';\n/* 😀 */ @debug 'look';\n",
		URL:    mainURL,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := events()
	if len(got) != 2 {
		t.Fatalf("events %+v, want 2", got)
	}
	if got[0].Type != godartsass.LogEventTypeWarning || got[0].Message != "careful" {
		t.Errorf("@warn event %+v", got[0])
	}
	// "/* " is 3 units, "😀" 2, " */ " 4: column 9 from 0.
	if want := mainURL + ":1:9: look"; got[1].Type != godartsass.LogEventTypeDebug || got[1].Message != want {
		t.Errorf("@debug event %+v, want message %q", got[1], want)
	}
}
