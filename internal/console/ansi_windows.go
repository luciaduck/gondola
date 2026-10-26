// This file is compiled only on Windows (the _windows suffix of its name is
// a build constraint); ansi_other.go holds the version for other systems.
// It is not tested on Windows yet: that starts with milestone M14.

package console

import (
	"os"
	"syscall"
)

// enableVirtualTerminalProcessing is the console mode flag that makes the
// Windows console interpret ANSI sequences (ENABLE_VIRTUAL_TERMINAL_PROCESSING).
const enableVirtualTerminalProcessing = 0x0004

// enableANSI turns on ANSI processing for the console behind file and reports
// whether it is on. It fails on old consoles and when file is not a console;
// Gondola then prints without color.
func enableANSI(file *os.File) bool {
	handle := syscall.Handle(file.Fd())
	var mode uint32
	if err := syscall.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	// The syscall package has GetConsoleMode but no SetConsoleMode, so it is
	// called from kernel32.dll directly. A non-zero result means success.
	setConsoleMode := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")
	result, _, _ := setConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
	return result != 0
}
