package frontmatter

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
)

// FuzzSplit checks what must hold for any file: Split does not panic; on
// success the body is exactly the end of the file and the line offset
// counts the lines before it; on failure the error names the file and
// points at a line and column that the console can print.
func FuzzSplit(f *testing.F) {
	seeds := []string{
		"",
		"# Hello\n",
		"---\n---\n",
		"---\ntitle: Home\n---\nbody\n",
		"---\r\ntitle: Home\r\n---\r\nbody\r\n",
		"\xEF\xBB\xBF---\ntitle: Home\n---\n",
		"---\ntitle: Home\n",
		"---",
		"---\n- a\n---\n",
		"---\ntags: [a, fleet]\ndate: 2024-05-01\nsitemap: yes\n---\n",
		"---\nsocial: {1: a, \"1\": b}\n---\n",
		"---\na: &a [x, x]\nb: [*a, *a]\n? [k]\n: v\n---\n",
		"---\na: 1\rb: 2\u2028c: [\n---\n",
		"---\ncittà: {è: 1, è: 2}\n---\n",
		"---\nsite: x\n---\n",
		"---\na: 1\n...\n--- x\n---\n",
		"\xFF\xFE-\x00",
		"---\nupdated: 2024-05-01T10:00:00+02:00\n---\n",
	}
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		document, err := Split(testFile, src)
		if err != nil {
			checkErrorPosition(t, src, err)
			return
		}
		if !bytes.HasSuffix(src, document.Body) {
			t.Fatalf("body %q is not the end of the file", document.Body)
		}
		before := src[:len(src)-len(document.Body)]
		if offset := bytes.Count(before, []byte("\n")); offset != document.BodyLineOffset {
			t.Fatalf("line offset %d, but %d lines come before the body", document.BodyLineOffset, offset)
		}
		if document.Variables == nil {
			t.Fatal("variables are nil")
		}
		// Merging never fails and always defines the special keys.
		merged := Merge(NewDefaults(builtInDefaults(), "2006-01-02"), []Document{document}, document)
		for _, name := range []string{layoutKey, titleKey, tagsKey, sitemapKey, excludeFromFleetKey, dateKey} {
			if _, ok := merged.Variables[name]; !ok {
				t.Fatalf("variable %q is missing", name)
			}
		}
	})
}

// checkErrorPosition fails t unless err is a *diag.Error naming testFile
// whose position, when known, is inside src.
func checkErrorPosition(t *testing.T, src []byte, err error) {
	t.Helper()
	var diagErr *diag.Error
	if !errors.As(err, &diagErr) {
		t.Fatalf("error %v is not a *diag.Error", err)
	}
	if diagErr.File != testFile {
		t.Fatalf("error %v names %q", err, diagErr.File)
	}
	if diagErr.Line == 0 {
		return
	}
	lines := bytes.Split(src, []byte("\n"))
	if diagErr.Line < 1 || diagErr.Line > len(lines) {
		t.Fatalf("error %v: line %d is outside the file's %d lines", err, diagErr.Line, len(lines))
	}
	// The caret may sit just after the last character, for an error at the
	// end of a line.
	if line := lines[diagErr.Line-1]; diagErr.Col < 1 || diagErr.Col > len(line)+1 {
		t.Fatalf("error %v: column %d is outside line %q", err, diagErr.Col, line)
	}
}
