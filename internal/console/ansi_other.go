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
