// Package plugin checks and runs the user's plugins: the programs listed in
// config.plugin, which run one at a time on the finished build before it is
// published (SPEC §5.13).
package plugin

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/luciaduck/gondola/internal/config"
)

// Mode is the command that runs the build, given to plugins in
// GONDOLA_MODE.
type Mode string

// The values of GONDOLA_MODE.
const (
	Ride  Mode = "ride"
	Serve Mode = "serve"
)

// The environment variables that Gondola gives to every plugin. New context
// for plugins is added here, never as arguments (SPEC §5.13).
const (
	outputVariable = "GONDOLA_OUTPUT"
	modeVariable   = "GONDOLA_MODE"
)

// executeBits are the permission bits of the owner, the group and the
// others that allow running a file. One of them is enough for the preflight.
const executeBits = 0o111

// shellSafeCharacters are those that a POSIX shell reads as plain text in a
// word, so a path made only of them needs no quotes in the chmod hint.
const shellSafeCharacters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./+,:@%="

// Printer is the part of the console that plugins write to.
// *console.Printer implements it.
type Printer interface {
	Executing(pluginPath, tempDir string)
}

// Check is the preflight of stage 1: each plugin must exist, be a file, and
// be executable. pluginPaths are relative to projectDir and written with
// "/", as config gives them; positions are where they are written in
// gondola.yaml, at the same indexes. It returns the error of the first
// plugin that fails.
func Check(projectDir string, pluginPaths []string, positions []config.Position) error {
	for i, pluginPath := range pluginPaths {
		position := config.Position{}
		if i < len(positions) {
			position = positions[i]
		}
		if err := checkPlugin(projectDir, pluginPath, position); err != nil {
			return err
		}
	}
	return nil
}

func checkPlugin(projectDir, pluginPath string, position config.Position) error {
	// Stat follows a symbolic link, so a link to a program is accepted.
	info, err := os.Stat(diskPath(projectDir, pluginPath))
	// ENOTDIR comes when a parent on the path is a file, as in
	// "minify.sh/run": the plugin does not exist either.
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		return position.Errorf("config.plugin %q does not exist; please check the path", pluginPath)
	}
	if err != nil {
		return position.Errorf("config.plugin %q cannot be checked: %w", pluginPath, withoutPath(err))
	}
	if info.IsDir() {
		return position.Errorf("config.plugin %q is a directory, not a program", pluginPath)
	}
	// Windows has no execute bit: whether the entry can run is known only
	// when it is started (M14).
	if runtime.GOOS != "windows" && info.Mode().Perm()&executeBits == 0 {
		return position.Errorf("config.plugin %q is not executable; please make it executable (chmod +x %s)", pluginPath, shellQuoted(pluginPath))
	}
	return nil
}

// shellQuoted returns text as a POSIX shell word, so that the chmod hint can
// be pasted as it is (M11, Q12): unchanged when it holds only safe
// characters, otherwise in single quotes, where each single quote of text
// closes the quotes, is escaped with a backslash, and reopens them.
func shellQuoted(text string) string {
	if strings.Trim(text, shellSafeCharacters) == "" {
		return text
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// Options are what running the plugins needs to know.
type Options struct {
	ProjectDir string
	// TempDir is the temp output, relative to ProjectDir and written with
	// "/". Plugins receive it as an absolute path.
	TempDir string
	Mode    Mode
	// Environment is what the plugins inherit, in the form of os.Environ.
	// GONDOLA_OUTPUT and GONDOLA_MODE replace any values it holds. A nil
	// Environment gives the plugins only those two variables.
	Environment []string
	// Stdout and Stderr receive the plugins' output as it is. When they are
	// *os.File, the plugins write to them directly and can tell whether
	// they are a terminal.
	Stdout  io.Writer
	Stderr  io.Writer
	Printer Printer
}

// Run runs the plugins one at a time, in order, and stops at the first one
// that fails, naming it. pluginPaths are relative to options.ProjectDir and
// written with "/".
func Run(pluginPaths []string, options Options) error {
	if len(pluginPaths) == 0 {
		return nil
	}
	absoluteProjectDir, err := filepath.Abs(options.ProjectDir)
	if err != nil {
		return fmt.Errorf("finding the project directory: %w", err)
	}
	absoluteTempDir := diskPath(absoluteProjectDir, options.TempDir)
	environment := pluginEnvironment(options.Environment, absoluteTempDir, options.Mode)
	for _, pluginPath := range pluginPaths {
		options.Printer.Executing(pluginPath, options.TempDir)
		// The program is always started by its absolute path: a bare name
		// such as "minify" would otherwise be looked up in PATH, while
		// config.plugin names a file of the project.
		command := exec.Command(diskPath(absoluteProjectDir, pluginPath), absoluteTempDir)
		command.Dir = absoluteProjectDir
		command.Env = environment
		// A nil Stdin reads from the null device, so a plugin never takes
		// the Enter key presses that gondola serve waits for (M11, Q4).
		command.Stdin = nil
		command.Stdout = options.Stdout
		command.Stderr = options.Stderr
		if err := runPlugin(command, pluginPath); err != nil {
			return err
		}
	}
	return nil
}

// runPlugin starts command and waits for it to exit.
func runPlugin(command *exec.Cmd, pluginPath string) error {
	if err := command.Start(); err != nil {
		reason := withoutPath(err)
		hint := ""
		if isNotAProgram(reason) {
			hint = notAProgramHint
		}
		return fmt.Errorf("plugin %q could not be started: %w%s", pluginPath, reason, hint)
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf("plugin %q failed: %w", pluginPath, err)
	}
	return nil
}

// pluginEnvironment returns inherited with the variables of Gondola added at
// the end. exec.Cmd keeps only the last value of a variable that appears
// twice, so these replace any inherited ones; on Windows it compares names
// ignoring letter case, as Windows does.
func pluginEnvironment(inherited []string, absoluteTempDir string, mode Mode) []string {
	environment := make([]string, 0, len(inherited)+2)
	environment = append(environment, inherited...)
	return append(environment,
		outputVariable+"="+absoluteTempDir,
		modeVariable+"="+string(mode),
	)
}

// diskPath turns slashPath, relative to dir and written with "/", into a
// path on disk.
func diskPath(dir, slashPath string) string {
	return filepath.Join(dir, filepath.FromSlash(slashPath))
}

// withoutPath returns the reason of a file system error without the path
// that the os package puts into it; messages name the plugin themselves,
// relative to the project root and with "/".
func withoutPath(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
