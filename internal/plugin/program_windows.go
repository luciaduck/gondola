package plugin

import (
	"errors"
	"syscall"
)

// errorBadExeFormat is ERROR_BAD_EXE_FORMAT of winerror.h, which package
// syscall does not name: "%1 is not a valid Win32 application." Windows gives
// it for a shell script or a PowerShell file, since it starts only programs
// and batch files.
const errorBadExeFormat syscall.Errno = 193

// notAProgramHint follows errorBadExeFormat's message (M14, Q4).
const notAProgramHint = ` (on Windows a plugin must be a program, such as a .exe file)`

// isNotAProgram reports whether err is the error of starting a file that the
// system cannot run.
func isNotAProgram(err error) bool {
	return errors.Is(err, errorBadExeFormat)
}
