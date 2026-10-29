package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/console"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{
			name:       "no arguments prints the help to standard error",
			args:       nil,
			wantCode:   exitUsageError,
			wantStderr: helpText,
		},
		{
			name:       "--help",
			args:       []string{"--help"},
			wantCode:   exitSuccess,
			wantStdout: helpText,
		},
		{
			name:       "-h",
			args:       []string{"-h"},
			wantCode:   exitSuccess,
			wantStdout: helpText,
		},
		{
			name:       "--version",
			args:       []string{"--version"},
			wantCode:   exitSuccess,
			wantStdout: "gondola v0.0.1\n",
		},
		{
			name:       "serve is not available yet",
			args:       []string{"serve"},
			wantCode:   exitBuildError,
			wantStderr: "error: \"serve\" is not available yet\n",
		},
		{
			name:       "unknown command",
			args:       []string{"build"},
			wantCode:   exitUsageError,
			wantStderr: "error: unknown command \"build\"; run gondola --help\n",
		},
		{
			name:       "single-dash version is unknown",
			args:       []string{"-version"},
			wantCode:   exitUsageError,
			wantStderr: "error: unknown command \"-version\"; run gondola --help\n",
		},
		{
			name:       "help as a word is unknown",
			args:       []string{"help"},
			wantCode:   exitUsageError,
			wantStderr: "error: unknown command \"help\"; run gondola --help\n",
		},
		{
			name:       "empty argument",
			args:       []string{""},
			wantCode:   exitUsageError,
			wantStderr: "error: unknown command \"\"; run gondola --help\n",
		},
		{
			name:       "extra argument",
			args:       []string{"ride", "--port=8080"},
			wantCode:   exitUsageError,
			wantStderr: "error: unexpected argument \"--port=8080\"; run gondola --help\n",
		},
		{
			name:       "extra argument after --version",
			args:       []string{"--version", "now"},
			wantCode:   exitUsageError,
			wantStderr: "error: unexpected argument \"now\"; run gondola --help\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			printer := console.New(console.Stream{Writer: &stdout}, console.Stream{Writer: &stderr}, nil)
			code := run(test.args, "v0.0.1", t.TempDir(), printer)
			if code != test.wantCode {
				t.Errorf("exit code = %d, want %d", code, test.wantCode)
			}
			if stdout.String() != test.wantStdout {
				t.Errorf("standard output:\n got %q\nwant %q", stdout.String(), test.wantStdout)
			}
			if stderr.String() != test.wantStderr {
				t.Errorf("standard error:\n got %q\nwant %q", stderr.String(), test.wantStderr)
			}
		})
	}
}

func TestResolveVersion(t *testing.T) {
	withMainVersion := func(mainVersion string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Path: "github.com/luciaduck/gondola", Version: mainVersion}}
	}
	tests := []struct {
		name      string
		injected  string
		buildInfo *debug.BuildInfo
		want      string
	}{
		{"injected version wins", "v0.0.1", withMainVersion("v0.2.0"), "v0.0.1"},
		{"injected without build info", "v0.0.1", nil, "v0.0.1"},
		{"version from go install", "", withMainVersion("v0.2.0"), "v0.2.0"},
		{"pre-release from go install", "", withMainVersion("v0.2.0-beta.1"), "v0.2.0-beta.1"},
		{"source checkout", "", withMainVersion("(devel)"), "dev"},
		{"pseudo-version", "", withMainVersion("v0.0.0-20261020215731-0a874f1b5626"), "dev"},
		{"pseudo-version after a tag", "", withMainVersion("v0.1.1-0.20261020215731-0a874f1b5626"), "dev"},
		{"pseudo-version after a pre-release", "", withMainVersion("v0.2.0-beta.1.0.20261020215731-0a874f1b5626"), "dev"},
		{"pseudo-version with uncommitted changes", "", withMainVersion("v0.0.0-20261020215731-0a874f1b5626+dirty"), "dev"},
		{"tag with uncommitted changes", "", withMainVersion("v0.2.0+dirty"), "dev"},
		{"empty module version", "", withMainVersion(""), "dev"},
		{"no build info", "", nil, "dev"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveVersion(test.injected, test.buildInfo); got != test.want {
				t.Errorf("resolveVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

// builtLine matches the last line of a successful build, whose time varies.
var builtLine = regexp.MustCompile(`^\\gondola/ Built 1 file and Copied 0 in [0-9]+\.[0-9]{2} seconds \([0-9]+\.[0-9]{2}ms each, v0\.0\.1\)$`)

func TestRide(t *testing.T) {
	const config = "config:\n  site-name: Test\n  version: 1\n  canonical-url: https://example.com\n  sass: false\n"
	t.Run("success", func(t *testing.T) {
		projectDir := writeProject(t, map[string]string{"gondola.yaml": config, "index.html": "<p>hello</p>\n"})
		code, stdout, stderr := runInProject(t, projectDir, "ride")
		if code != exitSuccess || stderr != "" {
			t.Fatalf("exit code %d, standard error %q", code, stderr)
		}
		lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
		wantFirst := []string{
			"> site is taking a gondola ride",
			"\\gondola/ Shaping ./_site.temp/index.html from ./index.html",
			"\\gondola/ Shipping _site.temp to _site",
		}
		if len(lines) != 4 || strings.Join(lines[:3], "\n") != strings.Join(wantFirst, "\n") || !builtLine.MatchString(lines[3]) {
			t.Errorf("standard output:\n%s", stdout)
		}
		content, err := os.ReadFile(filepath.Join(projectDir, "_site", "index.html"))
		if err != nil || string(content) != "<p>hello</p>\n" {
			t.Errorf("_site/index.html = %q, %v", content, err)
		}
	})
	t.Run("errors are printed with their source line, then counted", func(t *testing.T) {
		projectDir := writeProject(t, map[string]string{
			"gondola.yaml": config,
			"a.md":         "---\ntitle: x\nsite: y\n---\n",
			"b.md":         "---\ntags: [news, news]\n---\n",
		})
		code, stdout, stderr := runInProject(t, projectDir, "ride")
		if code != exitBuildError {
			t.Errorf("exit code = %d, want %d", code, exitBuildError)
		}
		if stdout != "> site is taking a gondola ride\n" {
			t.Errorf("standard output = %q", stdout)
		}
		wantStderr := "error: a.md:3:1: front matter key \"site\" is reserved by Gondola (templates use it for the site settings); please rename it\n" +
			"    3 | site: y\n" +
			"      | ^\n" +
			"error: b.md:2:14: front matter key \"tags\" lists the tag \"news\" twice; please keep only one\n" +
			"    2 | tags: [news, news]\n" +
			"      |              ^\n" +
			"build failed: 2 errors\n"
		if stderr != wantStderr {
			t.Errorf("standard error:\n got:\n%s\nwant:\n%s", stderr, wantStderr)
		}
	})
	t.Run("an error outside the pages", func(t *testing.T) {
		projectDir := t.TempDir()
		code, _, stderr := runInProject(t, projectDir, "ride")
		want := "error: gondola.yaml not found in the current directory; it holds the site settings and must set at least config.site-name, config.version and config.canonical-url\n" +
			"build failed: 1 error\n"
		if code != exitBuildError || stderr != want {
			t.Errorf("exit code %d, standard error:\n%s", code, stderr)
		}
	})
}

// runInProject runs the command line args on the project in projectDir and
// returns the exit code and what was printed.
func runInProject(t *testing.T, projectDir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut strings.Builder
	printer := console.New(console.Stream{Writer: &out}, console.Stream{Writer: &errOut}, os.DirFS(projectDir))
	code = run(args, "v0.0.1", projectDir, printer)
	return code, out.String(), errOut.String()
}

// writeProject creates the files in a new directory called "site" and
// returns its path.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	projectDir := filepath.Join(t.TempDir(), "site")
	for name, content := range files {
		diskPath := filepath.Join(projectDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(diskPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir
}
