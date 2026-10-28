package data

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// FuzzParseFile checks that parsing a data file never panics, and that every
// error is a *diag.Error naming the file whose position, when it has one,
// is a line of the file and a column of that line or just past its end, so
// the console can print it. Positions do not count the byte order mark.
func FuzzParseFile(f *testing.F) {
	seeds := []struct {
		src    string
		isJSON bool
	}{
		{`{"a": 1, "b": [1.5, true, null, "x"]}`, true},
		{`{"a": x}`, true},
		{"{\n  \"a\": 1,\n}\n", true},
		{`{"a": 1`, true},
		{"{\"a\": 1\n", true},
		{`{"a": 1e999}`, true},
		{"[1,\n -1e400, 2]", true},
		{"{\"a\": \xc3\xa9}", true},
		{"\xEF\xBB\xBF{x}", true},
		{"\xEF\xBB\xBF", true},
		{"\xc2\xa0", true},
		{"\xFF\xFE{\x00", true},
		{"{\"a\": \"caf\xe9\"}", true},
		{"", true},
		{"null", true},
		{"99999999999999999999", true},
		{"a: 1\nb: [x, y]\n", false},
		{"a: 1\na: 2\n", false},
		{"? [a]\n: 1\n", false},
		{"a: 1\n---\nb: 2\n", false},
		{"a: 1\nb: [\n", false},
		{"a:\n\tb: 1\n", false},
		{"\xEF\xBB\xBFa: 1\na: 2\n", false},
		{"a: 1\rb: 2\ra: 3\r", false},
		{"!x\n", false},
		{"", false},
	}
	for _, seed := range seeds {
		f.Add([]byte(seed.src), seed.isJSON)
	}
	f.Fuzz(func(t *testing.T, src []byte, isJSON bool) {
		sourcePath := "_data/x.yaml"
		if isJSON {
			sourcePath = "_data/x.json"
		}
		parse, _ := parserFor(sourcePath)
		_, err := parseFile(sourcePath, src, parse)
		if err == nil {
			return
		}
		var diagErr *diag.Error
		if !errors.As(err, &diagErr) {
			// A read of an already checked JSON file is the only other
			// error, and it must never happen.
			t.Fatalf("error is not a *diag.Error: %v", err)
		}
		if diagErr.File != sourcePath {
			t.Fatalf("error names %q, want %q: %v", diagErr.File, sourcePath, err)
		}
		if diagErr.Line == 0 {
			return
		}
		lines := bytes.Split(sourcetext.TrimByteOrderMark(src), []byte("\n"))
		if diagErr.Line > len(lines) {
			t.Fatalf("line %d is past the last line (%d): %v", diagErr.Line, len(lines), err)
		}
		if lineLength := len(lines[diagErr.Line-1]); diagErr.Col < 1 || diagErr.Col > lineLength+1 {
			t.Fatalf("column %d is outside line %d (%d bytes): %v", diagErr.Col, diagErr.Line, lineLength, err)
		}
	})
}
