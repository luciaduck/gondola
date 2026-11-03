package build

import (
	"fmt"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/plugin/plugintest"
)

// maxFuzzFileNameBytes keeps the file names that FuzzBuildPlugin writes
// well below the 255 bytes that Linux allows in one name.
const maxFuzzFileNameBytes = 120

// fuzzBuildPluginPath is the plugin of FuzzBuildPlugin's project.
var fuzzBuildPluginPath = plugintest.ProgramPath("tools/fuzz")

// isFuzzableOutputFile reports whether name can be a file that a plugin
// writes in the temp output: a clean relative path written with "/",
// without "..".
func isFuzzableOutputFile(name string) bool {
	if name == "" || name == "." || len(name) > maxFuzzFileNameBytes || !utf8.ValidString(name) ||
		strings.ContainsRune(name, 0) || path.IsAbs(name) || path.Clean(name) != name {
		return false
	}
	for _, element := range strings.Split(name, "/") {
		if element == ".." {
			return false
		}
	}
	return true
}

// FuzzBuildPlugin lets a plugin write any file into the temp output,
// perhaps delete a page, and exit with any code. On success the published
// output is the site plus the plugin's changes, the temp output is gone and
// the last line keeps Gondola's own counts (M11, Q6 and Q7); on failure the
// error names the plugin, nothing is published, and the temp output holds
// the changes.
func FuzzBuildPlugin(f *testing.F) {
	f.Add("feed.json", "{}", false, uint8(0))
	f.Add("index.html", "<p>minified</p>", false, uint8(0))
	f.Add("sitemap.xml", "", true, uint8(0))
	f.Add("deep/er/città $x.txt", "\x00\xff", true, uint8(1))
	f.Add("_site", "x", false, uint8(255))
	projectDir := writeProject(f, map[string]string{
		"gondola.yaml": minimalConfig + "  sitemap: true\n  plugin: [" + fuzzBuildPluginPath + "]\n",
		"index.html":   "<p>home</p>\n",
		"about.html":   "<p>about</p>\n",
	})
	plugintest.Install(f, testProgram, projectDir, fuzzBuildPluginPath, plugintest.Behavior{})
	// The site as Gondola builds it, before any plugin changes it.
	baseline := runBuild(f, projectDir)
	if baseline.failure != nil {
		f.Fatalf("the baseline build failed: %q", baseline.errorTexts())
	}
	siteFiles := readTree(f, filepath.Join(projectDir, "_site"))
	f.Fuzz(func(t *testing.T, fileName, content string, deleteIndex bool, exitCode uint8) {
		if !isFuzzableOutputFile(fileName) || int(exitCode) == plugintest.HelperFailureCode {
			t.Skip()
		}
		// A file of the site cannot also be a directory.
		for _, element := range strings.Split(path.Dir(fileName), "/") {
			if _, ok := siteFiles[element]; ok {
				t.Skip()
			}
		}
		behavior := plugintest.Behavior{WriteFiles: map[string][]byte{fileName: []byte(content)}, ExitCode: int(exitCode)}
		want := maps.Clone(siteFiles)
		want[fileName] = content
		if deleteIndex {
			behavior.DeleteFiles = []string{"index.html"}
			delete(want, "index.html")
		}
		plugintest.WriteBehavior(t, projectDir, fuzzBuildPluginPath, behavior)
		result := runBuild(t, projectDir)
		executing := `\gondola/ Executing ` + fuzzBuildPluginPath + " in _site.temp\n"
		if !strings.Contains(result.stdout, executing) {
			t.Errorf("the log has no Executing line:\n%s", result.stdout)
		}
		if exitCode != 0 {
			wantErrors := []string{fmt.Sprintf("plugin %q failed: exit status %d", fuzzBuildPluginPath, exitCode)}
			if !reflect.DeepEqual(result.errorTexts(), wantErrors) {
				t.Fatalf("errors %q, want %q\n%s", result.errorTexts(), wantErrors, result.stderr)
			}
			checkNotPublished(t, projectDir, result)
			if got := readTree(t, filepath.Join(projectDir, "_site.temp")); !reflect.DeepEqual(got, want) {
				t.Errorf("temp output %q, want %q", got, want)
			}
			return
		}
		if result.failure != nil {
			t.Fatalf("build failed: %q\n%s", result.errorTexts(), result.stderr)
		}
		if got := readTree(t, filepath.Join(projectDir, "_site")); !reflect.DeepEqual(got, want) {
			t.Errorf("output %q, want %q", got, want)
		}
		if dirExists(t, filepath.Join(projectDir, "_site.temp")) {
			t.Error("the temp output still exists after publishing")
		}
		wantEnd := executing + "\\gondola/ Shipping _site.temp to _site\n" +
			"\\gondola/ Built 2 files and Copied 0 in 0.04 seconds (20.00ms each, v0.0.1)\n"
		if !strings.HasSuffix(result.stdout, wantEnd) {
			t.Errorf("log differs\n got:\n%s\nwant it to end with:\n%s", result.stdout, wantEnd)
		}
	})
}
