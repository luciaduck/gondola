// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package console

import "os"

// ColorFor reports whether ANSI colors may be written to file. Colors are
// used only when file is a terminal, the NO_COLOR environment variable is
// empty or unset (noColor is its value), and, on Windows, the console accepts
// ANSI sequences. Each stream is checked on its own, so that redirecting
// standard error to a file does not fill the file with escape sequences.
func ColorFor(file *os.File, noColor string) bool {
	return useColor(isTerminal(file), noColor) && enableANSI(file)
}

// useColor holds the rule that does not depend on the operating system. It
// follows no-color.org: only a non-empty NO_COLOR turns colors off.
func useColor(isTerminal bool, noColor string) bool {
	return isTerminal && noColor == ""
}

// isTerminal reports whether file is a character device, which is what a
// terminal is. Pipes and regular files are not.
func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
