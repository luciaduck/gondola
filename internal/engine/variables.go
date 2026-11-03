// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine

import (
	"reflect"
	"slices"

	"github.com/CloudyKit/jet/v6"
	"github.com/luciaduck/gondola/internal/collections"
	"github.com/luciaduck/gondola/internal/config"
)

// The template variables that Gondola sets itself (SPEC §5.11). Package
// naming refuses them as front matter keys, so they never collide with the
// page's own variables.
const (
	copyVariable        = "copy"
	siteVariable        = "site"
	dataVariable        = "data"
	fleetVariable       = "fleet"
	collectionsVariable = "collections"
	pageVariable        = "page"
)

// Globals are the template variables that are the same for every page of a
// build: site, data, fleet and collections.
type Globals struct {
	site         map[string]any
	canonicalURL string
	data         map[string]any
	fleet        []map[string]any
	collections  map[string][]map[string]any
}

// NewGlobals returns the variables shared by every page. data is the value
// of data.Load; pages is the result of collections.Build.
func NewGlobals(settings config.Settings, data map[string]any, pages collections.Collections) Globals {
	// The keys are the template names of the config keys (naming's rule:
	// canonical-url becomes canonicalUrl).
	site := map[string]any{
		"name":         settings.SiteName,
		"version":      settings.SiteVersion,
		"description":  settings.Description,
		"canonicalUrl": settings.CanonicalURL,
	}
	return Globals{
		site:         site,
		canonicalURL: settings.CanonicalURL,
		data:         data,
		fleet:        pages.Fleet,
		collections:  pages.Tags,
	}
}

// Page is what the engine needs to know about the page it renders.
type Page struct {
	// SourcePath is relative to the project root, with "/", such as
	// "blog/post.md". Messages name the page with it.
	SourcePath string
	// URLPath is the page's URL path from package routing: it begins with
	// "/" and is already percent-encoded.
	URLPath string
	// Variables are the page's variables after merging with its layouts
	// and the defaults (frontmatter.Merged.Variables). Nothing changes them.
	Variables map[string]any
}

// variables are the values that one Jet execution sees, with their names
// kept apart for the message about an undefined variable.
type variables struct {
	values jet.VarMap
	names  []string
}

// newVariables returns the variables of page: its own under their template
// names, then site, data, fleet, collections and page.
func newVariables(globals Globals, page Page) variables {
	pageValue := map[string]any{
		"url":       page.URLPath,
		"canonical": globals.canonicalURL + page.URLPath,
		"source":    page.SourcePath,
		// The same map as the meta of the page's entry in fleet and
		// collections (package collections).
		"meta": page.Variables,
	}
	values := jet.VarMap{}
	for name, value := range page.Variables {
		values.Set(name, value)
	}
	values.Set(siteVariable, globals.site)
	values.Set(dataVariable, globals.data)
	values.Set(fleetVariable, globals.fleet)
	values.Set(collectionsVariable, globals.collections)
	values.Set(pageVariable, pageValue)
	return variables{values: values, names: sortedNames(values)}
}

// withCopy returns the variables of v plus copy, the page's HTML so far,
// for the second Jet round. v is not changed.
func (v variables) withCopy(copyHTML string) variables {
	values := make(jet.VarMap, len(v.values)+1)
	for name, value := range v.values {
		values[name] = value
	}
	values.Set(copyVariable, copyHTML)
	return variables{values: values, names: sortedNames(values)}
}

// sortedNames returns the names of values in byte order, so that messages
// never depend on the order of map iteration.
func sortedNames(values map[string]reflect.Value) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
