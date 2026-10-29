package build

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/console"
	"github.com/luciaduck/gondola/internal/sass"
)

// testVersion is the Gondola version that test builds print.
const testVersion = "v0.0.1"

// testBuildDuration is how long every test build measures itself, so that
// the last line is always "… in 0.04 seconds …".
const testBuildDuration = 40 * time.Millisecond

// minimalConfig is the smallest valid gondola.yaml, with Sass off so that
// tests that do not need Dart Sass run without it.
const minimalConfig = "config:\n  site-name: Test\n  version: 1\n  canonical-url: https://example.com\n  sass: false\n"

// buildResult is what a test build printed and returned.
type buildResult struct {
	failure *Failure
	stdout  string
	stderr  string
}

// errorTexts returns the messages of the failure's errors, or nil after a
// success.
func (r buildResult) errorTexts() []string {
	if r.failure == nil {
		return nil
	}
	texts := []string{}
	for _, err := range r.failure.Errors {
		texts = append(texts, err.Error())
	}
	return texts
}

// runBuild builds the project in projectDir with a fake clock, printing
// into buffers without colors.
func runBuild(t testing.TB, projectDir string) buildResult {
	t.Helper()
	var stdout, stderr strings.Builder
	printer := console.New(console.Stream{Writer: &stdout}, console.Stream{Writer: &stderr}, os.DirFS(projectDir))
	failure := Run(Options{
		ProjectDir:     projectDir,
		GondolaVersion: testVersion,
		SassStyle:      sass.Compressed,
		Printer:        printer,
		Now:            fakeClock(testBuildDuration),
	})
	return buildResult{failure: failure, stdout: stdout.String(), stderr: stderr.String()}
}

// fakeClock returns a clock whose every reading is step later than the one
// before, so a build, which reads it at the start and at the end, always
// measures step.
func fakeClock(step time.Duration) func() time.Time {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		current := now
		now = now.Add(step)
		return current
	}
}

// writeProject creates a project in a new directory called "site" (so that
// the first line of the log is always the same) and returns its path. files
// maps paths written with "/" to their contents; gondola.yaml is
// minimalConfig unless files holds one.
func writeProject(t testing.TB, files map[string]string) string {
	t.Helper()
	projectDir := filepath.Join(t.TempDir(), "site")
	if _, ok := files["gondola.yaml"]; !ok {
		writeFile(t, filepath.Join(projectDir, "gondola.yaml"), minimalConfig)
	}
	for name, content := range files {
		writeFile(t, filepath.Join(projectDir, filepath.FromSlash(name)), content)
	}
	return projectDir
}

func writeFile(t testing.TB, diskPath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readTree returns every file under dir, keyed by its path relative to dir
// written with "/". A missing dir gives nil.
func readTree(t testing.TB, dir string) map[string]string {
	t.Helper()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(diskPath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(diskPath)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, diskPath)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// copyTree copies every file under sourceDir to destinationDir.
func copyTree(t testing.TB, sourceDir, destinationDir string) {
	t.Helper()
	for name, content := range readTree(t, sourceDir) {
		writeFile(t, filepath.Join(destinationDir, filepath.FromSlash(name)), content)
	}
}

// sortedKeys returns the keys of files in order, for messages.
func sortedKeys(files map[string]string) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// requireSass skips the test when Dart Sass is not installed.
func requireSass(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("sass"); err != nil {
		t.Skip("Dart Sass is not installed")
	}
}

// dirExists reports whether diskPath is an existing directory.
func dirExists(t testing.TB, diskPath string) bool {
	t.Helper()
	info, err := os.Stat(diskPath)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return info.IsDir()
}
