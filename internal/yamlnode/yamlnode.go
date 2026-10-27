// Package yamlnode holds what Gondola's YAML readers share (gondola.yaml,
// front matter, and later the data files): the encoding check, the rule for
// empty values, the conversion of yaml.v3's positions and errors to
// positions in the file, and the decoding of free-form values.
//
// Gondola reads YAML as yaml.Node trees instead of decoding it into Go
// values directly, so that every error can point at line and column.
package yamlnode

import (
	"bytes"
	"strconv"

	"github.com/luciaduck/gondola/internal/diag"
	"gopkg.in/yaml.v3"
)

// utf16ByteOrderMarks start a file saved as UTF-16, little-endian and
// big-endian. Windows PowerShell 5.1 writes such files by default.
var utf16ByteOrderMarks = [][]byte{{0xFF, 0xFE}, {0xFE, 0xFF}}

// UTF16Message is the error for a file saved as UTF-16. It has no position:
// the console could not print a line of such a file.
const UTF16Message = "the file is saved as UTF-16; please save it as UTF-8"

// CheckEncoding returns a *diag.Error without a file name if src starts with
// a UTF-16 byte order mark. yaml.v3 would read UTF-16, but the positions in
// its errors would count the decoded text, not the bytes of the file, and
// every other source file Gondola reads is UTF-8. A UTF-8 byte order mark is
// fine.
func CheckEncoding(src []byte) error {
	for _, byteOrderMark := range utf16ByteOrderMarks {
		if bytes.HasPrefix(src, byteOrderMark) {
			return &diag.Error{Msg: UTF16Message}
		}
	}
	return nil
}

// Resolved returns the node that an alias (*name) points to, or node itself.
// Positions are still taken from the alias, which is where the user wrote
// the value.
func Resolved(node *yaml.Node) *yaml.Node {
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return node.Alias
	}
	return node
}

// IsEmpty reports whether node is null (written as null, ~ or nothing) or
// the empty string, which both mean "use the default". A tag does not change
// this: a tag with no text after it, such as "!x" or `!!str ""`, is empty
// too. The tag is checked first because "null" and "~" have text.
func IsEmpty(node *yaml.Node) bool {
	value := Resolved(node)
	if value.Kind != yaml.ScalarNode {
		return false
	}
	return value.ShortTag() == "!!null" || value.Value == ""
}

// Describe names a value for messages: "a map", "a list", or the scalar
// quoted as written.
func Describe(node *yaml.Node) string {
	value := Resolved(node)
	switch value.Kind {
	case yaml.MappingNode:
		return "a map"
	case yaml.SequenceNode:
		return "a list"
	}
	return strconv.Quote(value.Value)
}
