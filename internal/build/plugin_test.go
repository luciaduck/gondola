package build

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/plugin/plugintest"
)

// testProgram is the path of the plugin program that the tests install,
// built once by TestMain.
var testProgram string

func TestMain(m *testing.M) {
	programPath, err := plugintest.BuildProgram()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testProgram = programPath
	code := m.Run()
	os.RemoveAll(filepath.Dir(programPath))
	os.Exit(code)
}

// pluginConfig is minimalConfig with the sitemap on, one passthrough file,
// and the plugins tools/first and tools/second.
const pluginConfig = minimalConfig + "  sitemap: true\n  plugin: [tools/first, tools/second]\n" +
	"output:\n  passthrough: [favicon.ico]\n"

// TestBuildPlugins checks stage 8 (SPEC §5.13): the plugins run in order
// on the finished temp output, sitemap included, after the sitemap line and
// before Shipping; their output is in the log; what they change is
// published as it is; and the counts of the last line do not change.
func TestBuildPlugins(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml": pluginConfig,
		"index.html":   "<p>home</p>\n",
		"about.html":   "<p>about</p>\n",
		"favicon.ico":  "icon",
	})
	recordPath := filepath.Join(t.TempDir(), "records")
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{
		Name:       "first",
		RecordPath: recordPath,
		WriteFiles: map[string][]byte{"index.html": []byte("<p>minified</p>"), "feeds/feed.json": []byte("{}")},
		Stdout:     []byte("first says hello\n"),
	})
	plugintest.Install(t, testProgram, projectDir, "tools/second", plugintest.Behavior{
		Name:            "second",
		RecordPath:      recordPath,
		RecordVariables: []string{"GONDOLA_OUTPUT", "GONDOLA_MODE"},
		ListOutput:      true,
		DeleteFiles:     []string{"about.html"},
	})
	result := runBuild(t, projectDir)
	if result.failure != nil {
		t.Fatalf("build failed: %q\n%s", result.errorTexts(), result.stderr)
	}
	wantLog := `> site is taking a gondola ride
\gondola/ Shaping ./_site.temp/about.html from ./about.html
\gondola/ Shaping ./_site.temp/index.html from ./index.html
(sitemap) 2 pages were listed (there is no robots.txt)
\gondola/ Executing tools/first in _site.temp
first says hello
\gondola/ Executing tools/second in _site.temp
\gondola/ Shipping _site.temp to _site
\gondola/ Built 2 files and Copied 1 in 0.04 seconds (13.33ms each, v0.0.1)
`
	if result.stdout != wantLog {
		t.Errorf("log differs\n got:\n%s\nwant:\n%s", result.stdout, wantLog)
	}
	output := readTree(t, filepath.Join(projectDir, "_site"))
	wantFiles := []string{"favicon.ico", "feeds/feed.json", "index.html", "sitemap.xml"}
	if got := sortedKeys(output); !reflect.DeepEqual(got, wantFiles) {
		t.Errorf("output files %q, want %q", got, wantFiles)
	}
	if got, want := output["index.html"], "<p>minified</p>"; got != want {
		t.Errorf("index.html = %q, want the plugin's %q", got, want)
	}
	// Gondola does not check what the plugins did: the deleted page stays
	// in the sitemap (M11, Q6).
	if !strings.Contains(output["sitemap.xml"], "<loc>https://example.com/about.html</loc>") {
		t.Errorf("sitemap.xml lost the page that a plugin deleted:\n%s", output["sitemap.xml"])
	}
	records := plugintest.ReadRecords(t, recordPath)
	if len(records) != 2 || records[0].Name != "first" || records[1].Name != "second" {
		t.Fatalf("records %+v, want first then second", records)
	}
	absoluteTempDir := filepath.Join(projectDir, "_site.temp")
	second := records[1]
	if want := []string{absoluteTempDir}; !reflect.DeepEqual(second.Args, want) {
		t.Errorf("arguments %q, want %q", second.Args, want)
	}
	wantVariables := map[string]string{"GONDOLA_OUTPUT": absoluteTempDir, "GONDOLA_MODE": "ride"}
	if !reflect.DeepEqual(second.Variables, wantVariables) {
		t.Errorf("variables %v, want %v", second.Variables, wantVariables)
	}
	wantSeen := []string{"about.html", "favicon.ico", "feeds/feed.json", "index.html", "sitemap.xml"}
	if !reflect.DeepEqual(second.OutputFiles, wantSeen) {
		t.Errorf("the second plugin saw %q, want %q", second.OutputFiles, wantSeen)
	}
}

// TestBuildPluginFails checks that a failing plugin stops the build: the
// error names it, the plugins after it do not run, and the output is not
// published; the temp output keeps what the plugins changed.
func TestBuildPluginFails(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml":     pluginConfig,
		"index.html":       "<p>home</p>\n",
		"favicon.ico":      "icon",
		"_site/index.html": "<p>old</p>\n",
	})
	recordPath := filepath.Join(t.TempDir(), "records")
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{
		Name:       "first",
		RecordPath: recordPath,
		WriteFiles: map[string][]byte{"half-done.txt": []byte("x")},
		Stderr:     []byte("first: something went wrong\n"),
		ExitCode:   3,
	})
	plugintest.Install(t, testProgram, projectDir, "tools/second", plugintest.Behavior{Name: "second", RecordPath: recordPath})
	result := runBuild(t, projectDir)
	if want := []string{`plugin "tools/first" failed: exit status 3`}; !reflect.DeepEqual(result.errorTexts(), want) {
		t.Errorf("errors %q, want %q", result.errorTexts(), want)
	}
	checkNotPublished(t, projectDir, result)
	if want := "first: something went wrong\n"; result.stderr != want {
		t.Errorf("stderr %q, want %q", result.stderr, want)
	}
	if strings.Contains(result.stdout, "tools/second") {
		t.Errorf("the second plugin was announced after the first failed:\n%s", result.stdout)
	}
	if records := plugintest.ReadRecords(t, recordPath); len(records) != 1 {
		t.Errorf("records %+v, want only the first plugin's", records)
	}
	wantFiles := []string{"favicon.ico", "half-done.txt", "index.html", "sitemap.xml"}
	if got := sortedKeys(readTree(t, filepath.Join(projectDir, "_site.temp"))); !reflect.DeepEqual(got, wantFiles) {
		t.Errorf("temp output files %q, want %q", got, wantFiles)
	}
}

// TestBuildPluginPreflight checks that a plugin that cannot run is found
// before anything is deleted, so the old output stays (M11, Q2), and that
// the error points into gondola.yaml.
func TestBuildPluginPreflight(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml":           pluginConfig,
		"index.html":             "<p>home</p>\n",
		"_site/index.html":       "<p>old</p>\n",
		"_site.temp/leftover.md": "from a failed build",
	})
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{})
	result := runBuild(t, projectDir)
	want := []string{`gondola.yaml:7:25: config.plugin "tools/second" does not exist; please check the path`}
	if !reflect.DeepEqual(result.errorTexts(), want) {
		t.Errorf("errors %q, want %q", result.errorTexts(), want)
	}
	if got, want := readTree(t, filepath.Join(projectDir, "_site")), map[string]string{"index.html": "<p>old</p>\n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("output = %q, want the old %q", got, want)
	}
	if got, want := readTree(t, filepath.Join(projectDir, "_site.temp")), map[string]string{"leftover.md": "from a failed build"}; !reflect.DeepEqual(got, want) {
		t.Errorf("temp output = %q, want the old %q", got, want)
	}
	if wantLog := "> site is taking a gondola ride\n"; result.stdout != wantLog {
		t.Errorf("log %q, want %q", result.stdout, wantLog)
	}
}

// TestBuildPluginRemovesTempOutput checks that a plugin that deletes the
// temp output makes publishing fail (M11, Q6).
func TestBuildPluginRemovesTempOutput(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml": minimalConfig + "  plugin: [tools/first]\n",
		"index.html":   "<p>home</p>\n",
	})
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{RemoveOutput: true})
	result := runBuild(t, projectDir)
	want := []string{"renaming _site.temp to _site: no such file or directory"}
	if !reflect.DeepEqual(result.errorTexts(), want) {
		t.Errorf("errors %q, want %q", result.errorTexts(), want)
	}
	if dirExists(t, filepath.Join(projectDir, "_site")) {
		t.Error("the output directory exists after a failed build")
	}
}
