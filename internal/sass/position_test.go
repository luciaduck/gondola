package sass

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
	"unicode/utf8"
)

func TestSourcePosition(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		utf16Offset int
		wantLine    int
		wantCol     int
		wantOK      bool
	}{
		{"start", "a { }", 0, 1, 1, true},
		{"first line", "a { b: c }", 4, 1, 5, true},
		{"second line", "a {\n  b: c\n}", 6, 2, 3, true},
		{"line break itself", "a\nb", 1, 1, 2, true},
		{"just after a line break", "a\nb", 2, 2, 1, true},
		{"end of text", "a\nbc", 4, 2, 3, true},
		{"end after a final line break", "a\n", 2, 2, 1, true},
		{"empty text", "", 0, 1, 1, true},
		{"past the end", "abc", 4, 0, 0, false},
		{"negative", "abc", -1, 0, 0, false},
		// "é" is one UTF-16 unit and two bytes.
		{"two-byte character before", "é x", 2, 1, 4, true},
		// "😀" is two UTF-16 units and four bytes.
		{"four-byte character before", "/* 😀 */ $x", 9, 1, 12, true},
		{"emoji on an earlier line", "😀\n$x", 3, 2, 1, true},
		{"CRLF", "a {\r\n  b\r\n}", 7, 2, 3, true},
		// An invalid byte is one unit for Dart Sass (U+FFFD) and one byte.
		{"invalid UTF-8", "\xff x", 2, 1, 3, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			line, col, ok := sourcePosition(test.text, test.utf16Offset)
			if line != test.wantLine || col != test.wantCol || ok != test.wantOK {
				t.Errorf("sourcePosition(%q, %d) = %d, %d, %v; want %d, %d, %v",
					test.text, test.utf16Offset, line, col, ok, test.wantLine, test.wantCol, test.wantOK)
			}
		})
	}
}

func TestLineColumn(t *testing.T) {
	text := "a {}\n/* 😀 */ @debug x;\r\nlast"
	tests := []struct {
		name        string
		lineIndex   int
		utf16Column int
		wantCol     int
		wantOK      bool
	}{
		{"first line", 0, 2, 3, true},
		{"after an emoji", 1, 9, 12, true},
		{"end of a CRLF line", 1, 19, 22, true},
		{"last line", 2, 4, 5, true},
		{"column past the line", 0, 10, 0, false},
		{"line past the text", 3, 0, 0, false},
		{"negative line", -1, 0, 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			col, ok := lineColumn(text, test.lineIndex, test.utf16Column)
			if col != test.wantCol || ok != test.wantOK {
				t.Errorf("lineColumn(%d, %d) = %d, %v; want %d, %v", test.lineIndex, test.utf16Column, col, ok, test.wantCol, test.wantOK)
			}
		})
	}
}

func TestFileURL(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"main.scss", "with space.scss", "100%.scss", "a+b.scss", "café/é.scss", "a#b?.scss"} {
		t.Run(name, func(t *testing.T) {
			filePath := filepath.Join(dir, filepath.FromSlash(name))
			rawURL := fileURL(filePath)
			if !strings.HasPrefix(rawURL, "file:///") {
				t.Errorf("fileURL(%q) = %q, want a file:/// URL", filePath, rawURL)
			}
			back, ok := pathFromFileURL(rawURL)
			if !ok || back != filePath {
				t.Errorf("pathFromFileURL(%q) = %q, %v; want %q", rawURL, back, ok, filePath)
			}
		})
	}
}

func TestPathFromFileURLRefusesOtherURLs(t *testing.T) {
	for _, rawURL := range []string{"", "https://example.com/a.scss", "stdin", "sass:math", "%zz"} {
		if got, ok := pathFromFileURL(rawURL); ok {
			t.Errorf("pathFromFileURL(%q) = %q, true; want false", rawURL, got)
		}
	}
}

// expectedPosition computes what sourcePosition must return in another way:
// it encodes the text in UTF-16, cuts it at the offset, and measures the
// cut text in UTF-8. It works only for valid UTF-8 and for an offset that
// does not split a surrogate pair; ok is false otherwise.
func expectedPosition(text string, utf16Offset int) (line, col int, ok bool) {
	if !utf8.ValidString(text) || utf16Offset < 0 {
		return 0, 0, false
	}
	units := utf16.Encode([]rune(text))
	if utf16Offset > len(units) {
		return 0, 0, false
	}
	if utf16Offset > 0 && utf16Offset < len(units) && isHighSurrogate(units[utf16Offset-1]) {
		return 0, 0, false
	}
	before := string(utf16.Decode(units[:utf16Offset]))
	lastBreak := strings.LastIndex(before, "\n")
	return strings.Count(before, "\n") + 1, len(before) - lastBreak, true
}

// isHighSurrogate reports whether unit is the first half of a surrogate
// pair, which encodes a character outside the Basic Multilingual Plane.
func isHighSurrogate(unit uint16) bool {
	return unit >= 0xD800 && unit <= 0xDBFF
}

func FuzzSourcePosition(f *testing.F) {
	seeds := []struct {
		text   string
		offset int
	}{
		{"", 0},
		{"a { b: c }", 4},
		{"a {\n  b: c\n}", 6},
		{"a {\r\n  b\r\n}", 7},
		{"/* 😀 */ $x", 9},
		{"/* 😀 */ $x", 4},
		{"😀\n😀\n$x", 6},
		{"é\nü\n", 4},
		{"\xff x", 2},
		{"abc", 4},
		{"abc", -1},
		{"line\n\n\nend", 7},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.offset)
	}
	f.Fuzz(func(t *testing.T, text string, utf16Offset int) {
		line, col, ok := sourcePosition(text, utf16Offset)
		if !ok {
			if line != 0 || col != 0 {
				t.Fatalf("not ok but returned %d, %d", line, col)
			}
			if wantLine, wantCol, wantOK := expectedPosition(text, utf16Offset); wantOK {
				t.Fatalf("sourcePosition(%q, %d) not ok; want %d, %d", text, utf16Offset, wantLine, wantCol)
			}
			return
		}
		if line < 1 || col < 1 {
			t.Fatalf("sourcePosition(%q, %d) = %d, %d", text, utf16Offset, line, col)
		}
		// The position must exist in the text as the console reads it.
		lines := strings.Split(text, "\n")
		if line > len(lines) || col-1 > len(lines[line-1]) {
			t.Fatalf("sourcePosition(%q, %d) = %d, %d, outside the text", text, utf16Offset, line, col)
		}
		if wantLine, wantCol, wantOK := expectedPosition(text, utf16Offset); wantOK && (line != wantLine || col != wantCol) {
			t.Fatalf("sourcePosition(%q, %d) = %d, %d; want %d, %d", text, utf16Offset, line, col, wantLine, wantCol)
		}
	})
}

// canaryEnvironmentVariable turns on FuzzCanarySourcePosition.
const canaryEnvironmentVariable = "GONDOLA_FUZZ_CANARY"

// FuzzCanarySourcePosition IS WRITTEN TO FAIL. It is a canary for the fuzz
// setup, asked for by Lucia in M8: it claims something false, that the byte
// column equals the UTF-16 column, which holds only for ASCII text. Its
// seeds are all ASCII, so the seed replay passes; only the fuzzer can find
// the counterexample, and finding it within a minute shows that fuzzing
// FuzzSourcePosition really explores non-ASCII text.
//
// It is skipped unless GONDOLA_FUZZ_CANARY is set, so that the failing
// input the fuzzer saves under testdata/fuzz does not fail go test:
//
//	GONDOLA_FUZZ_CANARY=1 go test ./internal/sass -run '^$' -fuzz FuzzCanarySourcePosition -fuzztime 1m
func FuzzCanarySourcePosition(f *testing.F) {
	if os.Getenv(canaryEnvironmentVariable) == "" {
		f.Skip("the canary is written to fail; set " + canaryEnvironmentVariable + " to run it")
	}
	f.Add("a { b: c }", 4)
	f.Add("a {\n  b: c\n}", 6)
	f.Fuzz(func(t *testing.T, text string, utf16Offset int) {
		line, col, ok := sourcePosition(text, utf16Offset)
		if !ok || line != 1 {
			return
		}
		// The false claim: one UTF-16 unit is one byte.
		if col != utf16Offset+1 {
			t.Fatalf("canary: sourcePosition(%q, %d) has byte column %d, UTF-16 column %d", text, utf16Offset, col, utf16Offset+1)
		}
	})
}
