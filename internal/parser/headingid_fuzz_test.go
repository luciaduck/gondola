package parser

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/gosimple/slug"
	"github.com/yuin/goldmark/ast"
)

// headingIDPattern is what every automatic heading id looks like: the
// characters that gosimple/slug keeps, without a "-" at either end or two
// in a row, and possibly a "-1", "-2", … suffix, which has the same shape.
var headingIDPattern = regexp.MustCompile(`^[a-z0-9_]+(-[a-z0-9_]+)*$`)

// FuzzHeadingIDs gives arbitrary heading text to the heading ID generator
// and to Goldmark, and checks:
//   - every id has the shape of headingIDPattern;
//   - the id of a heading that Jet escaped is the slug of the text before
//     escaping (SPEC §5.5: `# {{ title }}` with "Fish & chips" gets
//     "fish-and-chips"), or the fallback when that slug is empty;
//   - the same heading three times gets id, id-1, id-2;
//   - Goldmark accepts the heading, and converts it the same way twice.
func FuzzHeadingIDs(f *testing.F) {
	seeds := []string{
		"",
		"Hello World",
		"Città di mare",
		"Perché sì",
		"你好",
		"Q&A",
		"Fish & chips",
		"Fish &amp; chips",
		"AT&T &amp",
		"Caf&#233; &#x3C;b&#x3E;",
		"&#0; &#xD800; &#99999999;",
		"&notanentity; &amp;amp;",
		`<b>"quoted"</b> 'single'`,
		"**Bold** `code` text",
		"snake_case",
		"  - spaced -  ",
		"!!!",
		"Notes 1",
		"_under_",
		"#",
		"# closing ##",
		"tab\tand\vvertical",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, heading string) {
		ids := newHeadingIDs()
		first := string(ids.Generate([]byte(heading), ast.KindHeading))
		checkHeadingID(t, heading, first)
		for _, suffix := range []string{"-1", "-2"} {
			if again := string(ids.Generate([]byte(heading), ast.KindHeading)); again != first+suffix {
				t.Fatalf("heading %q repeated: got %q, want %q", heading, again, first+suffix)
			}
		}

		// Jet escapes with text/template's HTMLEscapeString, which also
		// turns NUL into U+FFFD, so the text after unescaping differs there.
		if !strings.Contains(heading, "\x00") {
			escaped := template.HTMLEscapeString(heading)
			want := slug.Make(heading)
			if want == "" {
				want = fallbackHeadingID
			}
			if got := string(newHeadingIDs().Generate([]byte(escaped), ast.KindHeading)); got != want {
				t.Fatalf("heading %q escaped as %q: got id %q, want %q", heading, escaped, got, want)
			}
		}

		p := New(AliasFile{})
		src := []byte("# " + heading + "\n\n## " + heading + "\n")
		html, err := p.Markdown(src)
		if err != nil {
			t.Fatalf("Markdown(%q): %v", src, err)
		}
		again, err := p.Markdown(src)
		if err != nil || !bytes.Equal(html, again) {
			t.Fatalf("Markdown(%q) differs between two runs:\n%s\n%s", src, html, again)
		}
	})
}

// checkHeadingID fails unless id, made for heading, has the shape of
// headingIDPattern.
func checkHeadingID(t *testing.T, heading, id string) {
	t.Helper()
	if !headingIDPattern.MatchString(id) {
		t.Fatalf("heading %q: id %q does not have the shape of %s", heading, id, headingIDPattern)
	}
}
