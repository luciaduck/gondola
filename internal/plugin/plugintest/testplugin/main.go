// Command testplugin is the plugin that the tests of Gondola install in their
// projects. It reads what to do from the file next to it, named like it with
// ".json" added (plugintest.Behavior), and appends what it received to a
// record file (plugintest.Record). It is built by plugintest.BuildProgram
// and is not part of Gondola.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/luciaduck/gondola/internal/plugin/plugintest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "testplugin:", err)
		os.Exit(plugintest.HelperFailureCode)
	}
}

func run() error {
	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	behaviorText, err := os.ReadFile(executablePath + plugintest.BehaviorSuffix)
	if err != nil {
		return err
	}
	var behavior plugintest.Behavior
	if err := json.Unmarshal(behaviorText, &behavior); err != nil {
		return err
	}
	if err := record(behavior); err != nil {
		return err
	}
	if err := changeOutput(behavior); err != nil {
		return err
	}
	os.Stdout.Write(behavior.Stdout)
	os.Stderr.Write(behavior.Stderr)
	if behavior.KillItself {
		process, err := os.FindProcess(os.Getpid())
		if err != nil {
			return err
		}
		return process.Kill()
	}
	os.Exit(behavior.ExitCode)
	return nil
}

// record appends what the plugin received to behavior.RecordPath, one JSON
// object per line.
func record(behavior plugintest.Behavior) error {
	if behavior.RecordPath == "" {
		return nil
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	received := plugintest.Record{
		Name:       behavior.Name,
		Args:       os.Args[1:],
		WorkingDir: workingDir,
		Stdin:      string(stdin),
		Variables:  map[string]string{},
	}
	for _, name := range behavior.RecordVariables {
		if value, ok := os.LookupEnv(name); ok {
			received.Variables[name] = value
		}
	}
	if behavior.ListOutput && len(os.Args) > 1 {
		if received.OutputFiles, err = listFiles(os.Args[1]); err != nil {
			return err
		}
	}
	line, err := json.Marshal(received)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(behavior.RecordPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// changeOutput writes, deletes and removes what behavior asks for in the
// directory of the first argument.
func changeOutput(behavior plugintest.Behavior) error {
	if len(os.Args) < 2 {
		return nil
	}
	outputDir := os.Args[1]
	for _, name := range sortedKeys(behavior.WriteFiles) {
		target := filepath.Join(outputDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, behavior.WriteFiles[name], 0o644); err != nil {
			return err
		}
	}
	for _, name := range behavior.DeleteFiles {
		if err := os.Remove(filepath.Join(outputDir, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	if behavior.RemoveOutput {
		return os.RemoveAll(outputDir)
	}
	return nil
}

// listFiles returns the files under dir, relative to it, written with "/"
// and sorted.
func listFiles(dir string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(dir, func(diskPath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(dir, diskPath)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func sortedKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
