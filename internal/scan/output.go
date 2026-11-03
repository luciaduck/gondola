// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package scan

import (
	"fmt"
	"strings"
)

// SourceKind names the kind of source that writes an output file, in
// messages.
type SourceKind string

// The kinds of source that write output files.
const (
	PageSource        SourceKind = "page"
	SassSource        SourceKind = "Sass entry point"
	PassthroughSource SourceKind = "passthrough file"
	// SitemapSource is the sitemap that Gondola writes itself when
	// config.sitemap is true. It has no source file, and a collision with
	// it has a message of its own (M10, Q4).
	SitemapSource SourceKind = "sitemap"
)

// OutputPaths records which source writes each file of the output, so that
// two sources that would write the same file are an error naming both.
//
// Output paths are compared ignoring letter case on every file system: an
// output that holds both "A.css" and "a.css" breaks when it is published
// from Windows or macOS, as SPEC §5.7 says for pages. Scan claims the paths
// of Sass entry points and passthrough files; the build claims the pages'
// paths once routing has worked them out.
//
// The zero value is ready to use, as for strings.Builder: the map is made
// by the first Claim.
type OutputPaths struct {
	// claims are keyed by the output path in small letters.
	claims map[string]outputClaim
}

type outputClaim struct {
	outputPath string
	kind       SourceKind
	sourcePath string
}

// Claim records that the source at sourcePath (relative to the project
// root) writes outputPath (relative to the output directory). Both are
// written with "/". It returns an error when an earlier source writes the
// same path, or one that differs from it only in letter case.
func (o *OutputPaths) Claim(outputPath string, kind SourceKind, sourcePath string) error {
	if o.claims == nil {
		o.claims = make(map[string]outputClaim)
	}
	key := strings.ToLower(outputPath)
	earlier, found := o.claims[key]
	if !found {
		o.claims[key] = outputClaim{outputPath: outputPath, kind: kind, sourcePath: sourcePath}
		return nil
	}
	if earlier.kind == SitemapSource {
		return sitemapCollision(kind, sourcePath, outputPath)
	}
	if kind == SitemapSource {
		return sitemapCollision(earlier.kind, earlier.sourcePath, earlier.outputPath)
	}
	if earlier.outputPath == outputPath {
		return fmt.Errorf("%s %q and %s %q both write %q in the output; please rename or remove one",
			earlier.kind, earlier.sourcePath, kind, sourcePath, outputPath)
	}
	return fmt.Errorf("%s %q and %s %q both write %q and %q, which differ only in letter case, in the output; please rename or remove one",
		earlier.kind, earlier.sourcePath, kind, sourcePath, earlier.outputPath, outputPath)
}

// sitemapCollision is the error for a source that writes the file where
// Gondola writes the sitemap. outputPath is the path the source writes,
// which may differ from the sitemap's in letter case.
func sitemapCollision(kind SourceKind, sourcePath, outputPath string) error {
	return fmt.Errorf("%s %q writes %q, where Gondola writes the sitemap because config.sitemap is true; please rename or remove it, or turn config.sitemap off",
		kind, sourcePath, outputPath)
}

// Source returns the kind and the source path of what writes outputPath,
// written exactly so (letter case included), and false when nothing claimed
// it.
func (o *OutputPaths) Source(outputPath string) (SourceKind, string, bool) {
	claim, found := o.claims[strings.ToLower(outputPath)]
	if !found || claim.outputPath != outputPath {
		return "", "", false
	}
	return claim.kind, claim.sourcePath, true
}
