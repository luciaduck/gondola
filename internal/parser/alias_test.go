package parser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

func TestParseAliasFile(t *testing.T) {
	const page = ":::\\a\ntext\n:::\n"
	tests := []struct {
		name string
		src  string
		// wantPage is the Squero output of page with these aliases.
		wantPage string
	}{
		{"one alias", ":::\\a . section +x\n", "<section class=\"x\">\n\ntext\n\n</section>\n"},
		{"CRLF", ":::\\b . p\r\n:::\\a . section +x\r\n", "<section class=\"x\">\n\ntext\n\n</section>\n"},
		{"no final newline", ":::\\a . section +x", "<section class=\"x\">\n\ntext\n\n</section>\n"},
		{"byte order mark", byteOrderMark + ":::\\a . section +x\n", "<section class=\"x\">\n\ntext\n\n</section>\n"},
		{"blank lines", "\n   \n:::\\a . section\n\n", "<section>\n\ntext\n\n</section>\n"},
		{"comment", ":::\\a . section /the main section\n", "<section>\n\ntext\n\n</section>\n"},
		{"documentation only: the default tag", ":::\\a / documentation\n", "<div>\n\ntext\n\n</div>\n"},
		{"no body: the default tag", ":::\\a\n", "<div>\n\ntext\n\n</div>\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aliasFile, err := ParseAliasFile([]byte(test.src))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if !aliasFile.Found {
				t.Error("Found is false")
			}
			got, err := New(aliasFile).Squero(testFile, []byte(page), 0)
			if err != nil {
				t.Fatalf("Squero: unexpected error %v", err)
			}
			if string(got) != test.wantPage {
				t.Errorf("page:\n got %q\nwant %q", got, test.wantPage)
			}
		})
	}
}

func TestParseAliasFileEmpty(t *testing.T) {
	for _, src := range []string{"", "\n", "  \t\n\r\n", byteOrderMark} {
		aliasFile, err := ParseAliasFile([]byte(src))
		if err != nil {
			t.Errorf("%q: unexpected error %v", src, err)
			continue
		}
		if !aliasFile.Found || len(aliasFile.Aliases) != 0 {
			t.Errorf("%q: got %+v, want a found file without aliases", src, aliasFile)
		}
	}
}

func TestParseAliasFileErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"no colons", "note . aside\n", `alias.squero:1:1: alias definition must start with at least three colons`},
		{"no backslash", ":::note\n", `alias.squero:1:4: alias definition must continue with "\name"`},
		{"empty name", ":::\\ . div\n", `alias.squero:1:4: empty alias name`},
		{"no space after the dot", ":::\\a .div\n", `alias.squero:1:8: the '.' separator must be followed by a space`},
		{"no dot", ":::\\a x\n", `alias.squero:1:7: expected '.' or '/' after the alias name, found 'x'`},
		{"alias of an alias", ":::\\a . \\b\n", `alias.squero:1:9: alias "a" cannot reference another alias`},
		{"defined twice", ":::\\a . div\n:::\\a . p\n", `alias.squero:2:5: alias "a" is already defined`},
		{"bad header", ":::\\a . div ?x[\n", `alias.squero:1:13: unterminated value for attribute "x": missing ']'`},
		{"after blank lines", "\n\n:::\\a . div ?x[\n", `alias.squero:3:13: unterminated value for attribute "x": missing ']'`},
		{"CRLF", ":::\\b . p\r\n:::\\a . div ?x[\r\n", `alias.squero:2:13: unterminated value for attribute "x": missing ']'`},
		{
			// Columns do not count the byte order mark (package sourcetext).
			"byte order mark",
			byteOrderMark + ":::\\a . div ?x[\n",
			`alias.squero:1:13: unterminated value for attribute "x": missing ']'`,
		},
		{"UTF-16", "\xFF\xFE:\x00", "alias.squero: " + sourcetext.UTF16Message},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			aliasFile, err := ParseAliasFile([]byte(test.src))
			if err == nil {
				t.Fatalf("no error, got %+v; want %s", aliasFile, test.want)
			}
			if err.Error() != test.want {
				t.Errorf("error:\n got %s\nwant %s", err, test.want)
			}
			var diagErr *diag.Error
			if !errors.As(err, &diagErr) {
				t.Errorf("error %T is not a *diag.Error", err)
			}
		})
	}
}

func TestLoadAliasFile(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		aliasFile, err := LoadAliasFile(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		if aliasFile.Found || aliasFile.Aliases != nil {
			t.Errorf("got %+v, want the zero AliasFile", aliasFile)
		}
	})
	t.Run("present", func(t *testing.T) {
		projectDir := t.TempDir()
		writeFile(t, filepath.Join(projectDir, AliasFileName), ":::\\a . section\n")
		aliasFile, err := LoadAliasFile(projectDir)
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		if !aliasFile.Found || len(aliasFile.Aliases) != 1 {
			t.Errorf("got %+v, want one alias", aliasFile)
		}
	})
	t.Run("only in the project directory", func(t *testing.T) {
		projectDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(projectDir, "blog"), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(projectDir, "blog", AliasFileName), ":::\\a . section\n")
		aliasFile, err := LoadAliasFile(projectDir)
		if err != nil || aliasFile.Found {
			t.Errorf("got %+v, %v; want no alias file", aliasFile, err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		projectDir := t.TempDir()
		writeFile(t, filepath.Join(projectDir, AliasFileName), ":::\\a . div\n:::\\a . p\n")
		_, err := LoadAliasFile(projectDir)
		const want = `alias.squero:2:5: alias "a" is already defined`
		if err == nil || err.Error() != want {
			t.Errorf("error:\n got %v\nwant %s", err, want)
		}
	})
	t.Run("a directory", func(t *testing.T) {
		projectDir := t.TempDir()
		if err := os.Mkdir(filepath.Join(projectDir, AliasFileName), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := LoadAliasFile(projectDir)
		if err == nil || !strings.HasPrefix(err.Error(), "reading alias.squero: ") {
			t.Errorf("error %v, want one starting with %q", err, "reading alias.squero: ")
		}
	})
}

// writeFile writes content to path or stops the test.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
