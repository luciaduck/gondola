package config

import (
	"bytes"
	"errors"
	"path"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// FuzzParse checks that Parse never panics, that every error names
// gondola.yaml at a position the console can show, and that every accepted
// file gives a configuration that keeps the promises of the package. Each
// input is parsed twice: comparing directories by their exact letters, and
// ignoring letter case, as Load does on such a file system.
func FuzzParse(f *testing.F) {
	seeds := []string{
		"",
		minimal,
		withMinimal("  sass: false\n  port: 8080\n  plugin: [a/b.sh]\n"),
		withMinimal("input:\n  content: src\n  templates: [src/_t, shared]\n  data: d\n"),
		withMinimal("output:\n  routing: directory\n  directory: public\n  passthrough: [/robots.txt, .well-known/]\n"),
		withMinimal("default:\n  layout: base\n  og-image: /x.png\n  social: {name: a}\n  permalink: date\n"),
		withMinimal("default: {città: 1, città: 2}\n"),
		withMinimal("input:\n  templates: _site/x\n"),
		withMinimal("output:\n  directory: ../x\n"),
		withMinimal("---\na: b\n"),
		"config: &c\n  site-name: A\n  version: 1\n  canonical-url: https://x.com\ninput: *c\n",
		"x: &a [*a]\n",
		withMinimal("default:\n  loop: &a [*a]\n"),
		withMinimal("default:\n  <<: {a: 1}\n"),
		"config:\n  site-name: [\n",
		"\xff\xfe",
		// Line breaks that yaml.v3 counts but the console does not.
		"config:\r  sas: 1\r",
		"config:\n  site-name: S\u2028  sas: 1\n",
		"config:\u0085  sas: 1\n",
		"a: 'x\u2029y'\nb: [\n",
		// A UTF-8 byte order mark, which is accepted, and UTF-16, which is
		// refused.
		"\ufeffconfig:\n  sas: 1\n",
		"\xff\xfec\x00:\x00 \x00[\x00\n\x00",
		// Directories that differ only in letter case.
		withMinimal("input:\n  content: Site\n  templates: site/_t\noutput:\n  directory: SITE/_t/x\n"),
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		exact := checkParse(t, src, false)
		ignoringCase := checkParse(t, src, true)
		// Ignoring letter case only finds more directories that clash, so it
		// never accepts a file that the exact comparison refuses.
		if !exact && ignoringCase {
			t.Fatalf("accepted only when ignoring letter case")
		}
	})
}

// checkParse parses src and checks the result, reporting whether src was
// accepted.
func checkParse(t *testing.T, src []byte, ignoresLetterCase bool) bool {
	t.Helper()
	config, err := parseFile(src, ignoresLetterCase)
	if err != nil {
		var diagErr *diag.Error
		if !errors.As(err, &diagErr) || diagErr.File != FileName {
			t.Fatalf("error %q is not a diag.Error naming %s", err, FileName)
		}
		// Positions do not count a byte order mark (SPEC §4), so they are
		// checked against the text without it.
		checkErrorPosition(t, diagErr, sourcetext.TrimByteOrderMark(src))
		return false
	}
	checkInvariants(t, config, src)
	again, err := parseFile(src, ignoresLetterCase)
	if err != nil || !reflect.DeepEqual(config, again) {
		t.Fatalf("a second Parse gave a different result: %v", err)
	}
	return true
}

func checkInvariants(t *testing.T, config Config, src []byte) {
	t.Helper()
	settings := config.Settings
	if settings.SiteName == "" || settings.SiteVersion == "" {
		t.Fatalf("required text is empty: %+v", settings)
	}
	scheme := strings.ToLower(settings.CanonicalURL)
	if !strings.HasPrefix(scheme, "http://") && !strings.HasPrefix(scheme, "https://") {
		t.Fatalf("CanonicalURL %q is not http(s)", settings.CanonicalURL)
	}
	if settings.Port < minPort || settings.Port > maxPort {
		t.Fatalf("Port %d is out of range", settings.Port)
	}
	if settings.Engine != EngineJet {
		t.Fatalf("Engine = %q", settings.Engine)
	}
	if settings.FencedBlocks && !settings.Markdown {
		t.Fatalf("fenced blocks are on without Markdown")
	}
	if config.Output.Routing != RoutingFile && config.Output.Routing != RoutingDirectory {
		t.Fatalf("Routing = %q", config.Output.Routing)
	}
	defaults := config.Defaults
	switch defaults.Permalink {
	case PermalinkPath, PermalinkSlug, PermalinkDate:
	default:
		t.Fatalf("Permalink = %q", defaults.Permalink)
	}
	if defaults.Variables["permalink"] != string(defaults.Permalink) {
		t.Fatalf("permalink variable %#v differs from %q", defaults.Variables["permalink"], defaults.Permalink)
	}
	for _, builtIn := range []string{"layout", "title", "author", "date"} {
		if _, ok := defaults.Variables[builtIn]; !ok {
			t.Fatalf("built-in default %q is missing", builtIn)
		}
	}
	if _, ok := defaults.Variables["layout"].(string); !ok {
		t.Fatalf("layout is %#v, not text", defaults.Variables["layout"])
	}

	input := config.Input
	inputDirs := append(append(append([]string{}, input.TemplateDirs.Dirs...), input.DataDirs.Dirs...), input.PartialDirs.Dirs...)
	deletedDirs := []string{config.Output.Dir, config.Output.TempDir()}
	allPaths := append(append(append([]string{input.ContentDir}, inputDirs...), deletedDirs...), settings.PluginPaths...)
	for _, written := range allPaths {
		if written == "" || path.Clean(written) != written || strings.HasPrefix(written, "/") || hasParentElement(written) {
			t.Fatalf("path %q is not clean and relative", written)
		}
	}
	directories := pathComparer{ignoresLetterCase: config.IgnoresLetterCase}
	for _, role := range append(append([]string{}, inputDirs...), deletedDirs...) {
		if directories.isWithin(input.ContentDir, role) {
			t.Fatalf("role directory %q is or contains content %q", role, input.ContentDir)
		}
	}
	for _, deleted := range deletedDirs {
		for _, kept := range append(append([]string{}, inputDirs...), settings.PluginPaths...) {
			if directories.isWithin(kept, deleted) {
				t.Fatalf("deleted directory %q holds %q", deleted, kept)
			}
		}
		for _, inputDir := range inputDirs {
			if directories.isWithin(deleted, inputDir) {
				t.Fatalf("deleted directory %q is inside %q", deleted, inputDir)
			}
		}
	}
	// The positions kept for package scan must be printable too.
	positions := []Position{input.ContentDirPosition}
	for _, pattern := range config.Output.PassthroughPatterns {
		positions = append(positions, pattern.Position)
	}
	for _, position := range positions {
		checkErrorPosition(t, position.Errorf("x").(*diag.Error), sourcetext.TrimByteOrderMark(src))
	}
}

// checkErrorPosition checks that an error has a message and that its
// position is one the console can print: a line of the file (or the one
// just after it, for an error at the end), and a column inside that line or
// just past its end, at the start of a character.
func checkErrorPosition(t *testing.T, diagErr *diag.Error, src []byte) {
	t.Helper()
	if diagErr.Msg == "" && diagErr.Err == nil {
		t.Fatalf("error %+v has no message", diagErr)
	}
	if diagErr.Line == 0 {
		if diagErr.Col != 0 {
			t.Fatalf("error %q has a column but no line", diagErr)
		}
		return
	}
	lines := bytes.Split(src, []byte("\n"))
	if diagErr.Line < 1 || diagErr.Line > len(lines) {
		t.Fatalf("error %q: line %d is not in a file of %d lines", diagErr, diagErr.Line, len(lines))
	}
	if diagErr.Col == 0 {
		return
	}
	line := lines[diagErr.Line-1]
	if diagErr.Col < 1 || diagErr.Col > len(line)+1 {
		t.Fatalf("error %q: column %d is not in line %q", diagErr, diagErr.Col, line)
	}
	if diagErr.Col <= len(line) && !utf8.RuneStart(line[diagErr.Col-1]) {
		t.Fatalf("error %q: column %d is inside a character of line %q", diagErr, diagErr.Col, line)
	}
}
