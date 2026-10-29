package sass

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/scan"
)

// update rewrites the golden files instead of comparing with them:
// go test ./internal/sass -run TestGolden -update. Read the diff of the
// files before keeping it. The CSS is Dart Sass's; another version of Dart
// Sass may format it differently.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden/want")

const goldenSiteDir = "testdata/golden/site"

// TestGolden compiles the entry points that scan finds in the fixture site,
// in both styles, and compares every file written with testdata/golden/want.
func TestGolden(t *testing.T) {
	executablePath := requireSass(t)
	cfg, err := config.Load(goldenSiteDir)
	if err != nil {
		t.Fatal(err)
	}
	scanned, err := scan.Scan(goldenSiteDir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		style OutputStyle
	}{
		{"compressed", Compressed},
		{"expanded", Expanded},
	} {
		t.Run(test.name, func(t *testing.T) {
			compiler, err := Start(Options{
				ProjectDir:     goldenSiteDir,
				ExecutablePath: executablePath,
				Input:          cfg.Input,
				Style:          test.style,
				PrintWarning:   func(text string) { t.Errorf("unexpected warning: %s", text) },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer closeCompiler(t, compiler)
			outputDir := t.TempDir()
			compileErrors, err := compiler.CompileAll(scanned.SassEntryPoints, outputDir)
			if err != nil || len(compileErrors) > 0 {
				t.Fatalf("errors %v, %v", err, compileErrors)
			}
			compareTrees(t, outputDir, filepath.Join("testdata/golden/want", test.name))
		})
	}
}

func closeCompiler(t *testing.T, compiler *Compiler) {
	t.Helper()
	if err := compiler.Close(); err != nil {
		t.Error(err)
	}
}

// compareTrees compares every file of gotDir with the file at the same path
// in wantDir, and fails for files that only one of them has. With -update
// it replaces wantDir with gotDir instead.
func compareTrees(t *testing.T, gotDir, wantDir string) {
	t.Helper()
	got := readTree(t, gotDir)
	if *update {
		if err := os.RemoveAll(wantDir); err != nil {
			t.Fatal(err)
		}
		for name, content := range got {
			wantPath := filepath.Join(wantDir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(wantPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(wantPath, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	want := readTree(t, wantDir)
	for _, name := range sortedKeys(want) {
		gotContent, ok := got[name]
		if !ok {
			t.Errorf("%s was not written", name)
			continue
		}
		if gotContent != want[name] {
			t.Errorf("%s differs\n got %q\nwant %q", name, gotContent, want[name])
		}
	}
	for _, name := range sortedKeys(got) {
		if _, ok := want[name]; !ok {
			t.Errorf("%s was written but is not expected (run with -update to accept it)", name)
		}
	}
}

// readTree returns the files under dir by their path relative to dir, with
// "/". A directory that does not exist reads as empty.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relativePath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relativePath)] = string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return files
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
