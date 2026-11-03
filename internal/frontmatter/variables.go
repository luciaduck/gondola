// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package frontmatter

import (
	"fmt"
	"strings"
	"time"

	"github.com/luciaduck/gondola/internal/naming"
	"github.com/luciaduck/gondola/internal/yamlnode"
	"gopkg.in/yaml.v3"
)

// The template names of the keys that Gondola itself understands (SPEC §4).
const (
	layoutKey           = "layout"
	titleKey            = "title"
	authorKey           = "author"
	dateKey             = "date"
	permalinkKey        = "permalink"
	tagsKey             = "tags"
	sitemapKey          = "sitemap"
	excludeFromFleetKey = "excludeFromFleet"
)

// NoLayout is the layout value that means "no layout". On a page, the page
// is not run through Jet at all; on a layout template, it only means that
// the layout has no parent.
const NoLayout = "nil"

// keyNoun names the keys of a front matter block in messages from package
// naming.
const keyNoun = "front matter key"

// fleetTag cannot be a tag: templates use fleet for the collection of all
// pages, so a tag of that name would suggest a second, different fleet.
const fleetTag = "fleet"

// dateLayouts are the forms a date may be written in: the timestamp forms
// that yaml.v3 itself recognizes, so a date means the same whether it is
// quoted or not. A date without a zone is in UTC.
var dateLayouts = []string{
	"2006-1-2T15:4:5.999999999Z07:00",
	"2006-1-2t15:4:5.999999999Z07:00",
	"2006-1-2 15:4:5.999999999",
	"2006-1-2",
}

// dateHint tells the user how to write a date.
const dateHint = "please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z"

// Key is a key of a front matter block or of the default: section of
// gondola.yaml, as messages name it.
type Key struct {
	// Written is the key as the user wrote it, such as "og-image".
	Written string
	// InDefault is true for a key of default: in gondola.yaml, false for a
	// key of a page's or a layout's front matter.
	InDefault bool
}

// label names the key in messages: `front matter key "title"` or
// "default.title".
func (k Key) label() string {
	if k.InDefault {
		return "default." + k.Written
	}
	return fmt.Sprintf("%s %q", keyNoun, k.Written)
}

// valueLabel names the key in front of a value it holds: `date` in
// `date "x" is not a date`, or "default.date".
func (k Key) valueLabel() string {
	if k.InDefault {
		return "default." + k.Written
	}
	return k.Written
}

func readVariables(source yamlnode.Source) (map[string]any, error) {
	variables := map[string]any{}
	root, err := yamlnode.ParseDocument(source.Text, "front matter")
	if err != nil {
		return nil, err
	}
	if root == nil || yamlnode.IsEmpty(root) {
		return variables, nil
	}
	mapping := yamlnode.Resolved(root)
	if mapping.Kind != yaml.MappingNode {
		return nil, yamlnode.ErrorAt(root, "front matter must be a map of keys, not %s", yamlnode.Describe(root))
	}
	scope := naming.NewScope(keyNoun)
	// yaml.v3 does not report a repeated key when it builds nodes, so it
	// is checked here, before two equal keys reach the scope.
	firstLineByKey := make(map[string]int)
	// Content holds the keys and values one after the other.
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode, valueNode := mapping.Content[i], mapping.Content[i+1]
		if yamlnode.Resolved(keyNode).Kind != yaml.ScalarNode {
			return nil, yamlnode.ErrorAt(keyNode, "keys in front matter must be names, not %s", yamlnode.Describe(keyNode))
		}
		key := yamlnode.Resolved(keyNode).Value
		if firstLine, seen := firstLineByKey[key]; seen {
			return nil, yamlnode.ErrorAt(keyNode, "%s %q appears twice (first at line %d); please keep only one", keyNoun, key, firstLine)
		}
		firstLineByKey[key] = source.FileLine(keyNode.Line)
		name, err := scope.Add(key)
		if err != nil {
			return nil, yamlnode.WrapAt(keyNode, err)
		}
		if err := naming.CheckReserved(key, name, keyNoun); err != nil {
			return nil, yamlnode.WrapAt(keyNode, err)
		}
		value, err := ReadValue(name, Key{Written: key}, valueNode, source)
		if err != nil {
			return nil, err
		}
		variables[name] = value
	}
	return variables, nil
}

// ReadValue reads the value of one key, whose template name is name, from
// node. It is shared by front matter and the default: section of
// gondola.yaml, so that both follow the same rules:
//
//   - An empty value (null, ~, nothing, "", or a tag with no text) is "".
//   - layout, title, author and permalink are text, kept exactly as written
//     (title: 0.10 is "0.10"). A tag on the text is ignored. layout and
//     permalink are also checked with the rules of CheckLayout and
//     CheckPermalink.
//   - sitemap and excludeFromFleet are YAML booleans.
//   - tags is one tag or a list of tags, and becomes a []string. Tags are
//     trimmed; an empty tag, a tag listed twice and the tag "fleet" are
//     errors.
//   - date is a time.Time, in one of the forms of dateLayouts, quoted or
//     not.
//   - Any other value keeps its YAML type, decoded by yamlnode.DecodeValue.
//
// Errors are *diag.Error values without a file name, at positions in
// yaml.v3's counting within source.
func ReadValue(name string, key Key, node *yaml.Node, source yamlnode.Source) (any, error) {
	if yamlnode.IsEmpty(node) {
		return "", nil
	}
	switch name {
	case layoutKey:
		return readLayout(key, node)
	case titleKey, authorKey:
		return readText(key, node)
	case permalinkKey:
		return readPermalink(key, node)
	case sitemapKey, excludeFromFleetKey:
		return readBool(key, node)
	case tagsKey:
		return readTags(key, node)
	case dateKey:
		return readDate(key, node)
	}
	return yamlnode.DecodeValue(node, key.label(), source)
}

func readText(key Key, node *yaml.Node) (string, error) {
	value := yamlnode.Resolved(node)
	if value.Kind != yaml.ScalarNode {
		return "", yamlnode.ErrorAt(node, "%s must be text, not %s", key.label(), yamlnode.Describe(node))
	}
	// Value is the text as written, so 0.10 stays "0.10".
	return value.Value, nil
}

// readLayout reads text and checks it with the rules of CheckLayout.
func readLayout(key Key, node *yaml.Node) (string, error) {
	layout, err := readText(key, node)
	if err != nil {
		return "", err
	}
	if err := CheckLayout(key, layout); err != nil {
		return "", yamlnode.WrapAt(node, err)
	}
	return layout, nil
}

// readPermalink reads text and checks it with the rules of CheckPermalink.
// default.permalink never comes here: package config accepts only the three
// choices there.
func readPermalink(key Key, node *yaml.Node) (string, error) {
	permalink, err := readText(key, node)
	if err != nil {
		return "", err
	}
	if err := checkPermalink(key.valueLabel(), permalink); err != nil {
		return "", yamlnode.WrapAt(node, err)
	}
	return permalink, nil
}

func readBool(key Key, node *yaml.Node) (bool, error) {
	if yamlnode.Resolved(node).ShortTag() != "!!bool" {
		return false, yamlnode.ErrorAt(node, "%s must be true or false, not %s", key.label(), yamlnode.Describe(node))
	}
	var value bool
	err := node.Decode(&value)
	return value, err
}

func readTags(key Key, node *yaml.Node) ([]string, error) {
	value := yamlnode.Resolved(node)
	if value.Kind == yaml.ScalarNode {
		tag, err := readTag(key, node)
		if err != nil {
			return nil, err
		}
		return []string{tag}, nil
	}
	if value.Kind != yaml.SequenceNode {
		return nil, yamlnode.ErrorAt(node, "%s must be a tag or a list of tags, not %s", key.label(), yamlnode.Describe(node))
	}
	tags := make([]string, 0, len(value.Content))
	for _, entry := range value.Content {
		if yamlnode.Resolved(entry).Kind != yaml.ScalarNode {
			return nil, yamlnode.ErrorAt(entry, "each entry of %s must be a tag, not %s", key.label(), yamlnode.Describe(entry))
		}
		tag, err := readTag(key, entry)
		if err != nil {
			return nil, err
		}
		for _, earlier := range tags {
			if earlier == tag {
				return nil, yamlnode.ErrorAt(entry, "%s lists the tag %q twice; please keep only one", key.label(), tag)
			}
		}
		tags = append(tags, tag)
	}
	return tags, nil
}

// readTag reads one tag from a scalar node. Tags are content, not names,
// so they are trimmed but otherwise kept as written, in any letter case.
func readTag(key Key, node *yaml.Node) (string, error) {
	tag := ""
	if !yamlnode.IsEmpty(node) {
		tag = strings.TrimSpace(yamlnode.Resolved(node).Value)
	}
	switch tag {
	case "":
		return "", yamlnode.ErrorAt(node, "%s has an empty tag; please remove it", key.label())
	case fleetTag:
		return "", yamlnode.ErrorAt(node, "tag %q is reserved by Gondola (templates use fleet for the collection of all pages); please rename it", tag)
	}
	return tag, nil
}

func readDate(key Key, node *yaml.Node) (time.Time, error) {
	value := yamlnode.Resolved(node)
	if value.Kind != yaml.ScalarNode {
		return time.Time{}, yamlnode.ErrorAt(node, "%s must be a date, not %s", key.label(), yamlnode.Describe(node))
	}
	date, ok := parseDate(value.Value)
	if !ok {
		return time.Time{}, yamlnode.ErrorAt(node, "%s %q is not a date; %s", key.valueLabel(), value.Value, dateHint)
	}
	return date, nil
}

// parseDate parses text in one of the forms of dateLayouts. It parses the
// text as written rather than letting yaml.v3 decode the value, so that a
// quoted date works too, and so that the time does not take the zone of the
// computer (see yamlnode.PortableTime): with time.UTC as the location, an
// offset other than zero gets a zone that has only the offset.
func parseDate(text string) (time.Time, bool) {
	for _, layout := range dateLayouts {
		if date, err := time.ParseInLocation(layout, text, time.UTC); err == nil {
			return date, true
		}
	}
	return time.Time{}, false
}
