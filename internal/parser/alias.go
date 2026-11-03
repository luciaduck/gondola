// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
	"github.com/luciaduck/squero"
)

// AliasFileName is the file that defines the fence aliases. It is read from
// the project directory only, never from a subdirectory (SPEC §5.5).
const AliasFileName = "alias.squero"

// AliasFile is what Gondola read from alias.squero.
type AliasFile struct {
	// Aliases holds the aliases the file defines; nil when Found is false.
	Aliases squero.Aliases
	// Found reports whether the project has an alias.squero file. Without
	// one, the message for an unknown alias says so.
	Found bool
}

// LoadAliasFile reads alias.squero from projectDir. A missing file is not an
// error: Squero then runs without aliases and Found is false. Read it only
// when fenced-blocks is on, so that a malformed file that nothing uses does
// not stop the build.
func LoadAliasFile(projectDir string) (AliasFile, error) {
	src, err := os.ReadFile(filepath.Join(projectDir, AliasFileName))
	if errors.Is(err, fs.ErrNotExist) {
		return AliasFile{}, nil
	}
	if err != nil {
		return AliasFile{}, fmt.Errorf("reading %s: %w", AliasFileName, err)
	}
	return ParseAliasFile(src)
}

// ParseAliasFile parses src, the contents of alias.squero. Errors are
// *diag.Error values naming AliasFileName, at the position Squero reports.
func ParseAliasFile(src []byte) (AliasFile, error) {
	if err := sourcetext.CheckEncoding(src); err != nil {
		return AliasFile{}, diag.WithFile(err, AliasFileName)
	}
	// Squero would read a byte order mark as part of the first definition.
	aliases, err := squero.ParseAliases(sourcetext.TrimByteOrderMark(src))
	if err == nil {
		return AliasFile{Aliases: aliases, Found: true}, nil
	}
	diagErr, ok := fromSqueroError(err)
	if !ok {
		return AliasFile{}, fmt.Errorf("parsing %s: %w", AliasFileName, err)
	}
	return AliasFile{}, diag.WithFile(diagErr, AliasFileName)
}
