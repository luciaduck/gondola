package parser

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// FuzzSquero parses an alias file and a page as a build does, and checks:
// every error is a *diag.Error naming its file, at a line of that file and
// a column of that line (or just past its end); a page without a fence line
// comes out of Squero unchanged; and Goldmark accepts whatever Squero
// returns.
func FuzzSquero(f *testing.F) {
	seeds := []struct{ aliasFile, page string }{
		{"", ""},
		{"", "# Hello\n"},
		{testAliasFile, ":::\\note #first\ntext\n:::\n"},
		{"", ":::\\note\ntext\n:::\n"},
		{"", "---\ntitle: Post\n---\n::: +note ?data-x[abc\n:::\n"},
		{"", "---\r\ntitle: Post\r\n---\r\n::: +note\r\ntext\r\n:::\r\n"},
		{"", byteOrderMark + "::: +note ?x[\n:::\n"},
		{byteOrderMark + ":::\\a . div ?x[\n", "text\n"},
		{":::\\a . div\n:::\\a . p\n", "text\n"},
		{"", ":::: #outer\n::: +inner\n::: ?x[\n:::\n::::\n"},
		{"", "```\n::: ?x[\n```\n::: ?y[\n:::\n"},
		{"", "::://a -- b\n"},
		{"", "::: ?a[\"b]\n:::\n"},
		{"", "::: +note\rtext\r:::\r"},
		{"", ":::span +x /\nclick **me**\n:::\n"},
		{"", "::: ?title[perché] ?x[\n:::\n"},
		{"", "a\n\n::: +note\ntext\n"},
		{"\xFF\xFE:\x00", "text\n"},
		{"", "| a | b |\n|---|---|\n| 1 | 2 |\n\n## Città\n\n<div>\n\n**x**\n\n</div>\n"},
	}
	for _, seed := range seeds {
		f.Add(seed.aliasFile, seed.page)
	}
	f.Fuzz(func(t *testing.T, aliasSrc, page string) {
		aliasFile, err := ParseAliasFile([]byte(aliasSrc))
		if err != nil {
			checkFuzzError(t, err, AliasFileName, aliasSrc)
			return
		}
		document, err := frontmatter.Split(testFile, []byte(page))
		if err != nil {
			return
		}
		p := New(aliasFile)
		fenced, err := p.Squero(testFile, document.Body, document.BodyLineOffset)
		if err != nil {
			checkFuzzError(t, err, testFile, page)
			return
		}
		if !hasFenceLine(document.Body) && !bytes.Equal(fenced, document.Body) {
			t.Errorf("a body without fences changed:\n got %q\nwant %q", fenced, document.Body)
		}
		if _, err := p.Markdown(fenced); err != nil {
			t.Errorf("Markdown: %v", err)
		}
	})
}

// checkFuzzError fails the test unless err is a *diag.Error naming file at
// a position inside src, counted as package diag counts it.
func checkFuzzError(t *testing.T, err error, file, src string) {
	t.Helper()
	var diagErr *diag.Error
	if !errors.As(err, &diagErr) {
		t.Fatalf("error %v (%T) is not a *diag.Error", err, err)
	}
	if diagErr.File != file {
		t.Errorf("error %v names %q, want %q", err, diagErr.File, file)
	}
	if diagErr.Line == 0 {
		// Only the encoding error has no position.
		if diagErr.Msg != sourcetext.UTF16Message {
			t.Errorf("error %v has no line", err)
		}
		return
	}
	// Positions never count the byte order mark (package sourcetext).
	lines := bytes.Split(sourcetext.TrimByteOrderMark([]byte(src)), []byte("\n"))
	if diagErr.Line < 1 || diagErr.Line > len(lines) {
		t.Fatalf("error %v: line %d is outside the %d lines of the file", err, diagErr.Line, len(lines))
	}
	line := lines[diagErr.Line-1]
	if diagErr.Col < 1 || diagErr.Col > len(line)+1 {
		t.Errorf("error %v: column %d is outside line %q", err, diagErr.Col, line)
	}
}

// hasFenceLine reports whether a line of body starts with ":::", the only
// lines Squero may change.
func hasFenceLine(body []byte) bool {
	for _, line := range bytes.Split(body, []byte("\n")) {
		if bytes.HasPrefix(line, []byte(":::")) {
			return true
		}
	}
	return false
}
