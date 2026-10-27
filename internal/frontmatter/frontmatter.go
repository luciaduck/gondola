// Package frontmatter splits a source file (a page or a layout template)
// into its YAML front matter and its body, reads the front matter's
// variables under their template names, and merges the variables of a page
// with those of its layouts and the defaults of gondola.yaml.
//
// The split is written by hand rather than with a library: Gondola needs a
// block that starts on the first line, an error for a block that is never
// closed, and the exact number of lines before the body (SPEC §5.4). Only
// YAML is supported; another format would be detected in cutBlock.
package frontmatter

import (
	"bytes"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
	"github.com/luciaduck/gondola/internal/yamlnode"
)

// delimiter is the line that opens and closes a front matter block.
const delimiter = "---"

// delimiterTrailingSpace is what may follow "---" on a delimiter line:
// spaces and tabs that an editor left behind, and the "\r" of a CRLF line
// ending.
const delimiterTrailingSpace = " \t\r"

// unterminatedMessage is the error for a block that is opened but never
// closed. Without it, the whole file would silently become the body.
const unterminatedMessage = `the front matter that starts here has no closing "---" line; please add one`

// Document is a source file split into its front matter and its body.
type Document struct {
	// Variables holds the keys of the front matter under their template
	// names ("og-image" becomes "ogImage"). A key written empty (null, ~,
	// nothing, or "") holds "", so that it falls through to the defaults
	// when variables are merged but still exists. The special keys have the
	// types described in ReadValue.
	Variables map[string]any
	// Body is the rest of the file after the front matter: the whole file
	// when there is none. It never holds the UTF-8 byte order mark.
	Body []byte
	// BodyLineOffset is the number of lines of the file before Body. Adding
	// it to a line counted in Body gives the line in the whole file.
	BodyLineOffset int
}

// Split splits src, the contents of file, into front matter and body.
// file is the path printed in errors: relative to the project directory and
// written with "/". Errors that point at a place in the file are
// *diag.Error values whose lines count the whole file, the opening "---"
// being line 1.
func Split(file string, src []byte) (Document, error) {
	document, err := split(src)
	if err != nil {
		return Document{}, diag.WithFile(err, file)
	}
	return document, nil
}

func split(src []byte) (Document, error) {
	if err := sourcetext.CheckEncoding(src); err != nil {
		return Document{}, err
	}
	text := sourcetext.TrimByteOrderMark(src)
	block, body, found := cutBlock(text)
	if !found {
		return Document{Variables: map[string]any{}, Body: text}, nil
	}
	if body == nil {
		return Document{}, &diag.Error{Line: 1, Col: 1, Msg: unterminatedMessage}
	}
	// The block starts after the opening line, so its line 1 is line 2 of
	// the file.
	source := yamlnode.Source{Text: block, LineOffset: 1}
	variables, err := readVariables(source)
	if err != nil {
		return Document{}, source.ToFilePosition(err)
	}
	return Document{
		Variables:      variables,
		Body:           body,
		BodyLineOffset: bytes.Count(text[:len(text)-len(body)], []byte("\n")),
	}, nil
}

// cutBlock finds a front matter block at the start of text. found reports
// whether the first line is a delimiter. When it is, block is the text
// between the two delimiter lines and body the text after the closing one;
// body is nil (not just empty) when no closing line exists. Both are
// slices of text, so body is exactly the end of the file.
func cutBlock(text []byte) (block, body []byte, found bool) {
	firstLine, rest, _ := bytes.Cut(text, []byte("\n"))
	if !isDelimiter(firstLine) {
		return nil, nil, false
	}
	lineStart := 0
	for {
		lineLength := bytes.IndexByte(rest[lineStart:], '\n')
		lineEnd, nextLineStart := lineStart+lineLength, lineStart+lineLength+1
		if lineLength < 0 {
			lineEnd, nextLineStart = len(rest), len(rest)
		}
		if isDelimiter(rest[lineStart:lineEnd]) {
			// rest holds a delimiter, so it is not nil, and neither is
			// rest[nextLineStart:], even when it is empty.
			return rest[:lineStart], rest[nextLineStart:], true
		}
		if lineLength < 0 {
			return nil, nil, true
		}
		lineStart = nextLineStart
	}
}

// isDelimiter reports whether line, without its "\n", is a "---" line.
func isDelimiter(line []byte) bool {
	return string(bytes.TrimRight(line, delimiterTrailingSpace)) == delimiter
}
