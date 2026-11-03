// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import "fmt"

// InUseHint returns the hint that follows the error of deleting or renaming
// the directories that dirsDescription names, such as "_site" or
// "_site.temp or _site", or "" unless the system refused because they are in
// use (M14, Q6). Only Windows refuses: a file open inside the directory, or a
// program whose working directory is inside it, such as a terminal after
// "cd _site", blocks both deleting and renaming.
func InUseHint(err error, dirsDescription string) string {
	if !isInUse(err) {
		return ""
	}
	return fmt.Sprintf(" (close any program or terminal that is using %s, then try again)", dirsDescription)
}
