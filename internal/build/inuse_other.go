// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

//go:build !windows

package build

// isInUse reports whether err is the system refusing to delete or rename a
// directory that is in use; only Windows refuses.
func isInUse(err error) bool {
	return false
}
