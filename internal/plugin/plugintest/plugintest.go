// Package plugintest helps tests run real plugins without depending on a
// shell: it builds the small program in testplugin once, installs copies of
// it in test projects, and reads back what each copy received. It is used
// only by tests.
package plugintest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// BehaviorSuffix is added to the path of an installed plugin to name the
// file that tells it what to do.
const BehaviorSuffix = ".json"

// HelperFailureCode is the exit code of testplugin when it cannot do what
// its Behavior asks, such as reading the file. Tests give their plugins
// other codes, so a failure of the helper itself is not mistaken for one.
const HelperFailureCode = 125

// maxRecordLineBytes is the longest record line ReadRecords accepts. A
// record holds a list of output files, which can exceed bufio's default
// limit of 64 KiB for one line.
const maxRecordLineBytes = 16 << 20

// programPackage is the import path of the plugin program built for tests.
const programPackage = "github.com/luciaduck/gondola/internal/plugin/plugintest/testplugin"

// Behavior is what an installed plugin does when it runs, in this order:
// record what it received, change the output directory (its first
// argument), print, and exit.
type Behavior struct {
	// Name is copied into the Record, to tell plugins apart.
	Name string
	// RecordPath is the file the plugin appends its Record to; "" records
	// nothing.
	RecordPath string
	// RecordVariables are the environment variables to record.
	RecordVariables []string
	// ListOutput records the files of the output directory.
	ListOutput bool
	// WriteFiles maps paths relative to the output directory, written with
	// "/", to the contents the plugin writes there. The contents are bytes
	// because JSON would replace invalid UTF-8 in a string.
	WriteFiles map[string][]byte
	// DeleteFiles are paths relative to the output directory, written with
	// "/", that the plugin deletes.
	DeleteFiles []string
	// RemoveOutput deletes the output directory itself.
	RemoveOutput bool
	// Stdout and Stderr are written to the plugin's standard output and
	// error, byte for byte.
	Stdout []byte
	Stderr []byte
	// KillItself ends the plugin with a kill signal instead of an exit
	// code: on Unix, its exit is reported as "signal: killed".
	KillItself bool
	ExitCode   int
}

// Record is what one run of an installed plugin received.
type Record struct {
	Name       string
	Args       []string
	WorkingDir string
	// Stdin is everything the plugin could read from its standard input.
	Stdin string
	// Variables holds the values of Behavior.RecordVariables that were set.
	Variables map[string]string
	// OutputFiles are the files of the output directory, relative to it,
	// written with "/" and sorted; nil unless Behavior.ListOutput is set.
	OutputFiles []string
}

// BuildProgram builds testplugin into a new temporary directory and returns
// the path of the program. It runs the go command, which is there whenever
// tests are. The caller deletes the directory, usually at the end of
// TestMain.
func BuildProgram() (programPath string, err error) {
	dir, err := os.MkdirTemp("", "gondola-testplugin-")
	if err != nil {
		return "", err
	}
	programPath = filepath.Join(dir, "testplugin")
	if runtime.GOOS == "windows" {
		programPath += ".exe"
	}
	output, err := exec.Command("go", "build", "-o", programPath, programPackage).CombinedOutput()
	if err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("building %s: %w\n%s", programPackage, err, output)
	}
	return programPath, nil
}

// ProgramPath returns pluginPath, written with "/", as a test plugin must be
// named on this system: Windows starts a program only through an extension
// it knows, so "tools/check" becomes "tools/check.exe" there.
func ProgramPath(pluginPath string) string {
	if runtime.GOOS == "windows" {
		return pluginPath + ".exe"
	}
	return pluginPath
}

// Install copies the program at programPath to pluginPath, relative to
// projectDir and written with "/", makes it executable, and writes behavior
// next to it.
func Install(t testing.TB, programPath, projectDir, pluginPath string, behavior Behavior) {
	t.Helper()
	program, err := os.ReadFile(programPath)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(projectDir, filepath.FromSlash(pluginPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, program, 0o755); err != nil {
		t.Fatal(err)
	}
	WriteBehavior(t, projectDir, pluginPath, behavior)
}

// WriteBehavior replaces what the plugin installed at pluginPath does, so
// that a fuzz target can install the program once and change only this.
func WriteBehavior(t testing.TB, projectDir, pluginPath string, behavior Behavior) {
	t.Helper()
	behaviorText, err := json.Marshal(behavior)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(projectDir, filepath.FromSlash(pluginPath))
	if err := os.WriteFile(target+BehaviorSuffix, behaviorText, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ReadRecords returns the records appended to recordPath, in the order the
// plugins ran. A missing file means that no plugin ran, and gives nil.
func ReadRecords(t testing.TB, recordPath string) []Record {
	t.Helper()
	file, err := os.Open(recordPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, maxRecordLineBytes)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("%s: %v", recordPath, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}
