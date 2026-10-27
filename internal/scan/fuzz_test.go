package scan

import (
	"errors"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
)

// FuzzPattern checks passthrough patterns against file paths. A pattern
// must never panic, never let a path leave the project root, agree with
// matching in small letters when it ignores letter case, and match no file
// whose directories the walk would not enter.
func FuzzPattern(f *testing.F) {
	seeds := []struct {
		pattern  string
		filePath string
	}{
		{"robots.txt", "robots.txt"},
		{"/robots.txt", "sub/robots.txt"},
		{"*.ico", "favicon.ico"},
		{"assets/", "assets/fonts/a.woff2"},
		{".well-known/", ".well-known/security.txt"},
		{"docs/*/", "docs/a/b/c.txt"},
		{"*/c.txt", ".git/c.txt"},
		{"icon-[0-9].png", "icon-1.png"},
		{"[^a]", "/"},
		{"[Z-a]", "b"},
		{"Città/", "città/x"},
		{"./a//b/", "a/b/c"},
		{"../x", "x"},
		{"//x", "x"},
		{"C:/x", "x"},
		{"a\\b", "a/b"},
		{"**", "a"},
		{"a[", "a"},
		{"/", "a"},
		{"\u212a.txt", "k.txt"},
	}
	for _, seed := range seeds {
		f.Add(seed.pattern, seed.filePath, false)
		f.Add(seed.pattern, seed.filePath, true)
	}
	f.Fuzz(func(t *testing.T, text, filePath string, ignoresLetterCase bool) {
		position := config.Position{Line: 6, Col: 17}
		compiled, err := compilePattern(config.PassthroughPattern{Pattern: text, Position: position}, ignoresLetterCase)
		if err != nil {
			var diagErr *diag.Error
			if !errors.As(err, &diagErr) || diagErr.File != config.FileName || diagErr.Line != position.Line || diagErr.Col != position.Col {
				t.Fatalf("error %q does not point at the pattern in %s", err, config.FileName)
			}
			return
		}
		checkLevels(t, compiled)

		levels := strings.Split(filePath, "/")
		if slices.Contains(levels, "") {
			return // not a path the walk can produce
		}
		matched := compiled.matchesFile(levels)
		if matched {
			// The walk reaches a file only through directories that may
			// hold a match, so each one must say so.
			for depth := 1; depth < len(levels); depth++ {
				if !compiled.mayMatchBelow(levels[:depth]) {
					t.Fatalf("%q matches %q but not below %q, so the walk would miss it", text, filePath, path.Join(levels[:depth]...))
				}
			}
		}
		if ignoresLetterCase {
			checkFolding(t, text, filePath, matched)
		} else if !compiled.coversDirectory && !strings.Contains(text, "[") {
			// Without a character set, path.Match on the whole path agrees
			// with matching level by level: neither "*" nor "?" match "/".
			whole, err := path.Match(strings.Join(compiled.levels, "/"), filePath)
			if err != nil || whole != matched {
				t.Fatalf("%q on %q: level by level %v, whole path %v (%v)", text, filePath, matched, whole, err)
			}
		}
	})
}

// checkLevels checks that a compiled pattern stays inside the project root.
func checkLevels(t *testing.T, compiled pattern) {
	t.Helper()
	if len(compiled.levels) == 0 {
		t.Fatalf("%q has no levels", compiled.written)
	}
	for _, level := range compiled.levels {
		if level == "" || level == "." || level == ".." || strings.Contains(level, "/") || strings.Contains(level, `\`) {
			t.Fatalf("%q has the level %q", compiled.written, level)
		}
	}
	if hasDriveLetter(compiled.levels[0]) || hasDriveLetter(path.Clean(strings.TrimPrefix(compiled.written, "/"))) {
		t.Fatalf("%q starts with a drive", compiled.written)
	}
}

// checkFolding checks that ignoring letter case is the same as matching the
// pattern and the path in small letters.
func checkFolding(t *testing.T, text, filePath string, matched bool) {
	t.Helper()
	lower, err := compilePattern(config.PassthroughPattern{Pattern: strings.ToLower(text)}, false)
	if err != nil {
		t.Fatalf("%q is accepted but %q is not: %v", text, strings.ToLower(text), err)
	}
	if want := lower.matchesFile(strings.Split(strings.ToLower(filePath), "/")); matched != want {
		t.Fatalf("%q on %q ignoring case: %v, but %v in small letters", text, filePath, matched, want)
	}
}
