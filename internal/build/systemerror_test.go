package build

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// systemErrorText returns the system's own text for the error that cause
// makes inside a new directory: the reason that *fs.PathError and
// *os.LinkError wrap, such as "no such file or directory" on Linux and "The
// system cannot find the file specified." on Windows. Messages that end with
// such a reason are compared with it, so they stay exact on every system.
func systemErrorText(t testing.TB, cause func(dir string) error) string {
	t.Helper()
	err := cause(t.TempDir())
	reason := errors.Unwrap(err)
	if reason == nil {
		t.Fatalf("got %v, want an error that wraps the system's reason", err)
	}
	return reason.Error()
}

// missingFileError renames a file that does not exist.
func missingFileError(dir string) error {
	return os.Rename(filepath.Join(dir, "missing"), filepath.Join(dir, "renamed"))
}

// fileAsDirectoryError writes a file below a path that is a file.
func fileAsDirectoryError(dir string) error {
	filePath := filepath.Join(dir, "file")
	if err := os.WriteFile(filePath, nil, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(filePath, "below"), nil, 0o644)
}

// readDirectoryError reads a directory as a file.
func readDirectoryError(dir string) error {
	_, err := os.ReadFile(dir)
	return err
}
