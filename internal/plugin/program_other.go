//go:build !windows

package plugin

import (
	"errors"
	"syscall"
)

// notAProgramHint follows "exec format error", the error of a script without
// a "#!" line (M11, Q3).
const notAProgramHint = ` (a script needs a "#!" line naming its interpreter)`

// isNotAProgram reports whether err is the error of starting a file that the
// system cannot run.
func isNotAProgram(err error) bool {
	return errors.Is(err, syscall.ENOEXEC)
}
