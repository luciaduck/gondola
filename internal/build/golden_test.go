package build

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update rewrites the golden files instead of comparing with them:
// go test ./internal/build -run TestGolden -update. Read the diff of the
// rewritten files before keeping them. The CSS is Dart Sass's; another
// version of Dart Sass may format it differently.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// The golden project, the output it must produce, and its log.
const (
	goldenSiteDir = "testdata/golden/site"
	goldenWantDir = "testdata/golden/want"
	goldenLogPath = "testdata/golden/want.log"
)

// TestGolden builds a copy of the fixture site and compares the published
// output and the log with the golden files. The site uses every feature of
// the stages built so far, including Sass, so the test needs Dart Sass.
func TestGolden(t *testing.T) {
	requireSass(t)
	projectDir := filepath.Join(t.TempDir(), "site")
	copyTree(t, goldenSiteDir, projectDir)
	result := runBuild(t, projectDir)
	if result.failure != nil {
		t.Fatalf("build failed: %q\n%s", result.errorTexts(), result.stderr)
	}
	if result.stderr != "" {
		t.Errorf("standard error is not empty:\n%s", result.stderr)
	}
	if dirExists(t, filepath.Join(projectDir, "_site.temp")) {
		t.Error("the temp output still exists after publishing")
	}
	got := readTree(t, filepath.Join(projectDir, "_site"))
	if *update {
		if err := os.RemoveAll(goldenWantDir); err != nil {
			t.Fatal(err)
		}
		for name, content := range got {
			writeFile(t, filepath.Join(goldenWantDir, filepath.FromSlash(name)), content)
		}
		writeFile(t, goldenLogPath, result.stdout)
		return
	}
	want := readTree(t, goldenWantDir)
	for _, name := range sortedKeys(want) {
		if gotContent, ok := got[name]; !ok {
			t.Errorf("%s is missing from the output", name)
		} else if gotContent != want[name] {
			t.Errorf("%s differs:\n got:\n%s\nwant:\n%s", name, gotContent, want[name])
		}
	}
	for _, name := range sortedKeys(got) {
		if _, ok := want[name]; !ok {
			t.Errorf("%s is in the output but not in %s", name, goldenWantDir)
		}
	}
	wantLog, err := os.ReadFile(goldenLogPath)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if result.stdout != string(wantLog) {
		t.Errorf("log differs:\n got:\n%s\nwant:\n%s", result.stdout, wantLog)
	}
}

// TestGoldenIsDeterministic builds the fixture site twice and compares the
// two outputs byte for byte (SPEC §4).
func TestGoldenIsDeterministic(t *testing.T) {
	requireSass(t)
	outputs := []map[string]string{}
	logs := []string{}
	for range 2 {
		projectDir := filepath.Join(t.TempDir(), "site")
		copyTree(t, goldenSiteDir, projectDir)
		result := runBuild(t, projectDir)
		if result.failure != nil {
			t.Fatalf("build failed: %q", result.errorTexts())
		}
		outputs = append(outputs, readTree(t, filepath.Join(projectDir, "_site")))
		logs = append(logs, result.stdout)
	}
	if len(outputs[0]) != len(outputs[1]) {
		t.Fatalf("the builds wrote %d and %d files", len(outputs[0]), len(outputs[1]))
	}
	for name, content := range outputs[0] {
		if outputs[1][name] != content {
			t.Errorf("%s differs between two builds", name)
		}
	}
	if logs[0] != logs[1] {
		t.Errorf("the logs differ:\n%s\n%s", logs[0], logs[1])
	}
}
