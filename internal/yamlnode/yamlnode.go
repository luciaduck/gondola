// Package yamlnode holds what Gondola's YAML readers share (gondola.yaml,
// front matter, and later the data files): the rule for empty values, the
// conversion of yaml.v3's positions and errors to positions in the file, and
// the decoding of free-form values. The encoding check is in sourcetext,
// because files that are not YAML need it too.
//
// Gondola reads YAML as yaml.Node trees instead of decoding it into Go
// values directly, so that every error can point at line and column.
package yamlnode

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

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
