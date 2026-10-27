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
	if earlier.outputPath == outputPath {
		return fmt.Errorf("%s %q and %s %q both write %q in the output; please rename or remove one",
			earlier.kind, earlier.sourcePath, kind, sourcePath, outputPath)
	}
	return fmt.Errorf("%s %q and %s %q both write %q and %q, which differ only in letter case, in the output; please rename or remove one",
		earlier.kind, earlier.sourcePath, kind, sourcePath, earlier.outputPath, outputPath)
}
