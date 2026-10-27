package yamlnode

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Tags that DecodeValue sets or looks for on mapping keys.
const (
	stringTag = "!!str"
	mergeTag  = "!!merge"
)

// DecodeValue decodes a free-form value (a front matter value, an entry of
// default:) into Go values as yaml.v3 does, with two differences that keep
// templates working and the output deterministic:
//
//   - Every map is a map[string]any: the keys of nested maps are kept as
//     text, exactly as written. yaml.v3 alone would make a
//     map[interface{}]interface{} as soon as one key is not a string (such
//     as 1: one), and templates could not read it.
//   - Every time value is either in UTC or in a zone that has only an
//     offset. yaml.v3 alone would give the computer's local zone to a time
//     whose offset matches it, so the same file would print differently on
//     another computer.
//
// where names the value in messages, such as `front matter key "author"`.
// A list or a map written as a key, and a key written twice in one map, are
// errors. Errors are *diag.Error values without a file name, at positions
// in yaml.v3's counting; source is used only for the line written in the
// message about a repeated key.
//
// DecodeValue changes the tree it is given: the keys of maps get the string
// tag, and an alias written as a key is replaced by a copy of the text it
// points to.
func DecodeValue(node *yaml.Node, where string, source Source) (any, error) {
	keys := keyNormalizer{where: where, source: source, visited: make(map[*yaml.Node]bool)}
	if err := keys.normalize(node); err != nil {
		return nil, err
	}
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, syntaxErrorIn(node, err)
	}
	return withPortableTimes(value), nil
}

// syntaxErrorIn is SyntaxError for an error found while decoding node.
// yaml.v3 gives no line for some of them, such as "cannot decode !!str `x`
// as a !!int"; those point at the start of node, the closest known place.
func syntaxErrorIn(node *yaml.Node, err error) error {
	diagErr := syntaxError(err)
	if diagErr.Line == 0 {
		diagErr.Line, diagErr.Col = node.Line, node.Column
	}
	return diagErr
}

// keyNormalizer walks a node tree once and gives every mapping key the
// string tag, so that yaml.v3 decodes it as text. Its methods have value
// receivers: each call gets a copy of the struct, but a map value refers to
// the same map, so every copy shares visited.
type keyNormalizer struct {
	where  string
	source Source
	// visited holds the nodes already walked. Aliases are followed, because
	// an anchor may be defined somewhere this walk never reaches, but each
	// node is walked once: that keeps the walk linear even for documents
	// that repeat an alias many times.
	visited map[*yaml.Node]bool
}

func (k keyNormalizer) normalize(node *yaml.Node) error {
	if node == nil || k.visited[node] {
		return nil
	}
	k.visited[node] = true
	switch node.Kind {
	case yaml.AliasNode:
		return k.normalize(node.Alias)
	case yaml.MappingNode:
		return k.normalizeMapping(node)
	}
	for _, child := range node.Content {
		if err := k.normalize(child); err != nil {
			return err
		}
	}
	return nil
}

func (k keyNormalizer) normalizeMapping(mapping *yaml.Node) error {
	firstLineByKey := make(map[string]int)
	// Content holds the keys and values one after the other.
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode, valueNode := mapping.Content[i], mapping.Content[i+1]
		key := Resolved(keyNode)
		if key.Kind != yaml.ScalarNode {
			return ErrorAt(keyNode, "keys in %s must be text, not %s", k.where, Describe(keyNode))
		}
		// "<<: *base" merges another map into this one; yaml.v3 needs the
		// key's own tag to do it, and a merge may repeat keys on purpose.
		if key.ShortTag() == mergeTag {
			if err := k.normalize(valueNode); err != nil {
				return err
			}
			continue
		}
		if firstLine, seen := firstLineByKey[key.Value]; seen {
			return ErrorAt(keyNode, "key %q appears twice in %s (first at line %d); please keep only one", key.Value, k.where, firstLine)
		}
		firstLineByKey[key.Value] = k.source.FileLine(keyNode.Line)
		if keyNode.Kind == yaml.AliasNode || key.Anchor != "" {
			// Retagging an anchored node would also change it where an
			// alias uses it as a value, so the key gets its own copy.
			keyCopy := *key
			keyCopy.Anchor = ""
			keyCopy.Line, keyCopy.Column = keyNode.Line, keyNode.Column
			keyNode = &keyCopy
			mapping.Content[i] = keyNode
		}
		keyNode.Tag = stringTag
		if err := k.normalize(valueNode); err != nil {
			return err
		}
	}
	return nil
}

// withPortableTimes returns value with every time.Time in it moved to UTC
// when its offset is zero, or to a zone that has only its offset otherwise.
// Maps and lists are changed in place; they were just decoded and are not
// shared yet.
func withPortableTimes(value any) any {
	switch typed := value.(type) {
	case time.Time:
		return PortableTime(typed)
	case map[string]any:
		for key, entry := range typed {
			typed[key] = withPortableTimes(entry)
		}
	case []any:
		for i, entry := range typed {
			typed[i] = withPortableTimes(entry)
		}
	}
	return value
}

// PortableTime returns t in UTC when its offset is zero, or in a zone with
// no name and t's offset otherwise, so that formatting it does not depend
// on the zone of the computer that runs Gondola.
func PortableTime(t time.Time) time.Time {
	_, offsetSeconds := t.Zone()
	if offsetSeconds == 0 {
		return t.UTC()
	}
	return t.In(time.FixedZone("", offsetSeconds))
}
