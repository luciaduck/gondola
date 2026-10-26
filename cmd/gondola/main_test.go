package main

import (
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
			name:       "ride is not available yet",
			args:       []string{"ride"},
			wantCode:   exitBuildError,
			wantStderr: "error: \"ride\" is not available yet\n",
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
			code := run(test.args, "v0.0.1", printer)
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
