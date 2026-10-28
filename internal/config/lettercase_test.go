package config

import (
	"os"
	"path/filepath"
	"testing"
)

// fileSystemIgnoresCase finds out, independently of ignoresLetterCase,
// whether the file system of dir ignores letter case: it writes a file in
// small letters and looks for it in capitals.
func fileSystemIgnoresCase(t *testing.T, dir string) bool {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(filepath.Join(dir, "probe"))
	_, err := os.Stat(filepath.Join(dir, "PROBE"))
	return err == nil
}

func TestIgnoresLetterCase(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, FileName), []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fileSystemIgnoresCase(t, t.TempDir())
	if got := ignoresLetterCase(projectDir); got != want {
		t.Errorf("ignoresLetterCase = %v, want %v", got, want)
	}
	if got := ignoresLetterCase(filepath.Join(projectDir, "missing")); got {
		t.Errorf("ignoresLetterCase of a missing directory = true, want false")
	}
}

func TestIgnoresLetterCaseWithBothSpellings(t *testing.T) {
	projectDir := t.TempDir()
	if fileSystemIgnoresCase(t, projectDir) {
		t.Skip("this file system cannot hold gondola.yaml and GONDOLA.YAML side by side")
	}
	for _, name := range []string{FileName, "GONDOLA.YAML"} {
		if err := os.WriteFile(filepath.Join(projectDir, name), []byte(minimal), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if ignoresLetterCase(projectDir) {
		t.Errorf("two different files were taken for one")
	}
}

func TestLoadRecordsLetterCase(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, FileName), []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if want := fileSystemIgnoresCase(t, t.TempDir()); got.IgnoresLetterCase != want {
		t.Errorf("IgnoresLetterCase = %v, want %v", got.IgnoresLetterCase, want)
	}
}

// TestParseIgnoringLetterCase covers directories that are the same only on
// a file system that ignores letter case. Each file is accepted when the
// letters are compared exactly.
func TestParseIgnoringLetterCase(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"role directory is the content directory",
			withMinimal("input:\n  content: Site\n  templates: site\n"),
			`gondola.yaml:7:14: input.templates "site" is the content directory; role directories may be inside it but must not be it or contain it`},
		{"role directory contains the content directory",
			withMinimal("input:\n  content: site/src\n  templates: SITE\n"),
			`gondola.yaml:7:14: input.templates "SITE" contains the content directory "site/src"; role directories may be inside it but must not be it or contain it`},
		{"output is an input directory",
			withMinimal("output:\n  directory: _Data\n"),
			`gondola.yaml:6:14: output.directory "_Data" is also used by input.data; every build deletes it, so please choose another output directory`},
		{"output contains an input directory",
			withMinimal("input:\n  data: Out/data\noutput:\n  directory: out\n"),
			`gondola.yaml:8:14: output.directory "out" contains input.data "Out/data"; every build deletes it, so please choose another output directory`},
		{"output is inside an input directory",
			withMinimal("input:\n  partials: Styles\noutput:\n  directory: styles/out\n"),
			`gondola.yaml:8:14: output.directory "styles/out" is inside input.partials "Styles"; please choose an output directory outside it`},
		{"data directory inside another",
			withMinimal("input:\n  data: [Shared, shared/blog]\n"),
			`gondola.yaml:6:18: input.data "shared/blog" is inside input.data "Shared"; please list only one`},
		{"output contains a plugin",
			withMinimal("  plugin: [_SITE/minify.sh]\n"),
			`gondola.yaml:5:12: output.directory "_site" (the default) contains config.plugin "_SITE/minify.sh"; every build deletes it, so please move the plugin or choose another output directory`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			exact, err := parseFile([]byte(test.src), false)
			if err != nil {
				t.Fatalf("comparing exactly: %v", err)
			}
			if exact.IgnoresLetterCase {
				t.Errorf("IgnoresLetterCase = true when comparing exactly")
			}
			_, err = parseFile([]byte(test.src), true)
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestParseIgnoringLetterCaseRecordsIt(t *testing.T) {
	got, err := parseFile([]byte(minimal), true)
	if err != nil {
		t.Fatal(err)
	}
	want := defaultConfig()
	want.IgnoresLetterCase = true
	checkConfig(t, got, want)
}

func TestPositionErrorf(t *testing.T) {
	tests := []struct {
		position Position
		want     string
	}{
		{Position{Line: 3, Col: 5}, `gondola.yaml:3:5: input.content "src" does not exist`},
		{Position{}, `gondola.yaml: input.content "src" does not exist`},
	}
	for _, test := range tests {
		if got := test.position.Errorf("input.content %q does not exist", "src").Error(); got != test.want {
			t.Errorf("Errorf at %+v:\n got %s\nwant %s", test.position, got, test.want)
		}
	}
}

// TestParseKeptPositions checks that the positions kept for package scan
// count lines and columns as the console does, not as yaml.v3 does: yaml.v3
// also starts a line at LS (U+2028) and counts characters, not bytes.
func TestParseKeptPositions(t *testing.T) {
	src := withMinimal("  description: \"a\u2028b\"\ninput:\n  content: src\noutput:\n  passthrough: [città, \"b\"]\n")
	got := mustParse(t, src)
	if want := (Position{Line: 7, Col: 12}); got.Input.ContentDirPosition != want {
		t.Errorf("ContentDirPosition = %+v, want %+v", got.Input.ContentDirPosition, want)
	}
	want := []PassthroughPattern{
		{Pattern: "città", Position: Position{Line: 9, Col: 17}},
		// "città" takes 6 bytes, one more than its characters.
		{Pattern: "b", Position: Position{Line: 9, Col: 25}},
	}
	checkConfig(t, Config{Output: Output{PassthroughPatterns: got.Output.PassthroughPatterns}}, Config{Output: Output{PassthroughPatterns: want}})
}
