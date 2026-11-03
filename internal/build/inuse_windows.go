// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"errors"
	"syscall"
)

// errorSharingViolation is ERROR_SHARING_VIOLATION of winerror.h, which
// package syscall does not name: "The process cannot access the file because
// it is being used by another process."
const errorSharingViolation syscall.Errno = 32

// isInUse reports whether err is Windows refusing to delete or rename a file
// or directory that is in use. Renaming a directory that holds an open file
// gives "Access is denied." instead of the sharing violation.
func isInUse(err error) bool {
	return errors.Is(err, errorSharingViolation) || errors.Is(err, syscall.ERROR_ACCESS_DENIED)
}
