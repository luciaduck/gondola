// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package frontmatter

import (
	"time"
)

// Defaults are the values of the default: section of gondola.yaml,
// completed so that every variable Gondola defines exists on every page.
type Defaults struct {
	variables  map[string]any
	dateFormat string
}

// alwaysDefined are the special keys that the built-in default: section
// leaves out, with the value a page gets when no level sets them. Templates
// fail on undefined variables, so {{ if sitemap }} must work on every page.
// sitemap is true because a page is listed in the sitemap unless it says
// otherwise (robots.txt may still exclude it).
func alwaysDefined() map[string]any {
	return map[string]any{
		tagsKey:             []string{},
		sitemapKey:          true,
		excludeFromFleetKey: false,
	}
}

// NewDefaults returns the defaults made of variables, the entries of
// default: under their template names, as package config reads them with
// ReadValue (so their special keys already have the right types).
// dateFormat is the Go time layout of config.date-format, used to format
// the date that templates see. variables is not changed.
func NewDefaults(variables map[string]any, dateFormat string) Defaults {
	completed := alwaysDefined()
	for name, value := range variables {
		if _, builtIn := completed[name]; builtIn && isEmptyValue(value) {
			continue
		}
		completed[name] = value
	}
	return Defaults{variables: completed, dateFormat: dateFormat}
}

// Merged holds the variables of one page after merging.
type Merged struct {
	// Variables are the template variables from front matter and defaults,
	// under their template names. The date is already formatted with
	// config.date-format, or "" for a page without a date.
	Variables map[string]any
	// Date is the page's date, for ordering pages and for the sitemap. It
	// is meaningful only when HasDate is true.
	Date    time.Time
	HasDate bool
}

// Merge merges the variables of a page with those of its layouts and the
// defaults. When the same key appears in several places, the one closest to
// the page wins; a key that is empty ("") at one level falls through to the
// next weaker one, from strongest to weakest: the page, the nearest layout,
// …, the farthest layout, the defaults. layoutsNearestFirst lists the
// layout chain as it is followed, starting with the page's own layout.
//
// The layout key of a layout template is not merged: it only names that
// layout's parent (see ParentLayout). A page's layout is its own, or the
// default one.
//
// The returned map is new, but the values in it are not copied: a list or
// map may be shared with other pages, so callers must not change them.
func Merge(defaults Defaults, layoutsNearestFirst []Document, page Document) Merged {
	variables := make(map[string]any, len(defaults.variables)+len(page.Variables))
	// From the weakest level to the strongest, so that each level replaces
	// what the weaker ones set, unless its value is empty.
	mergeLevel(variables, defaults.variables, false)
	for i := len(layoutsNearestFirst) - 1; i >= 0; i-- {
		mergeLevel(variables, layoutsNearestFirst[i].Variables, true)
	}
	mergeLevel(variables, page.Variables, false)

	merged := Merged{Variables: variables}
	if date, ok := variables[dateKey].(time.Time); ok {
		merged.Date, merged.HasDate = date, true
		variables[dateKey] = date.Format(defaults.dateFormat)
	}
	return merged
}

// mergeLevel copies the variables of one level into merged. An empty value
// replaces nothing, but still makes the variable exist.
func mergeLevel(merged, level map[string]any, isLayout bool) {
	for name, value := range level {
		if isLayout && name == layoutKey {
			continue
		}
		if _, exists := merged[name]; exists && isEmptyValue(value) {
			continue
		}
		merged[name] = value
	}
}

// isEmptyValue reports whether a variable was left empty. ReadValue and
// package config turn every empty value into "".
func isEmptyValue(value any) bool {
	return value == nil || value == ""
}

// ParentLayout returns the name of the layout that wraps layout, a layout
// template's document, and false when the chain ends there: its layout key
// is absent, empty or "nil". Defaults are never applied to this key;
// otherwise default.layout: base would make base wrap itself.
func ParentLayout(layout Document) (string, bool) {
	parent, _ := layout.Variables[layoutKey].(string)
	if parent == "" || parent == NoLayout {
		return "", false
	}
	return parent, true
}
