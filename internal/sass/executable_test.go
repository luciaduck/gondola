package sass

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		text   string
		want   version
		wantOK bool
	}{
		{"1.106.1", version{1, 106, 1}, true},
		{"1.63.0", version{1, 63, 0}, true},
		{"2.0.0-beta.1", version{2, 0, 0}, true},
		{"1.63.0+build", version{1, 63, 0}, true},
		{"1.80.0 compiled with dart2js 3.5.0", version{}, false},
		{"Ruby Sass 3.7.4", version{}, false},
		{"1.63", version{}, false},
		{"v1.63.0", version{}, false},
		{"", version{}, false},
		{"99999999999999999999.0.0", version{}, false},
	}
	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			got, ok := parseVersion(test.text)
			if got != test.want || ok != test.wantOK {
				t.Errorf("parseVersion(%q) = %v, %v; want %v, %v", test.text, got, ok, test.want, test.wantOK)
			}
		})
	}
}

func TestVersionIsOlderThan(t *testing.T) {
	tests := []struct {
		v, other version
		want     bool
	}{
		{version{1, 62, 1}, version{1, 63, 0}, true},
		{version{1, 63, 0}, version{1, 63, 0}, false},
		{version{1, 106, 1}, version{1, 63, 0}, false},
		{version{0, 99, 99}, version{1, 0, 0}, true},
		{version{2, 0, 0}, version{1, 63, 0}, false},
		{version{1, 63, 0}, version{1, 63, 1}, true},
	}
	for _, test := range tests {
		if got := test.v.isOlderThan(test.other); got != test.want {
			t.Errorf("%v.isOlderThan(%v) = %v, want %v", test.v, test.other, got, test.want)
		}
	}
}

func TestCheckVersionOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the messages name sass.bat on Windows")
	}
	const executablePath = "/usr/local/bin/sass"
	long := strings.Repeat("x", maxQuotedOutputLength+20)
	tests := []struct {
		name    string
		output  string
		wantErr string
	}{
		{name: "current", output: "1.106.1\n"},
		{name: "oldest accepted", output: "1.63.0\n"},
		{name: "spaces around", output: "  1.70.0  \r\n"},
		{
			name:    "too old",
			output:  "1.58.3\n",
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is Dart Sass 1.58.3; Gondola needs 1.63.0 or newer, so please update it from https://sass-lang.com/install`,
		},
		{
			name:    "JavaScript version from npm",
			output:  "1.80.0 compiled with dart2js 3.5.0\n",
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is not Dart Sass (it printed "1.80.0 compiled with dart2js 3.5.0", the JavaScript version from npm); please install Dart Sass from https://sass-lang.com/install`,
		},
		{
			name:    "Ruby Sass",
			output:  "Ruby Sass 3.7.4\n",
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is not Dart Sass (it printed "Ruby Sass 3.7.4"); please install Dart Sass from https://sass-lang.com/install`,
		},
		{
			name:    "nothing printed",
			output:  "",
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is not Dart Sass (it printed ""); please install Dart Sass from https://sass-lang.com/install`,
		},
		{
			name:    "only the first line is quoted",
			output:  "something else\n1.106.1\n",
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is not Dart Sass (it printed "something else"); please install Dart Sass from https://sass-lang.com/install`,
		},
		{
			name:    "long output is cut",
			output:  long,
			wantErr: `the "sass" in PATH (/usr/local/bin/sass) is not Dart Sass (it printed "` + long[:maxQuotedOutputLength] + `"...); please install Dart Sass from https://sass-lang.com/install`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkVersionOutput(executablePath, test.output)
			checkError(t, err, test.wantErr)
		})
	}
}

// checkError fails the test unless err has exactly the message wantErr, or
// is nil when wantErr is empty.
func checkError(t *testing.T, err error, wantErr string) {
	t.Helper()
	if wantErr == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("no error, want %q", wantErr)
	}
	if err.Error() != wantErr {
		t.Fatalf("error\n got %q\nwant %q", err.Error(), wantErr)
	}
}

func TestFindExecutableNotFound(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the message names sass.bat on Windows")
	}
	t.Setenv("PATH", t.TempDir())
	_, err := FindExecutable("assets/main.scss")
	checkError(t, err, `Dart Sass is needed to compile assets/main.scss, but "sass" was not found in PATH; please install Dart Sass 1.63.0 or newer from https://sass-lang.com/install, or set config.sass to false`)
}

// TestFindExecutableFakes puts small shell scripts named sass in PATH, in
// place of the real executable.
func TestFindExecutableFakes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake executables are shell scripts")
	}
	tests := []struct {
		name    string
		script  string
		wantErr string // %s is replaced by the fake's path
	}{
		{name: "Dart Sass", script: "echo 1.106.1"},
		{
			name:    "too old",
			script:  "echo 1.62.1",
			wantErr: `the "sass" in PATH (%s) is Dart Sass 1.62.1; Gondola needs 1.63.0 or newer, so please update it from https://sass-lang.com/install`,
		},
		{
			name:    "npm",
			script:  "echo '1.80.0 compiled with dart2js 3.5.0'",
			wantErr: `the "sass" in PATH (%s) is not Dart Sass (it printed "1.80.0 compiled with dart2js 3.5.0", the JavaScript version from npm); please install Dart Sass from https://sass-lang.com/install`,
		},
		{
			name:    "fails",
			script:  "exit 3",
			wantErr: `the "sass" in PATH (%s) could not be run: exit status 3; please check the installation of Dart Sass (https://sass-lang.com/install)`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			fakePath := filepath.Join(dir, "sass")
			if err := os.WriteFile(fakePath, []byte("#!/bin/sh\n"+test.script+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			found, err := FindExecutable("assets/main.scss")
			if test.wantErr == "" {
				checkError(t, err, "")
				if found != fakePath {
					t.Errorf("found %q, want %q", found, fakePath)
				}
				return
			}
			checkError(t, err, strings.ReplaceAll(test.wantErr, "%s", fakePath))
		})
	}
}

func TestFindExecutableRealSass(t *testing.T) {
	requireSass(t)
}

// requireSass returns the Dart Sass executable, or skips the test when it is
// not installed. A sass that is installed but refused fails the test.
func requireSass(t testing.TB) string {
	t.Helper()
	if _, err := exec.LookPath(executableName); err != nil {
		t.Skip("Dart Sass is not installed")
	}
	executablePath, err := FindExecutable("test.scss")
	if err != nil {
		t.Fatal(err)
	}
	return executablePath
}
