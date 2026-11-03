// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package parser

import (
	"html"
	"strconv"

	"github.com/gosimple/slug"
	"github.com/yuin/goldmark/ast"
)

// fallbackHeadingID is the id of a heading that has no character left after
// slugging, such as "## !!!". It is Goldmark's own fallback. Only headings
// get automatic ids in Gondola (no attribute extension), so Goldmark's other
// fallback, "id", is never needed.
const fallbackHeadingID = "heading"

// headingIDs gives headings their automatic id attribute. It implements
// Goldmark's parser.IDs interface, which Goldmark calls for each heading.
//
// Goldmark's own generator drops every character that is not ASCII, so
// "## Città" became id="citt". This one makes the id with gosimple/slug,
// like slug permalinks: "citta". It keeps Goldmark's other rule: a repeated
// id gets "-1", "-2", and so on.
type headingIDs struct {
	used map[string]bool
}

func newHeadingIDs() *headingIDs {
	return &headingIDs{used: map[string]bool{}}
}

// Generate returns a new id for a heading whose source line is value (the
// Markdown as written, so "## **Bold** text" gives "bold-text"). HTML
// entities are decoded first, because Jet escapes the variables it writes
// into a page: "# {{ title }}" with the title "Fish & chips" reaches
// Goldmark as "# Fish &amp; chips", and its id must still be
// "fish-and-chips".
func (h *headingIDs) Generate(value []byte, _ ast.NodeKind) []byte {
	id := slug.Make(html.UnescapeString(string(value)))
	if id == "" {
		id = fallbackHeadingID
	}
	unique := id
	for suffix := 1; h.used[unique]; suffix++ {
		unique = id + "-" + strconv.Itoa(suffix)
	}
	h.used[unique] = true
	return []byte(unique)
}

// Put records an id that was written by hand, so that Generate does not
// give it to another heading. Goldmark calls it only when an extension lets
// authors set ids, which Gondola does not enable.
func (h *headingIDs) Put(value []byte) {
	h.used[string(value)] = true
}
