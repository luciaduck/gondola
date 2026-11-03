// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// The build constraint below compiles this file everywhere except Windows;
// ansi_windows.go holds the Windows version of the same function.

//go:build !windows

package console

import "os"

// enableANSI reports whether file accepts ANSI sequences. Terminals outside
// Windows always do.
func enableANSI(*os.File) bool {
	return true
}
