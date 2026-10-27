package scan

import (
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
)

func mustCompile(t *testing.T, text string, ignoresLetterCase bool) pattern {
	t.Helper()
	compiled, err := compilePattern(config.PassthroughPattern{Pattern: text}, ignoresLetterCase)
	if err != nil {
		t.Fatalf("compilePattern(%q): %v", text, err)
	}
	return compiled
}

func levelsOf(slashPath string) []string {
	if slashPath == "" {
		return nil
	}
	return strings.Split(slashPath, "/")
}

func TestPatternMatchesFile(t *testing.T) {
	tests := []struct {
		pattern           string
		filePath          string
		ignoresLetterCase bool
		want              bool
	}{
		{"robots.txt", "robots.txt", false, true},
		{"/robots.txt", "robots.txt", false, true},
		{"robots.txt", "sub/robots.txt", false, false},
		{"robots.txt", "Robots.txt", false, false},
		{"robots.txt", "Robots.TXT", true, true},
		{"Robots.txt", "robots.txt", true, true},
		{"*.ico", "favicon.ico", false, true},
		{"*.ico", "img/favicon.ico", false, false},
		{"*", "a", false, true},
		{"*", "a/b", false, false},
		{"img/*", "img/a.png", false, true},
		{"*/a.png", "img/a.png", false, true},
		{"?.txt", "a.txt", false, true},
		{"?.txt", "ab.txt", false, false},
		{"[ab].txt", "b.txt", false, true},
		{"[^ab].txt", "b.txt", false, false},
		{"[A-C].txt", "b.txt", true, true},
		{"[A-C].txt", "b.txt", false, false},
		{"assets/", "assets/a.png", false, true},
		{"assets/", "assets/fonts/deep/a.woff2", false, true},
		{"assets/", "assets", false, false},
		{"assets/", "assetsx/a.png", false, false},
		{"/assets/", "Assets/a.png", true, true},
		{"*/", "a/b.txt", false, true},
		{"*/", "b.txt", false, false},
		{"a/*/", "a/b/c.txt", false, true},
		{"a/*/", "a/c.txt", false, false},
		{"./a//b/./c.txt", "a/b/c.txt", false, true},
		{"città.md", "CITTÀ.md", true, true},
		{"città.md", "CITTÀ.md", false, false},
	}
	for _, test := range tests {
		compiled := mustCompile(t, test.pattern, test.ignoresLetterCase)
		if got := compiled.matchesFile(levelsOf(test.filePath)); got != test.want {
			t.Errorf("%q (ignoring case: %v) matches file %q = %v, want %v", test.pattern, test.ignoresLetterCase, test.filePath, got, test.want)
		}
	}
}

func TestPatternMayMatchBelow(t *testing.T) {
	tests := []struct {
		pattern string
		dirPath string
		want    bool
	}{
		{"robots.txt", "", true},
		{"robots.txt", "sub", false},
		{"a/b/c.txt", "a", true},
		{"a/b/c.txt", "a/b", true},
		{"a/b/c.txt", "a/b/c.txt", false},
		{"a/b/c.txt", "x", false},
		{"*/c.txt", ".git", true},
		{".w*/", ".git", false},
		{".w*/", ".well-known", true},
		{".w*/", ".well-known/deep/er", true},
		{"assets/", "assets/fonts", true},
		{"assets/fonts/", "assets", true},
		{"assets/fonts/", "assets/img", false},
	}
	for _, test := range tests {
		compiled := mustCompile(t, test.pattern, false)
		if got := compiled.mayMatchBelow(levelsOf(test.dirPath)); got != test.want {
			t.Errorf("%q may match below %q = %v, want %v", test.pattern, test.dirPath, got, test.want)
		}
	}
}

func TestPatternMatchesDirectoryItself(t *testing.T) {
	tests := []struct {
		pattern string
		dirPath string
		want    bool
	}{
		{".well-known", ".well-known", true},
		{"docs/*", "docs/a", true},
		{"docs/*", "docs", false},
		{".well-known/", ".well-known", false},
	}
	for _, test := range tests {
		compiled := mustCompile(t, test.pattern, false)
		if got := compiled.matchesDirectoryItself(levelsOf(test.dirPath)); got != test.want {
			t.Errorf("%q matches directory %q = %v, want %v", test.pattern, test.dirPath, got, test.want)
		}
	}
}

// TestCompilePatternDriveInSmallLetters covers the Kelvin sign (U+212A),
// whose small letter is an ASCII "k": ignoring letter case, "\u212a:" is
// matched as the drive "k:", so it is refused then, and only then.
func TestCompilePatternDriveInSmallLetters(t *testing.T) {
	kelvinDrive := config.PassthroughPattern{Pattern: "\u212a:"}
	if _, err := compilePattern(kelvinDrive, false); err != nil {
		t.Errorf("comparing exactly: %v", err)
	}
	_, err := compilePattern(kelvinDrive, true)
	want := "gondola.yaml: output.passthrough \"\u212a:\" " + `is not allowed: patterns are relative to the project directory and may not contain ".." or be absolute`
	if err == nil || err.Error() != want {
		t.Errorf("ignoring case, error:\n got %v\nwant %s", err, want)
	}
}

func TestCompilePatternErrorWithoutPosition(t *testing.T) {
	// A pattern from a default would have no position; none exists today,
	// but the message must still name gondola.yaml.
	_, err := compilePattern(config.PassthroughPattern{Pattern: "../x"}, false)
	want := `gondola.yaml: output.passthrough "../x" is not allowed: patterns are relative to the project directory and may not contain ".." or be absolute`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}
