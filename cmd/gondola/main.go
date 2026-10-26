// Command gondola is the command-line interface of Gondola, a static site
// generator: `gondola ride` builds the site, `gondola serve` builds and serves
// it for development.
package main

import (
	"fmt"
	"os"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/luciaduck/gondola/internal/console"
)

// version is Gondola's version. It is empty unless it is set at release time
// with the linker, including the leading "v":
//
//	go build -ldflags "-X main.version=v0.0.1" ./cmd/gondola
//
// When it is empty, resolveVersion looks for a version elsewhere.
var version string

// devVersion is printed when no version was injected and the Go toolchain
// recorded no release version either, which is the case for builds from a
// source checkout.
const devVersion = "dev"

// pseudoVersionSuffix matches the end of a Go pseudo-version, such as
// v0.0.0-20261020215731-0a874f1b5626: a 14-digit commit time and a 12-digit
// commit hash. Since Go 1.24, `go build` in a checkout records one, and it is
// not a release.
var pseudoVersionSuffix = regexp.MustCompile(`[-.][0-9]{14}-[0-9a-f]{12}$`)

// Exit codes of the gondola command.
const (
	exitSuccess    = 0
	exitBuildError = 1
	exitUsageError = 2
)

const helpText = `Gondola builds a static site from the current directory, using gondola.yaml.

Usage:
  gondola ride        build the site once
  gondola serve       build, serve on localhost, rebuild when Enter is pressed
  gondola --help      show this help
  gondola --version   print Gondola's version
`

func main() {
	noColor := os.Getenv("NO_COLOR")
	printer := console.New(
		console.Stream{Writer: os.Stdout, Color: console.ColorFor(os.Stdout, noColor)},
		console.Stream{Writer: os.Stderr, Color: console.ColorFor(os.Stderr, noColor)},
		os.DirFS("."),
	)
	buildInfo, _ := debug.ReadBuildInfo()
	os.Exit(run(os.Args[1:], resolveVersion(version, buildInfo), printer))
}

// run executes the command line args (without the program name) and returns
// the exit code. It is separate from main so that tests can call it.
func run(args []string, gondolaVersion string, printer *console.Printer) int {
	if len(args) == 0 {
		printer.PrintStderr(helpText)
		return exitUsageError
	}
	command := args[0]
	// Future flags would be parsed here, with one flag.FlagSet per command
	// that takes flags. For now no command accepts any argument.
	if len(args) > 1 {
		printer.Error(fmt.Errorf("unexpected argument %q; run gondola --help", args[1]))
		return exitUsageError
	}
	switch command {
	case "--help", "-h":
		printer.Print(helpText)
		return exitSuccess
	case "--version":
		printer.Print(fmt.Sprintf("gondola %s\n", gondolaVersion))
		return exitSuccess
	case "ride", "serve":
		// ride arrives with milestone M9, serve with M12.
		printer.Error(fmt.Errorf("%q is not available yet", command))
		return exitBuildError
	default:
		printer.Error(fmt.Errorf("unknown command %q; run gondola --help", command))
		return exitUsageError
	}
}

// resolveVersion returns the version Gondola reports. In order: the version
// injected at release time; the module version that the Go toolchain records
// when Gondola is installed with `go install …@v0.1.0`, if it is a clean
// release; devVersion. buildInfo is what debug.ReadBuildInfo returned, and
// may be nil.
func resolveVersion(injected string, buildInfo *debug.BuildInfo) string {
	if injected != "" {
		return injected
	}
	if buildInfo != nil && isReleaseVersion(buildInfo.Main.Version) {
		return buildInfo.Main.Version
	}
	return devVersion
}

// isReleaseVersion reports whether a version recorded by the Go toolchain
// names a release. It rejects "(devel)" (no version known), pseudo-versions,
// and versions with a "+" suffix such as "+dirty" (uncommitted changes).
func isReleaseVersion(recorded string) bool {
	return strings.HasPrefix(recorded, "v") &&
		!strings.Contains(recorded, "+") &&
		!pseudoVersionSuffix.MatchString(recorded)
}
