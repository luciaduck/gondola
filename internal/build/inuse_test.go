package build

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"syscall"
	"testing"
)

func TestInUseHint(t *testing.T) {
	const hint = " (close any program or terminal that is using _site, then try again)"
	// Error numbers of winerror.h: they mean "in use" only on Windows.
	const (
		fileNotFound     syscall.Errno = 2
		accessDenied     syscall.Errno = 5
		sharingViolation syscall.Errno = 32
	)
	onWindows := func(want string) string {
		if runtime.GOOS == "windows" {
			return want
		}
		return ""
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"sharing violation", &os.PathError{Op: "unlinkat", Path: "_site", Err: sharingViolation}, onWindows(hint)},
		{"access denied while renaming", &os.LinkError{Op: "rename", Old: "_site.temp", New: "_site", Err: accessDenied}, onWindows(hint)},
		{"another error", &os.PathError{Op: "unlinkat", Path: "_site", Err: fileNotFound}, ""},
		{"no system error", os.ErrInvalid, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := InUseHint(test.err, "_site"); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

// TestBuildOutputInUse checks that a build fails at once, before writing
// anything, when Windows refuses to delete the output because a file in it
// is open, and that the error says what to do.
func TestBuildOutputInUse(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses to delete a directory that is in use")
	}
	projectDir := writeProject(t, map[string]string{
		"index.html":       "<p>home</p>\n",
		"_site/index.html": "<p>old</p>\n",
	})
	// Go opens files without FILE_SHARE_DELETE, as many editors do.
	openFile, err := os.Open(filepath.Join(projectDir, "_site", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	defer openFile.Close()
	result := runBuild(t, projectDir)
	want := []string{"deleting _site: The process cannot access the file because it is being used by another process." +
		" (close any program or terminal that is using _site, then try again)"}
	if !reflect.DeepEqual(result.errorTexts(), want) {
		t.Errorf("errors %q, want %q", result.errorTexts(), want)
	}
	if dirExists(t, filepath.Join(projectDir, "_site.temp")) {
		t.Error("the temp output was created after the output could not be deleted")
	}
}
