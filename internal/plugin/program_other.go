// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

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
