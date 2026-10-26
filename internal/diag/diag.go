// Package diag defines the error type for problems that point at a position
// in a source file, so that the console can print the offending line with a
// caret under it.
package diag

import (
	"path/filepath"
	"strconv"
)

// Error is a problem at a known position in a source file.
//
// Line and Col start at 1; zero means unknown. Col counts bytes from the
// start of the line, which is how Squero reports columns. Line refers to the
// whole file, frontmatter included.
type Error struct {
	// File is the source file, relative to the project root and written
	// with "/" on every platform. Empty when it is not known yet.
	File string
	Line int
	Col  int
	// Msg is the message for the user. When it is empty, the message of Err
	// is used instead.
	Msg string
	// Err is the underlying cause, if any.
	Err error
}

// Error formats the error as "file:line:col: message", leaving out the parts
// of the position that are not known.
func (e *Error) Error() string {
	position := e.position()
	message := e.message()
	if position == "" {
		return message
	}
	return position + ": " + message
}

// Unwrap returns the underlying cause, so that errors.Is and errors.As can
// look through an Error.
func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) position() string {
	position := e.File
	if e.Line > 0 {
		if position != "" {
			position += ":"
		}
		position += strconv.Itoa(e.Line)
		if e.Col > 0 {
			position += ":" + strconv.Itoa(e.Col)
		}
	}
	return position
}

func (e *Error) message() string {
	switch {
	case e.Err == nil:
		return e.Msg
	case e.Msg == "":
		return e.Err.Error()
	default:
		return e.Msg + ": " + e.Err.Error()
	}
}

// WithFile returns err with its file name set to file, when err is an
// *Error that does not name a file yet. An error that already names a file
// keeps it: an error raised inside an included fragment must keep pointing at
// the fragment, not at the page that included it. Other errors are returned
// unchanged.
//
// Call WithFile before wrapping err with fmt.Errorf: the wrapping copies the
// message text when it is created, so a file added afterwards would not
// appear in it. For the same reason only an *Error itself is changed, never
// one found inside a wrapping error.
func WithFile(err error, file string) error {
	diagErr, ok := err.(*Error)
	if !ok || diagErr.File != "" {
		return err
	}
	// Return a changed copy, so that the caller's error value stays as it was.
	withFile := *diagErr
	withFile.File = filepath.ToSlash(file)
	return &withFile
}

// ShiftLines returns err with lineOffset added to its line, when err is an
// *Error with a known line that does not name a file yet. It turns a line
// counted in a page's body into a line of the whole file, by adding the number
// of lines taken by the frontmatter.
//
// An error that already names a file is returned unchanged: its position is
// already final. As with WithFile, call it before wrapping err.
func ShiftLines(err error, lineOffset int) error {
	diagErr, ok := err.(*Error)
	if !ok || diagErr.File != "" || diagErr.Line <= 0 {
		return err
	}
	shifted := *diagErr
	shifted.Line += lineOffset
	return &shifted
}
