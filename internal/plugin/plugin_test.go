package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
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

// recordingPrinter keeps the Executing lines, without the prefix.
type recordingPrinter struct {
	lines []string
}

func (p *recordingPrinter) Executing(pluginPath, tempDir string) {
	p.lines = append(p.lines, fmt.Sprintf("Executing %s in %s", pluginPath, tempDir))
}

// testTempDir is the temp output of the test projects.
const testTempDir = "_site.temp"

// runResult is what Run returned and what the plugins printed.
type runResult struct {
	err     error
	stdout  string
	stderr  string
	printed []string
}

// runPlugins runs pluginPaths in projectDir, creating the temp output
// first, with options filled in where the test left them empty.
func runPlugins(t testing.TB, projectDir string, pluginPaths []string, options Options) runResult {
	t.Helper()
	if options.TempDir == "" {
		options.TempDir = testTempDir
	}
	if options.Mode == "" {
		options.Mode = Ride
	}
	if err := os.MkdirAll(filepath.Join(projectDir, filepath.FromSlash(options.TempDir)), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	printer := &recordingPrinter{}
	options.ProjectDir = projectDir
	options.Stdout = &stdout
	options.Stderr = &stderr
	options.Printer = printer
	err := Run(pluginPaths, options)
	return runResult{err: err, stdout: stdout.String(), stderr: stderr.String(), printed: printer.lines}
}

// errorText returns the message of err, or "" for nil.
func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// writeScript writes an executable text file, for the cases that need a
// script rather than a program.
func writeScript(t testing.TB, projectDir, scriptPath, content string) {
	t.Helper()
	target := filepath.Join(projectDir, filepath.FromSlash(scriptPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

// skipOnWindows skips a test of Unix behavior: execute bits, scripts with
// "#!" lines, signals.
func skipOnWindows(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Unix behavior")
	}
}

func TestCheck(t *testing.T) {
	position := config.Position{Line: 5, Col: 5}
	tests := []struct {
		name string
		// prepare creates the files of the project.
		prepare     func(t *testing.T, projectDir string)
		pluginPaths []string
		unixOnly    bool
		want        string
	}{
		{
			name:        "no plugins",
			prepare:     func(t *testing.T, projectDir string) {},
			pluginPaths: []string{},
		},
		{
			name: "an executable file",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "tools/minify.sh", "#!/bin/sh\n")
			},
			pluginPaths: []string{"tools/minify.sh"},
		},
		{
			name: "a file at the project root",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "minify", "#!/bin/sh\n")
			},
			pluginPaths: []string{"minify"},
		},
		{
			name:        "missing",
			prepare:     func(t *testing.T, projectDir string) {},
			pluginPaths: []string{"tools/minify.sh"},
			want:        `gondola.yaml:5:5: config.plugin "tools/minify.sh" does not exist; please check the path`,
		},
		{
			name: "a parent that is a file",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "tools", "#!/bin/sh\n")
			},
			pluginPaths: []string{"tools/minify.sh"},
			want:        `gondola.yaml:5:5: config.plugin "tools/minify.sh" does not exist; please check the path`,
		},
		{
			name: "a directory",
			prepare: func(t *testing.T, projectDir string) {
				mkdir(t, filepath.Join(projectDir, "tools"))
			},
			pluginPaths: []string{"tools"},
			want:        `gondola.yaml:5:5: config.plugin "tools" is a directory, not a program`,
		},
		{
			name: "not executable",
			prepare: func(t *testing.T, projectDir string) {
				writeFile(t, filepath.Join(projectDir, "tools", "minify.sh"), "#!/bin/sh\n", 0o644)
			},
			pluginPaths: []string{"tools/minify.sh"},
			unixOnly:    true,
			want:        `gondola.yaml:5:5: config.plugin "tools/minify.sh" is not executable; please make it executable (chmod +x tools/minify.sh)`,
		},
		{
			name: "not executable, with a space",
			prepare: func(t *testing.T, projectDir string) {
				writeFile(t, filepath.Join(projectDir, "my tools", "run"), "#!/bin/sh\n", 0o644)
			},
			pluginPaths: []string{"my tools/run"},
			unixOnly:    true,
			want:        `gondola.yaml:5:5: config.plugin "my tools/run" is not executable; please make it executable (chmod +x 'my tools/run')`,
		},
		{
			name: "executable by others only",
			prepare: func(t *testing.T, projectDir string) {
				writeFile(t, filepath.Join(projectDir, "tools", "minify.sh"), "#!/bin/sh\n", 0o645)
			},
			pluginPaths: []string{"tools/minify.sh"},
			unixOnly:    true,
		},
		{
			name: "a link to an executable file",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "shared/minify.sh", "#!/bin/sh\n")
				symlink(t, filepath.Join("..", "shared", "minify.sh"), filepath.Join(projectDir, "tools", "minify.sh"))
			},
			pluginPaths: []string{"tools/minify.sh"},
			unixOnly:    true,
		},
		{
			name: "a link to nothing",
			prepare: func(t *testing.T, projectDir string) {
				symlink(t, "nowhere.sh", filepath.Join(projectDir, "tools", "minify.sh"))
			},
			pluginPaths: []string{"tools/minify.sh"},
			unixOnly:    true,
			want:        `gondola.yaml:5:5: config.plugin "tools/minify.sh" does not exist; please check the path`,
		},
		{
			name: "a link to a directory",
			prepare: func(t *testing.T, projectDir string) {
				mkdir(t, filepath.Join(projectDir, "shared"))
				symlink(t, filepath.Join("..", "shared"), filepath.Join(projectDir, "tools", "minify"))
			},
			pluginPaths: []string{"tools/minify"},
			unixOnly:    true,
			want:        `gondola.yaml:5:5: config.plugin "tools/minify" is a directory, not a program`,
		},
		{
			name: "the first failing plugin is named",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "tools/first", "#!/bin/sh\n")
				mkdir(t, filepath.Join(projectDir, "tools", "third"))
			},
			pluginPaths: []string{"tools/first", "tools/second", "tools/third"},
			want:        `gondola.yaml:5:5: config.plugin "tools/second" does not exist; please check the path`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.unixOnly {
				skipOnWindows(t)
			}
			projectDir := t.TempDir()
			test.prepare(t, projectDir)
			positions := make([]config.Position, len(test.pluginPaths))
			for i := range positions {
				positions[i] = position
			}
			if got := errorText(Check(projectDir, test.pluginPaths, positions)); got != test.want {
				t.Errorf("got  %q\nwant %q", got, test.want)
			}
		})
	}
}

// TestCheckPositions checks that the errors point at the entries as
// gondola.yaml writes them, through config.
func TestShellQuoted(t *testing.T) {
	tests := []struct{ text, want string }{
		{"tools/minify.sh", "tools/minify.sh"},
		{"a-b_c+d,e:f@g%h=i", "a-b_c+d,e:f@g%h=i"},
		{"my tools/run", "'my tools/run'"},
		{"città/run", "'città/run'"},
		{"$HOME/run", "'$HOME/run'"},
		{"it's/run", `'it'\''s/run'`},
		{"a*b", "'a*b'"},
		{"~/run", "'~/run'"},
	}
	for _, test := range tests {
		if got := shellQuoted(test.text); got != test.want {
			t.Errorf("shellQuoted(%q) = %s, want %s", test.text, got, test.want)
		}
	}
}

func TestCheckPositions(t *testing.T) {
	src := "config:\n  site-name: Test\n  version: 1\n  canonical-url: https://example.com\n" +
		"  plugin:\n    - tools/first\n    - ./tools//second\n"
	cfg, err := config.Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	writeScript(t, projectDir, "tools/first", "#!/bin/sh\n")
	err = Check(projectDir, cfg.Settings.PluginPaths, cfg.Settings.PluginPositions)
	want := `gondola.yaml:7:7: config.plugin "tools/second" does not exist; please check the path`
	if got := errorText(err); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCheckUnreadableDirectory(t *testing.T) {
	skipOnWindows(t)
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	projectDir := t.TempDir()
	writeScript(t, projectDir, "tools/minify.sh", "#!/bin/sh\n")
	toolsDir := filepath.Join(projectDir, "tools")
	if err := os.Chmod(toolsDir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(toolsDir, 0o755) })
	err := Check(projectDir, []string{"tools/minify.sh"}, []config.Position{{Line: 5, Col: 5}})
	want := `gondola.yaml:5:5: config.plugin "tools/minify.sh" cannot be checked: permission denied`
	if got := errorText(err); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// TestRunContract checks what a plugin receives (SPEC §5.13): the absolute
// temp output as its only argument and in GONDOLA_OUTPUT, GONDOLA_MODE, the
// inherited environment, the project root as working directory, and an
// empty standard input.
func TestRunContract(t *testing.T) {
	tests := []struct {
		name    string
		mode    Mode
		tempDir string
	}{
		{"ride", Ride, "_site.temp"},
		{"serve", Serve, "_site.temp"},
		{"a nested temp output", Ride, "build/public.temp"},
		{"spaces and other letters", Ride, "my site è $HOME.temp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			recordPath := filepath.Join(t.TempDir(), "records")
			plugintest.Install(t, testProgram, projectDir, "tools/check", plugintest.Behavior{
				RecordPath:      recordPath,
				RecordVariables: []string{"GONDOLA_OUTPUT", "GONDOLA_MODE", "GONDOLA_TEST_INHERITED"},
			})
			result := runPlugins(t, projectDir, []string{"tools/check"}, Options{
				TempDir: test.tempDir,
				Mode:    test.mode,
				// Values of Gondola's variables that were already set are
				// replaced.
				Environment: []string{"GONDOLA_OUTPUT=wrong", "GONDOLA_MODE=wrong", "GONDOLA_TEST_INHERITED=kept"},
			})
			if result.err != nil {
				t.Fatalf("%v\n%s", result.err, result.stderr)
			}
			absoluteTempDir := filepath.Join(projectDir, filepath.FromSlash(test.tempDir))
			want := []plugintest.Record{{
				Args:       []string{absoluteTempDir},
				WorkingDir: resolvedPath(t, projectDir),
				Stdin:      "",
				Variables: map[string]string{
					"GONDOLA_OUTPUT":         absoluteTempDir,
					"GONDOLA_MODE":           string(test.mode),
					"GONDOLA_TEST_INHERITED": "kept",
				},
			}}
			if got := plugintest.ReadRecords(t, recordPath); !reflect.DeepEqual(got, want) {
				t.Errorf("records differ\n got %+v\nwant %+v", got, want)
			}
			wantPrinted := []string{"Executing tools/check in " + test.tempDir}
			if !reflect.DeepEqual(result.printed, wantPrinted) {
				t.Errorf("printed %q, want %q", result.printed, wantPrinted)
			}
		})
	}
}

// TestRunNilEnvironment checks that a nil Environment gives the plugin only
// Gondola's two variables.
func TestRunNilEnvironment(t *testing.T) {
	projectDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "records")
	plugintest.Install(t, testProgram, projectDir, "tools/check", plugintest.Behavior{
		RecordPath:      recordPath,
		RecordVariables: []string{"GONDOLA_MODE", "PATH", "HOME"},
	})
	result := runPlugins(t, projectDir, []string{"tools/check"}, Options{})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stderr)
	}
	records := plugintest.ReadRecords(t, recordPath)
	want := map[string]string{"GONDOLA_MODE": "ride"}
	if len(records) != 1 || !reflect.DeepEqual(records[0].Variables, want) {
		t.Errorf("records %+v, want one with variables %v", records, want)
	}
}

// TestRunIgnoresPath checks that a plugin named without a directory is the
// file of the project root, even when PATH holds a program of that name.
func TestRunIgnoresPath(t *testing.T) {
	projectDir := t.TempDir()
	pathDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "records")
	plugintest.Install(t, testProgram, projectDir, "minify", plugintest.Behavior{Name: "project", RecordPath: recordPath})
	plugintest.Install(t, testProgram, pathDir, "minify", plugintest.Behavior{Name: "path", RecordPath: recordPath})
	result := runPlugins(t, projectDir, []string{"minify"}, Options{Environment: []string{"PATH=" + pathDir}})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stderr)
	}
	records := plugintest.ReadRecords(t, recordPath)
	if len(records) != 1 || records[0].Name != "project" {
		t.Errorf("records %+v, want one from the project's plugin", records)
	}
}

// TestRunOrder checks that plugins run one at a time, in order, and that
// each sees the changes of the ones before it.
func TestRunOrder(t *testing.T) {
	projectDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "records")
	writeFile(t, filepath.Join(projectDir, testTempDir, "index.html"), "home", 0o644)
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{
		Name: "first", RecordPath: recordPath, ListOutput: true,
		WriteFiles: map[string][]byte{"feed.json": []byte("{}")},
	})
	plugintest.Install(t, testProgram, projectDir, "tools/second", plugintest.Behavior{
		Name: "second", RecordPath: recordPath, ListOutput: true,
		DeleteFiles: []string{"index.html"},
	})
	plugintest.Install(t, testProgram, projectDir, "third", plugintest.Behavior{
		Name: "third", RecordPath: recordPath, ListOutput: true,
	})
	result := runPlugins(t, projectDir, []string{"tools/first", "tools/second", "third"}, Options{})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stderr)
	}
	var got [][]string
	for _, record := range plugintest.ReadRecords(t, recordPath) {
		got = append(got, append([]string{record.Name}, record.OutputFiles...))
	}
	want := [][]string{
		{"first", "index.html"},
		{"second", "feed.json", "index.html"},
		{"third", "feed.json"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	wantPrinted := []string{
		"Executing tools/first in _site.temp",
		"Executing tools/second in _site.temp",
		"Executing third in _site.temp",
	}
	if !reflect.DeepEqual(result.printed, wantPrinted) {
		t.Errorf("printed %q, want %q", result.printed, wantPrinted)
	}
}

// TestRunOutput checks that the plugins' standard output and error reach
// the console byte for byte, in the order the plugins ran.
func TestRunOutput(t *testing.T) {
	projectDir := t.TempDir()
	plugintest.Install(t, testProgram, projectDir, "tools/first", plugintest.Behavior{
		Stdout: []byte("first out\n"), Stderr: []byte("first err\x1b[31m"),
	})
	plugintest.Install(t, testProgram, projectDir, "tools/second", plugintest.Behavior{
		Stdout: []byte("second\r\nout\x00\xff"), Stderr: []byte("second err\n"),
	})
	result := runPlugins(t, projectDir, []string{"tools/first", "tools/second"}, Options{})
	if result.err != nil {
		t.Fatalf("%v\n%s", result.err, result.stderr)
	}
	if want := "first out\nsecond\r\nout\x00\xff"; result.stdout != want {
		t.Errorf("stdout %q, want %q", result.stdout, want)
	}
	if want := "first err\x1b[31msecond err\n"; result.stderr != want {
		t.Errorf("stderr %q, want %q", result.stderr, want)
	}
}

func TestRunFailure(t *testing.T) {
	tests := []struct {
		name string
		// prepare installs tools/failing.
		prepare  func(t *testing.T, projectDir string)
		unixOnly bool
		want     string
	}{
		{
			name: "exit status 1",
			prepare: func(t *testing.T, projectDir string) {
				plugintest.Install(t, testProgram, projectDir, "tools/failing", plugintest.Behavior{ExitCode: 1})
			},
			want: `plugin "tools/failing" failed: exit status 1`,
		},
		{
			name: "exit status 3",
			prepare: func(t *testing.T, projectDir string) {
				plugintest.Install(t, testProgram, projectDir, "tools/failing", plugintest.Behavior{ExitCode: 3})
			},
			want: `plugin "tools/failing" failed: exit status 3`,
		},
		{
			name: "exit status 255",
			prepare: func(t *testing.T, projectDir string) {
				plugintest.Install(t, testProgram, projectDir, "tools/failing", plugintest.Behavior{ExitCode: 255})
			},
			want: `plugin "tools/failing" failed: exit status 255`,
		},
		{
			name: "killed",
			prepare: func(t *testing.T, projectDir string) {
				plugintest.Install(t, testProgram, projectDir, "tools/failing", plugintest.Behavior{KillItself: true})
			},
			unixOnly: true,
			want:     `plugin "tools/failing" failed: signal: killed`,
		},
		{
			name: "a script without a #! line",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "tools/failing", "echo hello\n")
			},
			unixOnly: true,
			want:     `plugin "tools/failing" could not be started: exec format error (a script needs a "#!" line naming its interpreter)`,
		},
		{
			name: "a missing interpreter",
			prepare: func(t *testing.T, projectDir string) {
				writeScript(t, projectDir, "tools/failing", "#!/no/such/interpreter\n")
			},
			unixOnly: true,
			want:     `plugin "tools/failing" could not be started: no such file or directory`,
		},
		{
			name:     "deleted after the preflight",
			prepare:  func(t *testing.T, projectDir string) {},
			unixOnly: true,
			want:     `plugin "tools/failing" could not be started: no such file or directory`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.unixOnly {
				skipOnWindows(t)
			}
			projectDir := t.TempDir()
			recordPath := filepath.Join(t.TempDir(), "records")
			plugintest.Install(t, testProgram, projectDir, "tools/before", plugintest.Behavior{Name: "before", RecordPath: recordPath})
			plugintest.Install(t, testProgram, projectDir, "tools/after", plugintest.Behavior{Name: "after", RecordPath: recordPath})
			test.prepare(t, projectDir)
			result := runPlugins(t, projectDir, []string{"tools/before", "tools/failing", "tools/after"}, Options{})
			if got := errorText(result.err); got != test.want {
				t.Errorf("got  %q\nwant %q", got, test.want)
			}
			// The plugins after the failing one do not run.
			var names []string
			for _, record := range plugintest.ReadRecords(t, recordPath) {
				names = append(names, record.Name)
			}
			if want := []string{"before"}; !reflect.DeepEqual(names, want) {
				t.Errorf("ran %q, want %q", names, want)
			}
			wantPrinted := []string{"Executing tools/before in _site.temp", "Executing tools/failing in _site.temp"}
			if !reflect.DeepEqual(result.printed, wantPrinted) {
				t.Errorf("printed %q, want %q", result.printed, wantPrinted)
			}
		})
	}
}

// TestRunFailureKeepsExitError checks that the exit of a failed plugin can
// still be read from the error.
func TestRunFailureKeepsExitError(t *testing.T) {
	projectDir := t.TempDir()
	plugintest.Install(t, testProgram, projectDir, "tools/failing", plugintest.Behavior{ExitCode: 7})
	result := runPlugins(t, projectDir, []string{"tools/failing"}, Options{})
	var exitErr *exec.ExitError
	if !errors.As(result.err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Errorf("got %v, want an *exec.ExitError with code 7", result.err)
	}
}

func TestRunNoPlugins(t *testing.T) {
	result := runPlugins(t, t.TempDir(), nil, Options{})
	if result.err != nil || len(result.printed) != 0 {
		t.Errorf("got error %v and lines %q, want neither", result.err, result.printed)
	}
}

// resolvedPath returns diskPath with its symbolic links resolved, as the
// working directory of a process reports it.
func resolvedPath(t testing.TB, diskPath string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(diskPath)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func mkdir(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t testing.TB, diskPath, content string, permissions os.FileMode) {
	t.Helper()
	mkdir(t, filepath.Dir(diskPath))
	if err := os.WriteFile(diskPath, []byte(content), permissions); err != nil {
		t.Fatal(err)
	}
	// WriteFile applies the umask; the tests of the execute bits need the
	// exact permissions.
	if err := os.Chmod(diskPath, permissions); err != nil {
		t.Fatal(err)
	}
}

func symlink(t testing.TB, target, linkPath string) {
	t.Helper()
	mkdir(t, filepath.Dir(linkPath))
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}
}
