package yamlnode

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/diag"
	"gopkg.in/yaml.v3"
)

// Source is the YAML text that nodes were parsed from, and the number of
// lines of its file that come before that text. gondola.yaml is parsed as a
// whole file (no lines before it); a front matter block starts after its
// opening "---" line.
type Source struct {
	// Text is the YAML that yaml.v3 parsed.
	Text []byte
	// LineOffset is the number of lines of the file before Text.
	LineOffset int
}

// FileLine returns the line of the file, counted as package diag counts
// lines, where line yamlLine of Text (counted as yaml.v3 counts lines) is.
// Use it for a line written inside a message, such as "first at line 5".
func (s Source) FileLine(yamlLine int) int {
	line, _ := s.FilePosition(yamlLine, 1)
	return line
}

// ToFilePosition converts the position of a diag error from yaml.v3's way
// of counting within Text to Gondola's way of counting within the file, so
// that the console prints the right line with the caret under the right
// character. yaml.v3 counts columns in characters and also starts a new line
// at a lone "\r", NEL (U+0085), LS (U+2028) and PS (U+2029); Gondola counts
// lines separated by "\n" and columns in bytes (see package diag). An error
// without a line, or that is not a *diag.Error, is returned unchanged.
func (s Source) ToFilePosition(err error) error {
	diagErr, ok := err.(*diag.Error)
	if !ok || diagErr.Line <= 0 {
		return err
	}
	converted := *diagErr
	converted.Line, converted.Col = s.FilePosition(diagErr.Line, diagErr.Col)
	return &converted
}

// FilePosition converts line and col as yaml.v3 counts them in Text into a
// line and column of the file. A col of 0 (unknown) stays 0. Use it for a
// position kept for a later message, such as the position of a value that
// another package checks.
func (s Source) FilePosition(line, col int) (fileLine, fileCol int) {
	offset := yamlOffset(s.Text, line, max(col, 1))
	before := s.Text[:offset]
	fileLine = bytes.Count(before, []byte("\n")) + 1 + s.LineOffset
	if col > 0 {
		// LastIndexByte is -1 on the first line, which makes this offset+1.
		fileCol = offset - bytes.LastIndexByte(before, '\n')
	}
	return fileLine, fileCol
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

// yamlLinePattern matches the position that yaml.v3 writes at the start of
// its messages, as in "yaml: line 3: did not find expected key". The (?s)
// flag lets the message span several lines.
var yamlLinePattern = regexp.MustCompile(`(?s)^(?:yaml: )?line (\d+): (.*)$`)

// SyntaxError turns an error from yaml.v3 into a *diag.Error without a file
// name, at the line the error names, in yaml.v3's counting. yaml.v3 gives no
// column, so the column is 1.
func SyntaxError(err error) error {
	return syntaxError(err)
}

func syntaxError(err error) *diag.Error {
	message := err.Error()
	// A *yaml.TypeError holds one message per problem; the first is enough,
	// since reading stops at the first error.
	var typeErr *yaml.TypeError
	if errors.As(err, &typeErr) && len(typeErr.Errors) > 0 {
		message = typeErr.Errors[0]
	}
	diagErr := &diag.Error{}
	if match := yamlLinePattern.FindStringSubmatch(message); match != nil {
		diagErr.Line, _ = strconv.Atoi(match[1])
		diagErr.Col = 1
		message = match[2]
	}
	diagErr.Msg = "invalid YAML: " + strings.TrimPrefix(message, "yaml: ")
	return diagErr
}

// ParseDocument parses src as a single YAML document and returns its root
// node, or nil when the document is empty. documentName says what src is in
// the message for a second document, such as "gondola.yaml". Errors are
// *diag.Error values without a file name, at positions in yaml.v3's
// counting.
func ParseDocument(src []byte, documentName string) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(src))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		return nil, SyntaxError(err)
	}
	// A second document would otherwise be ignored without a word.
	var extraDocument yaml.Node
	err := decoder.Decode(&extraDocument)
	if err == nil {
		return nil, ErrorAt(&extraDocument, "a second YAML document starts here; %s must hold only one, so please remove it", documentName)
	}
	if !errors.Is(err, io.EOF) {
		return nil, SyntaxError(err)
	}
	if len(document.Content) == 0 {
		return nil, nil
	}
	return document.Content[0], nil
}

// ErrorAt returns a *diag.Error without a file name at the position of node,
// in yaml.v3's counting, or without a position when node is nil.
func ErrorAt(node *yaml.Node, format string, args ...any) error {
	return WrapAt(node, fmt.Errorf(format, args...))
}

// WrapAt is ErrorAt for an error that already has its message, such as one
// from package naming.
func WrapAt(node *yaml.Node, err error) error {
	diagErr := &diag.Error{Err: err}
	if node != nil {
		diagErr.Line, diagErr.Col = node.Line, node.Column
	}
	return diagErr
}
