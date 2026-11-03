// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package server

import (
	"bufio"
	"io"
	"strings"
)

// The commands typed on standard input, each followed by Enter. Letter case
// and spaces around them do not matter (SPEC §5.16, point 6).
const (
	rebuildCommand = "g"
	quitCommand    = "q"
)

// commands carries what the user typed to the server's loop. rebuild and
// unknown hold at most one signal each, so that several lines typed during a
// build are answered once when it ends.
type commands struct {
	rebuild chan struct{}
	unknown chan struct{}
	// quit is closed by q.
	quit chan struct{}
}

func newCommands() commands {
	return commands{
		rebuild: make(chan struct{}, 1),
		unknown: make(chan struct{}, 1),
		quit:    make(chan struct{}),
	}
}

// readCommands reads input line by line until q, the end of input, or a read
// error. At the end of input it only stops reading: the server goes on
// until Ctrl+C, so that `gondola serve < /dev/null` keeps serving.
func readCommands(input io.Reader, received commands) {
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		switch line := strings.TrimSpace(scanner.Text()); {
		case strings.EqualFold(line, rebuildCommand):
			signal(received.rebuild)
		case strings.EqualFold(line, quitCommand):
			close(received.quit)
			return
		default:
			signal(received.unknown)
		}
	}
	// A read error, such as a line longer than the scanner's buffer, ends
	// the commands as the end of input does: serving goes on, and Ctrl+C
	// still closes the server.
	_ = scanner.Err()
}

// signal sends on a channel with room for one signal, unless one is already
// waiting there.
func signal(pending chan struct{}) {
	select {
	case pending <- struct{}{}:
	default:
	}
}
