// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package frontmatter

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// layoutPathSeparator separates the directories of a layout name, such as
// "blog/post", on every platform.
const layoutPathSeparator = "/"

// CheckLayout returns an error when layout, the value of key, cannot name a
// template: it must be a path relative to the template directories, written
// with "/", without ".." and without empty or "." parts, and it must not
// contain "\" or control characters. "" (no value) and "nil" are accepted.
// The error has no position.
//
// Front matter checks every layout when it is read, where the position is
// known, and so does package config for default.layout; package engine
// checks again before it looks for a layout, so that no caller can make it
// read a file outside the template directories.
func CheckLayout(key Key, layout string) error {
	if layout == "" || layout == NoLayout {
		return nil
	}
	label := key.valueLabel()
	// yaml.v3 refuses text that is not UTF-8, so front matter never gets
	// here with one; the check is for the other callers.
	if !utf8.ValidString(layout) {
		return fmt.Errorf("%s %q is not valid UTF-8; please write it in UTF-8", label, layout)
	}
	for _, character := range layout {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s %q contains a control character (%U); please remove it", label, layout, character)
		}
	}
	if strings.Contains(layout, `\`) {
		return fmt.Errorf(`%s %q contains "\\"; please separate directories with "/"`, label, layout)
	}
	if strings.HasPrefix(layout, layoutPathSeparator) || slices.Contains(strings.Split(layout, layoutPathSeparator), "..") {
		return fmt.Errorf(`%s %q is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`, label, layout)
	}
	// fs.ValidPath refuses what is left: empty parts ("a//b", "blog/") and
	// "." parts ("./page"), except "." alone, which it accepts as the root.
	if layout == "." || !fs.ValidPath(layout) {
		return fmt.Errorf(`%s %q has an empty or "." part; please write it as a path such as "page" or "blog/post"`, label, layout)
	}
	return nil
}
