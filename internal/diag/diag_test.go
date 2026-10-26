package diag

import (
	"errors"
	"fmt"
	"testing"
)

var errCause = errors.New("yaml: mapping values are not allowed here")

func TestErrorString(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "full position",
			err:  &Error{File: "blog/first-post.md", Line: 14, Col: 3, Msg: "missing ']'"},
			want: "blog/first-post.md:14:3: missing ']'",
		},
		{
			name: "no column",
			err:  &Error{File: "about.md", Line: 3, Msg: "bad"},
			want: "about.md:3: bad",
		},
		{
			name: "column without line is ignored",
			err:  &Error{File: "about.md", Col: 7, Msg: "bad"},
			want: "about.md: bad",
		},
		{
			name: "no line",
			err:  &Error{File: "about.md", Msg: "bad"},
			want: "about.md: bad",
		},
		{
			name: "no file",
			err:  &Error{Line: 2, Col: 5, Msg: "bad"},
			want: "2:5: bad",
		},
		{
			name: "no position",
			err:  &Error{Msg: "bad"},
			want: "bad",
		},
		{
			name: "message and cause",
			err:  &Error{File: "a.md", Line: 1, Col: 1, Msg: "invalid front matter", Err: errCause},
			want: "a.md:1:1: invalid front matter: yaml: mapping values are not allowed here",
		},
		{
			name: "cause only",
			err:  &Error{File: "a.md", Line: 4, Err: errCause},
			want: "a.md:4: yaml: mapping values are not allowed here",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Errorf("Error() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestUnwrap(t *testing.T) {
	err := fmt.Errorf("rendering page: %w", &Error{File: "a.md", Line: 1, Err: errCause})
	if !errors.Is(err, errCause) {
		t.Errorf("errors.Is did not find the cause through *Error")
	}
	var diagErr *Error
	if !errors.As(err, &diagErr) || diagErr.File != "a.md" {
		t.Errorf("errors.As did not find the *Error")
	}
}

func TestWithFile(t *testing.T) {
	tests := []struct {
		name string
		err  func() error
		file string
		want string
	}{
		{
			name: "sets the file",
			err:  func() error { return &Error{Line: 14, Col: 3, Msg: "missing ']'"} },
			file: "blog/first-post.md",
			want: "blog/first-post.md:14:3: missing ']'",
		},
		{
			// fmt.Errorf has already written the message; WithFile must be
			// called before wrapping.
			name: "wrapped errors are unchanged",
			err: func() error {
				return fmt.Errorf("running squero: %w", &Error{Line: 2, Col: 1, Msg: "bad fence"})
			},
			file: "index.md",
			want: "running squero: 2:1: bad fence",
		},
		{
			name: "keeps a file that is already set",
			err:  func() error { return &Error{File: "snippets/note.md", Line: 2, Msg: "bad fence"} },
			file: "index.md",
			want: "snippets/note.md:2: bad fence",
		},
		{
			name: "error without position",
			err:  func() error { return &Error{Msg: "bad"} },
			file: "index.md",
			want: "index.md: bad",
		},
		{
			name: "other errors are unchanged",
			err:  func() error { return errCause },
			file: "index.md",
			want: errCause.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := WithFile(test.err(), test.file)
			if got.Error() != test.want {
				t.Errorf("WithFile() = %q, want %q", got.Error(), test.want)
			}
		})
	}
}

func TestWithFileReturnsACopy(t *testing.T) {
	original := &Error{Line: 3, Msg: "bad"}
	WithFile(original, "a.md")
	ShiftLines(original, 10)
	if original.File != "" || original.Line != 3 {
		t.Errorf("the original error was changed: %+v", original)
	}
}

func TestWithFileNil(t *testing.T) {
	if err := WithFile(nil, "a.md"); err != nil {
		t.Errorf("WithFile(nil) = %v, want nil", err)
	}
}

func TestShiftLines(t *testing.T) {
	tests := []struct {
		name       string
		err        func() error
		lineOffset int
		want       string
	}{
		{
			name:       "shifts the line",
			err:        func() error { return &Error{Line: 9, Col: 3, Msg: "missing ']'"} },
			lineOffset: 5,
			want:       "14:3: missing ']'",
		},
		{
			name:       "zero offset",
			err:        func() error { return &Error{Line: 9, Col: 3, Msg: "bad"} },
			lineOffset: 0,
			want:       "9:3: bad",
		},
		{
			name: "wrapped errors are unchanged",
			err: func() error {
				return fmt.Errorf("running squero: %w", &Error{Line: 1, Col: 1, Msg: "bad"})
			},
			lineOffset: 4,
			want:       "running squero: 1:1: bad",
		},
		{
			name:       "unknown line stays unknown",
			err:        func() error { return &Error{Msg: "bad"} },
			lineOffset: 4,
			want:       "bad",
		},
		{
			name:       "error that names a file is final",
			err:        func() error { return &Error{File: "snippets/note.md", Line: 2, Msg: "bad"} },
			lineOffset: 4,
			want:       "snippets/note.md:2: bad",
		},
		{
			name:       "other errors are unchanged",
			err:        func() error { return errCause },
			lineOffset: 4,
			want:       errCause.Error(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ShiftLines(test.err(), test.lineOffset)
			if got.Error() != test.want {
				t.Errorf("ShiftLines() = %q, want %q", got.Error(), test.want)
			}
		})
	}
}

func TestShiftLinesThenWithFile(t *testing.T) {
	// The order used for a page body: shift by the frontmatter, then name the
	// page.
	err := WithFile(ShiftLines(&Error{Line: 9, Col: 3, Msg: "missing ']'"}, 5), "blog/first-post.md")
	want := "blog/first-post.md:14:3: missing ']'"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestShiftLinesNil(t *testing.T) {
	if err := ShiftLines(nil, 3); err != nil {
		t.Errorf("ShiftLines(nil) = %v, want nil", err)
	}
}
