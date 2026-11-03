// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package sass

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// executableName is the name looked up in PATH. On Windows, exec.LookPath
// adds the extensions of PATHEXT, so it also finds the sass.bat of the Dart
// Sass release; godartsass looks for the same name.
const executableName = "sass"

// installURL is where the messages send the user to install Dart Sass.
const installURL = "https://sass-lang.com/install"

// minimumVersion is the oldest Dart Sass that Gondola accepts: 1.63.0 is the
// first release whose sass executable runs the embedded protocol 2.0 with
// --embedded, which is what godartsass/v2 speaks (Dart Sass changelog,
// 1.63.0). Older releases do not know --embedded at all.
var minimumVersion = version{1, 63, 0}

// maxQuotedOutputLength limits how much of what "sass --version" printed is
// quoted in a message, in bytes, in case the "sass" in PATH is some other
// program that prints a lot.
const maxQuotedOutputLength = 100

// dartSassVersionPattern matches what Dart Sass prints for --version: only
// the version, such as "1.106.1", sometimes with a pre-release suffix.
var dartSassVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:[-+]\S*)?$`)

// javaScriptMarker is part of what the pure JavaScript build of Sass (the
// "sass" package on npm) prints for --version, as in
// "1.80.0 compiled with dart2js 3.5.0". That build cannot run --embedded.
const javaScriptMarker = "compiled with dart2js"

// version is a Dart Sass version: major, minor, patch.
type version [3]int

func (v version) String() string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

func (v version) isOlderThan(other version) bool {
	for i := range v {
		if v[i] != other[i] {
			return v[i] < other[i]
		}
	}
	return false
}

// FindExecutable finds the Dart Sass executable in PATH and checks that it is
// Dart Sass, at least minimumVersion. neededFor is the source path of a Sass
// entry point, named in the message when the executable is missing. It
// returns the executable's path, to be given to Start, so that the checked
// executable is the one that runs.
//
// Call it only when config.sass is true and scan found an entry point: a site
// without Sass files does not need Dart Sass installed.
func FindExecutable(neededFor string) (string, error) {
	executablePath, err := exec.LookPath(executableName)
	if err != nil {
		return "", fmt.Errorf("Dart Sass is needed to compile %s, but %q was not found in PATH; please install Dart Sass %s or newer from %s, or set config.sass to false",
			neededFor, displayedExecutableName(), minimumVersion, installURL)
	}
	output, err := exec.Command(executablePath, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("the %q in PATH (%s) could not be run: %w; please check the installation of Dart Sass (%s)",
			displayedExecutableName(), filepath.ToSlash(executablePath), err, installURL)
	}
	if err := checkVersionOutput(executablePath, string(output)); err != nil {
		return "", err
	}
	return executablePath, nil
}

// displayedExecutableName is the executable's name as the user knows it on
// this system: the Dart Sass release for Windows ships sass.bat.
func displayedExecutableName() string {
	if runtime.GOOS == "windows" {
		return "sass.bat"
	}
	return executableName
}

// checkVersionOutput checks what "sass --version" printed: Dart Sass prints
// its version alone; anything else is another program named sass.
func checkVersionOutput(executablePath, output string) error {
	firstLine, _, _ := strings.Cut(strings.TrimSpace(output), "\n")
	firstLine = strings.TrimSpace(firstLine)
	where := fmt.Sprintf("the %q in PATH (%s)", displayedExecutableName(), filepath.ToSlash(executablePath))

	found, ok := parseVersion(firstLine)
	if !ok {
		printed := quotedOutput(firstLine)
		if strings.Contains(firstLine, javaScriptMarker) {
			printed += ", the JavaScript version from npm"
		}
		return fmt.Errorf("%s is not Dart Sass (it printed %s); please install Dart Sass from %s", where, printed, installURL)
	}
	if found.isOlderThan(minimumVersion) {
		return fmt.Errorf("%s is Dart Sass %s; Gondola needs %s or newer, so please update it from %s", where, firstLine, minimumVersion, installURL)
	}
	return nil
}

// parseVersion reads a version printed by Dart Sass, such as "1.106.1".
func parseVersion(text string) (version, bool) {
	match := dartSassVersionPattern.FindStringSubmatch(text)
	if match == nil {
		return version{}, false
	}
	var parsed version
	for i := range parsed {
		number, err := strconv.Atoi(match[i+1])
		if err != nil {
			// Only a number too large for an int gets here.
			return version{}, false
		}
		parsed[i] = number
	}
	return parsed, true
}

// quotedOutput quotes text for a message, cut to maxQuotedOutputLength bytes.
func quotedOutput(text string) string {
	if len(text) <= maxQuotedOutputLength {
		return strconv.Quote(text)
	}
	// Cutting may split a multi-byte character; strconv.Quote then shows the
	// stray bytes as \x escapes, which is fine for a cut excerpt.
	return strconv.Quote(text[:maxQuotedOutputLength]) + "..."
}
