package frontmatter

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
)

// testFile is the file name that tests pass to Split.
const testFile = "blog/post.md"

// byteOrderMark is built at run time: a raw byte order mark inside a Go
// source file is a compile error.
var byteOrderMark = string(rune(0xFEFF))

func TestSplitBody(t *testing.T) {
	tests := []struct {
		name          string
		src           string
		wantVariables map[string]any
		wantBody      string
		wantOffset    int
	}{
		{"no front matter", "# Hello\n", map[string]any{}, "# Hello\n", 0},
		{"empty file", "", map[string]any{}, "", 0},
		{"empty block", "---\n---\n# Hello\n", map[string]any{}, "# Hello\n", 2},
		{"block with a comment only", "---\n# just a note\n---\nbody", map[string]any{}, "body", 3},
		{"block holding null", "---\n~\n---\nbody", map[string]any{}, "body", 3},
		{"one key", "---\ntitle: Home\n---\n# Hello\n", map[string]any{"title": "Home"}, "# Hello\n", 3},
		{"nothing after the block", "---\ntitle: Home\n---\n", map[string]any{"title": "Home"}, "", 3},
		{"closing line at the end of the file", "---\ntitle: Home\n---", map[string]any{"title": "Home"}, "", 2},
		{"blank lines after the block stay in the body", "---\ntitle: Home\n---\n\n\nbody\n", map[string]any{"title": "Home"}, "\n\nbody\n", 3},
		{"CRLF", "---\r\ntitle: Home\r\n---\r\nbody\r\n", map[string]any{"title": "Home"}, "body\r\n", 3},
		{"spaces after the delimiters", "--- \t\ntitle: Home\n---  \nbody", map[string]any{"title": "Home"}, "body", 3},
		{"byte order mark", byteOrderMark + "---\ntitle: Home\n---\nbody", map[string]any{"title": "Home"}, "body", 3},
		{"byte order mark without front matter", byteOrderMark + "body", map[string]any{}, "body", 0},
		{"a later --- line belongs to the body", "---\na: 1\n---\nbody\n---\nmore\n", map[string]any{"a": 1}, "body\n---\nmore\n", 3},
		// The block must start on the first line, so a Markdown page that
		// starts with a horizontal rule after a blank line is all body.
		{"blank line before the delimiter", "\n---\ntitle: Home\n---\nbody", map[string]any{}, "\n---\ntitle: Home\n---\nbody", 0},
		{"indented delimiter", " ---\ntitle: Home\n---\nbody", map[string]any{}, " ---\ntitle: Home\n---\nbody", 0},
		{"four dashes", "----\ntitle: Home\n----\nbody", map[string]any{}, "----\ntitle: Home\n----\nbody", 0},
		{"delimiter with text after it", "---yaml\ntitle: Home\n---\nbody", map[string]any{}, "---yaml\ntitle: Home\n---\nbody", 0},
		{"indented closing line is not a delimiter", "---\na: |\n  ---\n---\nbody", map[string]any{"a": "---\n"}, "body", 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Split(testFile, []byte(test.src))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Variables, test.wantVariables) {
				t.Errorf("variables %#v, want %#v", got.Variables, test.wantVariables)
			}
			if string(got.Body) != test.wantBody {
				t.Errorf("body %q, want %q", got.Body, test.wantBody)
			}
			if got.BodyLineOffset != test.wantOffset {
				t.Errorf("line offset %d, want %d", got.BodyLineOffset, test.wantOffset)
			}
			// The body is exactly the end of the file (SPEC §5.4, V1).
			if !bytes.HasSuffix([]byte(test.src), got.Body) {
				t.Errorf("body %q is not the end of the file", got.Body)
			}
		})
	}
}

func TestSplitAbsentAndEmptyBlockAgree(t *testing.T) {
	// An absent block and an empty block give the same variables (SPEC
	// §5.4, V1); only the lines before the body differ.
	absent, err := Split(testFile, []byte("body\n"))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := Split(testFile, []byte("---\n---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(absent.Variables, empty.Variables) || !bytes.Equal(absent.Body, empty.Body) {
		t.Errorf("absent %+v, empty %+v", absent, empty)
	}
}

func TestSplitErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"unterminated", "---\ntitle: Home\n# Hello\n", testFile + `:1:1: the front matter that starts here has no closing "---" line; please add one`},
		{"only the opening line", "---\n", testFile + `:1:1: the front matter that starts here has no closing "---" line; please add one`},
		{"opening line without line break", "---", testFile + `:1:1: the front matter that starts here has no closing "---" line; please add one`},
		{"unterminated after a byte order mark", byteOrderMark + "---\na: 1\n", testFile + `:1:1: the front matter that starts here has no closing "---" line; please add one`},
		{"UTF-16 little-endian", "\xFF\xFE-\x00-\x00-\x00", testFile + ": the file is saved as UTF-16; please save it as UTF-8"},
		{"UTF-16 big-endian", "\xFE\xFF\x00-\x00-\x00-", testFile + ": the file is saved as UTF-16; please save it as UTF-8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Split(testFile, []byte(test.src))
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
			var diagErr *diag.Error
			if !errors.As(err, &diagErr) || diagErr.File != testFile {
				t.Errorf("error %#v does not name the file", err)
			}
		})
	}
}
