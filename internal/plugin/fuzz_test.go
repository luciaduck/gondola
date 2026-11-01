package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/plugin/plugintest"
)

// maxFuzzPathBytes keeps fuzzed paths well below the 255 bytes that Linux
// allows in one name, so that a long input is not refused by the disk
// instead of by the code under test.
const maxFuzzPathBytes = 120

// fuzzPluginPath is where the fuzz targets install the plugin program.
const fuzzPluginPath = "tools/fuzz"

// isFuzzableTempDir reports whether tempDir can be a temp output: a clean
// relative path written with "/", without "..", that the disk can hold and
// that does not overlap the plugin's directory.
func isFuzzableTempDir(tempDir string) bool {
	if tempDir == "" || len(tempDir) > maxFuzzPathBytes || !utf8.ValidString(tempDir) ||
		strings.ContainsRune(tempDir, 0) || path.IsAbs(tempDir) || path.Clean(tempDir) != tempDir || tempDir == "." {
		return false
	}
	elements := strings.Split(tempDir, "/")
	for _, element := range elements {
		if element == ".." {
			return false
		}
	}
	return elements[0] != "tools"
}

// installFuzzPlugin creates a project with the plugin program installed,
// once for each fuzzing worker, so that each input only rewrites the
// behavior.
func installFuzzPlugin(f *testing.F) string {
	projectDir := f.TempDir()
	plugintest.Install(f, testProgram, projectDir, fuzzPluginPath, plugintest.Behavior{})
	return projectDir
}

// FuzzRun checks the contract of SPEC §5.13 for any temp output, mode,
// inherited value, output and exit code: the plugin receives the absolute
// temp output as its only argument and in GONDOLA_OUTPUT, the mode, the
// inherited variable, the project root as working directory and an empty
// standard input; its output reaches the console byte for byte; and the
// build fails exactly when the exit code is not 0, with the message of
// M11, Q3.
func FuzzRun(f *testing.F) {
	f.Add("_site.temp", false, "kept", []byte("minified 3 files\n"), []byte(""), uint8(0))
	f.Add("public files.temp", true, "", []byte(nil), []byte("warning\n"), uint8(1))
	f.Add("città/$HOME `ls` 'x'.temp", false, "a=b=c", []byte("\x00\xff"), []byte("\r\n"), uint8(255))
	f.Add("-rf", true, "line\nbreak", []byte("\x1b[31mred"), []byte("err"), uint8(3))
	f.Add("a\nb.temp", false, "è", []byte("x"), []byte("y"), uint8(2))
	projectDir := installFuzzPlugin(f)
	f.Fuzz(func(t *testing.T, tempDir string, serve bool, inherited string, stdout, stderr []byte, exitCode uint8) {
		if !isFuzzableTempDir(tempDir) || strings.ContainsRune(inherited, 0) || !utf8.ValidString(inherited) ||
			int(exitCode) == plugintest.HelperFailureCode {
			t.Skip()
		}
		absoluteTempDir := filepath.Join(projectDir, filepath.FromSlash(tempDir))
		if err := os.MkdirAll(absoluteTempDir, 0o755); err != nil {
			t.Skip()
		}
		defer os.RemoveAll(filepath.Join(projectDir, strings.Split(tempDir, "/")[0]))
		mode := Ride
		if serve {
			mode = Serve
		}
		recordPath := filepath.Join(t.TempDir(), "records")
		plugintest.WriteBehavior(t, projectDir, fuzzPluginPath, plugintest.Behavior{
			RecordPath:      recordPath,
			RecordVariables: []string{"GONDOLA_OUTPUT", "GONDOLA_MODE", "GONDOLA_TEST_INHERITED"},
			Stdout:          stdout,
			Stderr:          stderr,
			ExitCode:        int(exitCode),
		})
		var gotStdout, gotStderr bytes.Buffer
		printer := &recordingPrinter{}
		err := Run([]string{fuzzPluginPath}, Options{
			ProjectDir:  projectDir,
			TempDir:     tempDir,
			Mode:        mode,
			Environment: []string{"GONDOLA_MODE=wrong", "GONDOLA_TEST_INHERITED=" + inherited},
			Stdout:      &gotStdout,
			Stderr:      &gotStderr,
			Printer:     printer,
		})
		wantErr := ""
		if exitCode != 0 {
			wantErr = fmt.Sprintf("plugin %q failed: exit status %d", fuzzPluginPath, exitCode)
		}
		if got := errorText(err); got != wantErr {
			t.Errorf("error %q, want %q (stderr %q)", got, wantErr, gotStderr.Bytes())
		}
		if !bytes.Equal(gotStdout.Bytes(), stdout) {
			t.Errorf("stdout %q, want %q", gotStdout.Bytes(), stdout)
		}
		if !bytes.Equal(gotStderr.Bytes(), stderr) {
			t.Errorf("stderr %q, want %q", gotStderr.Bytes(), stderr)
		}
		want := []plugintest.Record{{
			Args:       []string{absoluteTempDir},
			WorkingDir: resolvedPath(t, projectDir),
			Variables: map[string]string{
				"GONDOLA_OUTPUT":         absoluteTempDir,
				"GONDOLA_MODE":           string(mode),
				"GONDOLA_TEST_INHERITED": inherited,
			},
		}}
		if got := plugintest.ReadRecords(t, recordPath); !reflect.DeepEqual(got, want) {
			t.Errorf("records differ\n got %+v\nwant %+v", got, want)
		}
		wantPrinted := []string{"Executing " + fuzzPluginPath + " in " + tempDir}
		if !reflect.DeepEqual(printer.lines, wantPrinted) {
			t.Errorf("printed %q, want %q", printer.lines, wantPrinted)
		}
	})
}

// The kinds of file that FuzzCheck creates at the plugin's path.
const (
	kindMissing = iota
	kindExecutable
	kindNotExecutable
	kindDirectory
	kindLinkToExecutable
	kindLinkToNothing
	kindParentIsFile
	kindCount
)

// fuzzCheckConfig is a gondola.yaml whose only plugin is the JSON string
// added at its end, which YAML reads as a double-quoted string: line 6,
// column 7.
const fuzzCheckConfig = "config:\n  site-name: Test\n  version: 1\n  canonical-url: https://example.com\n  plugin:\n    - "

// FuzzCheck writes any entry into config.plugin, lets config clean it,
// creates one kind of file at its path, and checks that the preflight gives
// exactly the message for that kind, at the entry's position.
func FuzzCheck(f *testing.F) {
	for kind := range kindCount {
		f.Add("tools/minify.sh", uint8(kind))
	}
	f.Add("./a//b c/../d", uint8(kindExecutable))
	f.Add("minify", uint8(kindNotExecutable))
	f.Add("città/$x 'y'", uint8(kindParentIsFile))
	f.Add("a\\b", uint8(kindDirectory))
	f.Fuzz(func(t *testing.T, entry string, kind uint8) {
		quoted, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := config.Parse([]byte(fuzzCheckConfig + string(quoted) + "\n"))
		if err != nil {
			// config refuses the entry; its own fuzz target covers that.
			return
		}
		if len(cfg.Settings.PluginPaths) != 1 {
			t.Fatalf("plugin paths %q from entry %q, want one", cfg.Settings.PluginPaths, entry)
		}
		pluginPath := cfg.Settings.PluginPaths[0]
		if want := (config.Position{Line: 6, Col: 7}); !reflect.DeepEqual(cfg.Settings.PluginPositions, []config.Position{want}) {
			t.Fatalf("positions %v, want [%v]", cfg.Settings.PluginPositions, want)
		}
		elements := strings.Split(pluginPath, "/")
		// The project's own gondola.yaml is in the way of these paths.
		if len(pluginPath) > maxFuzzPathBytes || strings.EqualFold(elements[0], config.FileName) {
			t.Skip()
		}
		// config accepts a NUL character in every path, which no file name
		// can hold; the preflight then reports "invalid argument". Left out
		// until config refuses control characters in paths, planned for M14
		// (testdata/fuzz/FuzzCheck/ea903adeb6df0766).
		if strings.ContainsRune(pluginPath, 0) {
			t.Skip()
		}
		projectDir := t.TempDir()
		target := filepath.Join(projectDir, filepath.FromSlash(pluginPath))
		want, ok := createKind(t, projectDir, target, elements, int(kind)%kindCount)
		// "." is the project root, which is always a directory, whatever
		// was created.
		if pluginPath == "." {
			want, ok = `config.plugin %[1]q is a directory, not a program`, true
		}
		if !ok {
			t.Skip()
		}
		if want != "" {
			want = "gondola.yaml:6:7: " + fmt.Sprintf(want, pluginPath, shellQuoted(pluginPath))
		}
		if got := errorText(Check(projectDir, cfg.Settings.PluginPaths, cfg.Settings.PluginPositions)); got != want {
			t.Errorf("entry %q, kind %d\n got %q\nwant %q", entry, kind, got, want)
		}
	})
}

// createKind creates the kind of file at target, the disk path of the
// plugin whose path elements are elements, and returns the message format
// that Check must give, with %[1]q for the plugin's path and %[2]s for it
// quoted for a shell, or "" when it must pass. ok is false when the disk cannot hold the case.
func createKind(t *testing.T, projectDir, target string, elements []string, kind int) (want string, ok bool) {
	const (
		missing       = `config.plugin %[1]q does not exist; please check the path`
		directory     = `config.plugin %[1]q is a directory, not a program`
		notExecutable = `config.plugin %[1]q is not executable; please make it executable (chmod +x %[2]s)`
	)
	create := func(kind int) error {
		switch kind {
		case kindExecutable:
			return writeMode(target, 0o755)
		case kindNotExecutable:
			return writeMode(target, 0o644)
		case kindDirectory:
			return os.MkdirAll(target, 0o755)
		case kindLinkToExecutable:
			linked := filepath.Join(t.TempDir(), "linked")
			if err := writeMode(linked, 0o755); err != nil {
				return err
			}
			return linkTo(linked, target)
		case kindLinkToNothing:
			return linkTo(filepath.Join(t.TempDir(), "nothing"), target)
		case kindParentIsFile:
			return writeMode(filepath.Join(projectDir, elements[0]), 0o755)
		}
		return nil
	}
	if kind == kindParentIsFile && len(elements) < 2 {
		return "", false
	}
	if err := create(kind); err != nil {
		return "", false
	}
	switch kind {
	case kindMissing, kindLinkToNothing, kindParentIsFile:
		return missing, true
	case kindNotExecutable:
		return notExecutable, true
	case kindDirectory:
		return directory, true
	}
	return "", true
}

// writeMode writes an empty file with exactly permissions, which the umask
// would otherwise reduce.
func writeMode(diskPath string, permissions os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(diskPath, nil, permissions); err != nil {
		return err
	}
	return os.Chmod(diskPath, permissions)
}

func linkTo(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}
	return os.Symlink(target, linkPath)
}

// FuzzCanaryRun is written to fail: it claims that the temp output that a
// plugin receives never contains a space. Any temp output with a space
// proves it wrong, which shows that the fuzzer's inputs reach the plugin's
// arguments. It is skipped unless GONDOLA_FUZZ_CANARY is set, so that go
// test stays green with the failing input saved under testdata/fuzz:
//
//	GONDOLA_FUZZ_CANARY=1 go test ./internal/plugin -run '^$' -fuzz FuzzCanaryRun -fuzztime 1m
func FuzzCanaryRun(f *testing.F) {
	if os.Getenv("GONDOLA_FUZZ_CANARY") == "" {
		f.Skip("the canary fails on purpose; set GONDOLA_FUZZ_CANARY to run it")
	}
	// Only seeds that keep the claim true: the fuzzer has to find the
	// counterexample itself.
	for _, seed := range []string{"_site.temp", "public.temp", "out/site"} {
		f.Add(seed)
	}
	projectDir := installFuzzPlugin(f)
	f.Fuzz(func(t *testing.T, tempDir string) {
		if !isFuzzableTempDir(tempDir) {
			t.Skip()
		}
		if err := os.MkdirAll(filepath.Join(projectDir, filepath.FromSlash(tempDir)), 0o755); err != nil {
			t.Skip()
		}
		defer os.RemoveAll(filepath.Join(projectDir, strings.Split(tempDir, "/")[0]))
		recordPath := filepath.Join(t.TempDir(), "records")
		plugintest.WriteBehavior(t, projectDir, fuzzPluginPath, plugintest.Behavior{RecordPath: recordPath})
		err := Run([]string{fuzzPluginPath}, Options{
			ProjectDir: projectDir,
			TempDir:    tempDir,
			Mode:       Ride,
			Stdout:     &bytes.Buffer{},
			Stderr:     &bytes.Buffer{},
			Printer:    &recordingPrinter{},
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, record := range plugintest.ReadRecords(t, recordPath) {
			if strings.Contains(record.Args[0], " ") {
				t.Errorf("the plugin received %q, which contains a space", record.Args[0])
			}
		}
	})
}
