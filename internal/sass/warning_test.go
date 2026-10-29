package sass

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEventPosition(t *testing.T) {
	projectDir := writeSite(t, map[string]string{
		"_components/sass/_x.scss": "a {}\n/* 😀 */ @debug 'x';\n",
	})
	compiler := &Compiler{projectDir: projectDir}
	entryFilePath := filepath.Join(projectDir, "main.scss")
	entryText := "\n  @debug 'y';\n"
	partialURL := "file://" + filepath.ToSlash(filepath.Join(projectDir, "_components", "sass", "_x.scss"))
	entryURL := "file://" + filepath.ToSlash(entryFilePath)
	tests := []struct {
		name         string
		message      string
		wantPosition string
		wantMessage  string
	}{
		{"no position", "careful", "", "careful"},
		{"entry point", entryURL + ":1:2: y", "main.scss:2:3", "y"},
		{"partial after an emoji", partialURL + ":1:9: x", "_components/sass/_x.scss:2:12", "x"},
		{"several lines", entryURL + ":0:0: one\n\ntwo", "main.scss:1:1", "one\n\ntwo"},
		{"column past the line", entryURL + ":1:99: y", "main.scss:2", "y"},
		{"line past the file", entryURL + ":7:0: y", "main.scss:8", "y"},
		{"missing file", "file://" + filepath.ToSlash(filepath.Join(projectDir, "gone.scss")) + ":2:3: z", "gone.scss:3", "z"},
		{"not a file URL", "https://example.com/a.scss:1:1: w", "", "https://example.com/a.scss:1:1: w"},
		{"number too large", entryURL + ":99999999999999999999:1: y", "", entryURL + ":99999999999999999999:1: y"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			position, message := compiler.eventPosition(test.message, entryFilePath, entryText)
			if position != test.wantPosition || message != test.wantMessage {
				t.Errorf("eventPosition(%q) = %q, %q; want %q, %q", test.message, position, message, test.wantPosition, test.wantMessage)
			}
		})
	}
}

func FuzzEventPosition(f *testing.F) {
	projectDir := f.TempDir()
	entryFilePath := filepath.Join(projectDir, "main.scss")
	entryURL := "file://" + filepath.ToSlash(entryFilePath)
	for _, seed := range []string{
		"careful",
		entryURL + ":0:0: x",
		entryURL + ":1:9: look",
		entryURL + ":1:-1: x",
		entryURL + ":3:0: one\n\ntwo",
		"file://:1:1: ",
		"file:///a:b:1:2:3: c",
		"",
	} {
		f.Add(seed, "a {}\n/* 😀 */ @debug 'look';\n")
	}
	f.Fuzz(func(t *testing.T, eventMessage, entryText string) {
		compiler := &Compiler{projectDir: projectDir}
		position, message := compiler.eventPosition(eventMessage, entryFilePath, entryText)
		if position == "" {
			if message != eventMessage {
				t.Fatalf("no position, but the message changed: %q became %q", eventMessage, message)
			}
			return
		}
		if !strings.HasSuffix(eventMessage, message) {
			t.Fatalf("message %q is not the end of %q", message, eventMessage)
		}
		parts := strings.Split(position, ":")
		if len(parts) < 2 {
			t.Fatalf("position %q has no line", position)
		}
	})
}
