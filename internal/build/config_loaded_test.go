package build

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/console"
	"github.com/luciaduck/gondola/internal/plugin"
	"github.com/luciaduck/gondola/internal/sass"
)

func TestBuildConfigLoaded(t *testing.T) {
	const portConfig = minimalConfig + "  port: 8080\noutput:\n  directory: public\n"
	errRefused := errors.New("config.port changed")
	tests := []struct {
		name string
		// refuse is what the callback returns.
		refuse      error
		wantFailure bool
		// wantOldOutput is whether public/old.html survives the build.
		wantOldOutput bool
	}{
		{name: "nil lets the build go on", refuse: nil, wantFailure: false, wantOldOutput: false},
		{name: "an error stops the build before anything is deleted", refuse: errRefused, wantFailure: true, wantOldOutput: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, map[string]string{
				"gondola.yaml":    portConfig,
				"index.html":      "<p>hello</p>\n",
				"public/old.html": "old\n",
			})
			var stdout, stderr strings.Builder
			var seen []config.Config
			failure := Run(Options{
				ProjectDir:     projectDir,
				GondolaVersion: testVersion,
				SassStyle:      sass.Expanded,
				Mode:           plugin.Serve,
				Printer:        console.New(console.Stream{Writer: &stdout}, console.Stream{Writer: &stderr}, os.DirFS(projectDir)),
				PluginStdout:   &stdout,
				PluginStderr:   &stderr,
				Now:            fakeClock(testBuildDuration),
				ConfigLoaded: func(cfg config.Config) error {
					seen = append(seen, cfg)
					return test.refuse
				},
			})
			if len(seen) != 1 || seen[0].Settings.Port != 8080 || seen[0].Output.Dir != "public" {
				t.Fatalf("ConfigLoaded saw %d calls (%+v), want one with port 8080 and output public", len(seen), seen)
			}
			if (failure != nil) != test.wantFailure {
				t.Fatalf("failure = %v, want failure %v", failure, test.wantFailure)
			}
			if test.wantFailure && (len(failure.Errors) != 1 || !errors.Is(failure.Errors[0], errRefused)) {
				t.Errorf("errors = %v, want only the callback's error", failure.Errors)
			}
			_, err := os.Stat(filepath.Join(projectDir, "public", "old.html"))
			if gotOldOutput := err == nil; gotOldOutput != test.wantOldOutput {
				t.Errorf("public/old.html exists = %v, want %v", gotOldOutput, test.wantOldOutput)
			}
		})
	}
}

func TestBuildConfigLoadedNotCalledWithoutConfig(t *testing.T) {
	projectDir := t.TempDir()
	var stdout, stderr strings.Builder
	called := false
	failure := Run(Options{
		ProjectDir:   projectDir,
		Printer:      console.New(console.Stream{Writer: &stdout}, console.Stream{Writer: &stderr}, os.DirFS(projectDir)),
		PluginStdout: &stdout,
		PluginStderr: &stderr,
		Now:          fakeClock(testBuildDuration),
		ConfigLoaded: func(config.Config) error {
			called = true
			return nil
		},
	})
	if failure == nil || called {
		t.Errorf("failure = %v, ConfigLoaded called = %v; want a failure and no call", failure, called)
	}
}
