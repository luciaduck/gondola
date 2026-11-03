// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package sourcetext holds the Encoding rule of SPEC §4, shared by every
// package that reads a source file (gondola.yaml, pages, layouts,
// alias.squero, and later the data files): UTF-16 is refused, and a UTF-8
// byte order mark is allowed but is not part of the text.
//
// Positions in errors never count the byte order mark: column 1 of line 1 is
// the first character after it, and the console does not print it.
package sourcetext

import (
	"bytes"

	"github.com/luciaduck/gondola/internal/diag"
)

// utf8ByteOrderMark may start a UTF-8 file; some Windows editors write it.
// It is written as bytes because a raw mark in Go source does not compile.
var utf8ByteOrderMark = []byte("\xEF\xBB\xBF")

// utf16ByteOrderMarks start a file saved as UTF-16, little-endian and
// big-endian. Windows PowerShell 5.1 writes such files by default.
var utf16ByteOrderMarks = [][]byte{{0xFF, 0xFE}, {0xFE, 0xFF}}

// UTF16Message is the error for a file saved as UTF-16. It has no position:
// the console could not print a line of such a file.
const UTF16Message = "the file is saved as UTF-16; please save it as UTF-8"

// CheckEncoding returns a *diag.Error without a file name if src starts with
// a UTF-16 byte order mark. yaml.v3 would read UTF-16, but the positions in
// its errors would count the decoded text, not the bytes of the file, and
// every other source file Gondola reads is UTF-8. A UTF-8 byte order mark is
// fine.
func CheckEncoding(src []byte) error {
	for _, byteOrderMark := range utf16ByteOrderMarks {
		if bytes.HasPrefix(src, byteOrderMark) {
			return &diag.Error{Msg: UTF16Message}
		}
	}
	return nil
}

// TrimByteOrderMark returns src without its UTF-8 byte order mark, if it has
// one. The result is a slice of src. Read files through it before counting
// positions, so that columns on line 1 do not count the mark.
func TrimByteOrderMark(src []byte) []byte {
	return bytes.TrimPrefix(src, utf8ByteOrderMark)
}
