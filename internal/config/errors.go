package config

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/diag"
	"gopkg.in/yaml.v3"
)

// yamlLinePattern matches the position that yaml.v3 writes at the start of
// its messages, as in "yaml: line 3: did not find expected key". The (?s)
// flag lets the message span several lines.
var yamlLinePattern = regexp.MustCompile(`(?s)^(?:yaml: )?line (\d+): (.*)$`)

// yamlError turns an error from yaml.v3 into a diag error at the line it
// names. yaml.v3 gives no column, so the column is 1, as for frontmatter.
func yamlError(err error) error {
	message := err.Error()
	// A *yaml.TypeError holds one message per problem; the first is enough,
	// since configuration errors stop at once.
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) && len(typeErr.Errors) > 0 {
		message = typeErr.Errors[0]
	}
	diagErr := &diag.Error{File: FileName}
	if match := yamlLinePattern.FindStringSubmatch(message); match != nil {
		diagErr.Line, _ = strconv.Atoi(match[1])
		diagErr.Col = 1
		message = match[2]
	}
	diagErr.Msg = "invalid YAML: " + strings.TrimPrefix(message, "yaml: ")
	return diagErr
}

// toFilePosition converts the position of a diag error from yaml.v3's way
// of counting to Gondola's, so that the console prints the right line with
// the caret under the right character. yaml.v3 counts columns in characters
// and also starts a new line at a lone "\r", NEL (U+0085), LS (U+2028) and
// PS (U+2029); Gondola counts lines separated by "\n" and columns in bytes
// (see package diag). An error without a line is returned unchanged.
func toFilePosition(err error, src []byte) error {
	diagErr, ok := err.(*diag.Error)
	if !ok || diagErr.Line <= 0 {
		return err
	}
	offset := yamlOffset(src, diagErr.Line, max(diagErr.Col, 1))
	before := src[:offset]
	converted := *diagErr
	converted.Line = bytes.Count(before, []byte("\n")) + 1
	if diagErr.Col > 0 {
		// LastIndexByte is -1 on the first line, which makes this offset+1.
		converted.Col = offset - bytes.LastIndexByte(before, '\n')
	}
	return &converted
}

// yamlOffset returns the byte offset in src of the position that yaml.v3
// calls line and column (both starting at 1). A column past the end of its
// line gives the end of the line; a line past the end of src gives the end
// of src.
func yamlOffset(src []byte, line, col int) int {
	currentLine, currentCol := 1, 1
	offset := 0
	for offset < len(src) {
		if currentLine == line && currentCol == col {
			return offset
		}
		if breakSize := yamlLineBreakSize(src[offset:]); breakSize > 0 {
			if currentLine == line {
				return offset
			}
			currentLine, currentCol = currentLine+1, 1
			offset += breakSize
			continue
		}
		// Invalid UTF-8 counts as one character of one byte.
		_, size := utf8.DecodeRune(src[offset:])
		currentCol++
		offset += size
	}
	return offset
}

// yamlLineBreaks are the line breaks that yaml.v3 recognizes besides "\r\n",
// "\r" and "\n": NEL, LS and PS.
var yamlLineBreaks = []string{"\u0085", "\u2028", "\u2029"}

// yamlLineBreakSize returns the length in bytes of the line break at the
// start of text, or 0 if text does not start with one.
func yamlLineBreakSize(text []byte) int {
	switch {
	case bytes.HasPrefix(text, []byte("\r\n")):
		return 2
	case len(text) > 0 && (text[0] == '\r' || text[0] == '\n'):
		return 1
	}
	for _, lineBreak := range yamlLineBreaks {
		if bytes.HasPrefix(text, []byte(lineBreak)) {
			return len(lineBreak)
		}
	}
	return 0
}
