package parser

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/luciaduck/gondola/internal/console"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/squero"
)

// byteOrderMark is the UTF-8 byte order mark, written with byte escapes:
// a raw mark in Go source does not compile.
const byteOrderMark = "\xEF\xBB\xBF"

// testFile is the name the tests give to the page they parse.
const testFile = "blog/post.md"

// testAliasFile defines the aliases that the tests use.
const testAliasFile = ":::\\note . aside +note ?role[note]\n" +
	":::\\warning . div +warning\n"

// mustParseAliasFile parses src or stops the test.
func mustParseAliasFile(t testing.TB, src string) AliasFile {
	t.Helper()
	aliasFile, err := ParseAliasFile([]byte(src))
	if err != nil {
		t.Fatalf("ParseAliasFile: %v", err)
	}
	return aliasFile
}

// squeroOnFile splits page as Gondola does before running Squero, so that the
// tests cover the line offset of the front matter.
func squeroOnFile(t testing.TB, p *Parser, page string) ([]byte, error) {
	t.Helper()
	document, err := frontmatter.Split(testFile, []byte(page))
	if err != nil {
		t.Fatalf("frontmatter.Split: %v", err)
	}
	return p.Squero(testFile, document.Body, document.BodyLineOffset)
}

func TestSqueroErrors(t *testing.T) {
	withAliases := New(mustParseAliasFile(t, testAliasFile))
	withEmptyAliasFile := New(mustParseAliasFile(t, ""))
	withoutAliasFile := New(AliasFile{})
	const badFence = "::: +note ?data-x[abc\n"
	const badFenceMessage = `unterminated value for attribute "data-x": missing ']'`
	tests := []struct {
		name   string
		parser *Parser
		page   string
		want   string
	}{
		{
			name:   "no front matter",
			parser: withAliases,
			page:   "intro\n" + badFence + "text\n:::\n",
			want:   testFile + ":2:11: " + badFenceMessage,
		},
		{
			name:   "front matter shifts the line",
			parser: withAliases,
			page:   "---\ntitle: Post\n---\nintro\n" + badFence + "text\n:::\n",
			want:   testFile + ":5:11: " + badFenceMessage,
		},
		{
			name:   "empty front matter",
			parser: withAliases,
			page:   "---\n---\n" + badFence + ":::\n",
			want:   testFile + ":3:11: " + badFenceMessage,
		},
		{
			name:   "CRLF line endings",
			parser: withAliases,
			page:   "---\r\ntitle: Post\r\n---\r\nintro\r\n::: +note ?data-x[abc\r\ntext\r\n:::\r\n",
			want:   testFile + ":5:11: " + badFenceMessage,
		},
		{
			// Columns do not count the byte order mark (package sourcetext).
			name:   "byte order mark on the error line",
			parser: withAliases,
			page:   byteOrderMark + badFence + ":::\n",
			want:   testFile + ":1:11: " + badFenceMessage,
		},
		{
			name:   "byte order mark and front matter",
			parser: withAliases,
			page:   byteOrderMark + "---\n---\n" + badFence + ":::\n",
			want:   testFile + ":3:11: " + badFenceMessage,
		},
		{
			name:   "accented text before the error counts in bytes",
			parser: withAliases,
			page:   "::: ?title[perché] ?x[\n:::\n",
			want:   testFile + `:1:21: unterminated value for attribute "x": missing ']'`,
		},
		{
			name:   "unclosed fence",
			parser: withAliases,
			page:   "---\ntitle: Post\n---\na\n\n::: +note\ntext\n",
			want:   testFile + `:6:1: unclosed fence: no matching ":::" closing marker`,
		},
		{
			name:   "error inside a nested block",
			parser: withAliases,
			page:   "---\ntitle: Post\n---\n:::: #outer\n::: +inner\n::: ?x[\n:::\n::::\n",
			want:   testFile + `:6:5: unterminated value for attribute "x": missing ']'`,
		},
		{
			name:   "fence inside a code block is skipped",
			parser: withAliases,
			page:   "```\n::: ?x[\n```\n::: ?y[\n:::\n",
			want:   testFile + `:4:5: unterminated value for attribute "y": missing ']'`,
		},
		{
			name:   "HTML comment",
			parser: withAliases,
			page:   "---\n---\n::://a -- b\n",
			want:   testFile + `:3:8: an HTML comment cannot contain '--'`,
		},
		{
			name:   "unknown alias with an alias file",
			parser: withAliases,
			page:   "---\n---\n:::\\missing\ntext\n:::\n",
			want:   testFile + `:3:4: unknown alias "missing"`,
		},
		{
			name:   "unknown alias with an empty alias file",
			parser: withEmptyAliasFile,
			page:   ":::\\note\ntext\n:::\n",
			want:   testFile + `:1:4: unknown alias "note"`,
		},
		{
			name:   "unknown alias without an alias file",
			parser: withoutAliasFile,
			page:   ":::\\note\ntext\n:::\n",
			want:   testFile + `:1:4: unknown alias "note" (there is no alias.squero in the project directory)`,
		},
		{
			// Squero splits lines only at "\n", so a file with lone "\r" line
			// endings is one line for it. SPEC §5.5 accepts this: such files
			// are very rare.
			name:   "lone CR line endings: the fence never closes",
			parser: withAliases,
			page:   "::: +note\rtext\r:::\r",
			want:   testFile + `:1:1: unclosed fence: no matching ":::" closing marker`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := squeroOnFile(t, test.parser, test.page)
			if err == nil {
				t.Fatalf("no error, output %q; want %s", got, test.want)
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

// TestSqueroErrorExcerpt checks the whole path of SPEC §4: the console must
// print the line the error points at, with the caret under the column.
func TestSqueroErrorExcerpt(t *testing.T) {
	tests := []struct {
		name string
		page string
		want string
	}{
		{
			name: "front matter and CRLF",
			page: "---\r\ntitle: Post\r\n---\r\n\r\n::: +note ?data-x[abc\r\n:::\r\n",
			want: "error: blog/post.md:5:11: unterminated value for attribute \"data-x\": missing ']'\n" +
				"    5 | ::: +note ?data-x[abc\n" +
				"      |           ^\n",
		},
		{
			name: "byte order mark on line 1",
			page: byteOrderMark + "::: +note ?data-x[abc\n:::\n",
			want: "error: blog/post.md:1:11: unterminated value for attribute \"data-x\": missing ']'\n" +
				"    1 | ::: +note ?data-x[abc\n" +
				"      |           ^\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := squeroOnFile(t, New(AliasFile{}), test.page)
			if err == nil {
				t.Fatal("no error")
			}
			sources := fstest.MapFS{testFile: {Data: []byte(test.page)}}
			var out, errOut strings.Builder
			printer := console.New(console.Stream{Writer: &out}, console.Stream{Writer: &errOut}, sources)
			printer.Error(err)
			if errOut.String() != test.want {
				t.Errorf("printed:\n got %q\nwant %q", errOut.String(), test.want)
			}
		})
	}
}

// TestSqueroUnknownAliasMessage fails if a new Squero version changes the
// message that squeroUnknownAliasPrefix recognizes.
func TestSqueroUnknownAliasMessage(t *testing.T) {
	_, err := squero.Process([]byte(":::\\note\n:::\n"))
	var parseErr *squero.ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("error %v (%T) is not a *squero.ParseError", err, err)
	}
	if want := squeroUnknownAliasPrefix + `"note"`; parseErr.Message() != want {
		t.Errorf("Squero's message is %q, want %q", parseErr.Message(), want)
	}
}

func TestSqueroOutput(t *testing.T) {
	withAliases := New(mustParseAliasFile(t, testAliasFile))
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty", "", ""},
		{"no fence: unchanged", "# Title\n\nText *here*.\n", "# Title\n\nText *here*.\n"},
		{"no fence: CRLF and no final newline kept", "a\r\nb", "a\r\nb"},
		{"no fence: Jet code kept", "{{ include \"x.md\" }}\n", "{{ include \"x.md\" }}\n"},
		{
			"block element",
			"::: +note #prince\nclick **me**\n:::\n",
			"<div class=\"note\" id=\"prince\">\n\nclick **me**\n\n</div>\n",
		},
		{
			"inline element",
			":::span +x /\nclick **me**\n:::\n",
			"<span class=\"x\">click **me**</span>\n",
		},
		{
			"alias",
			":::\\note #first\ntext\n:::\n",
			"<aside class=\"note\" id=\"first\" role=\"note\">\n\ntext\n\n</aside>\n",
		},
		{
			"code fence with a fence inside",
			"```md\n::: +note\n```\n",
			"```md\n::: +note\n```\n",
		},
		{
			"lone CR line endings: fences after line 1 stay text",
			"intro\r::: +note\rtext\r:::\r",
			"intro\r::: +note\rtext\r:::\r",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := withAliases.Squero(testFile, []byte(test.body), 0)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if string(got) != test.want {
				t.Errorf("got  %q\nwant %q", got, test.want)
			}
		})
	}
}

func TestMarkdown(t *testing.T) {
	p := New(AliasFile{})
	tests := []struct {
		name     string
		markdown string
		want     string
	}{
		{"empty", "", ""},
		{"paragraph", "Hello *world*.\n", "<p>Hello <em>world</em>.</p>\n"},
		{
			"raw HTML block passes through",
			"<div class=\"x\">\n\n**bold**\n\n</div>\n",
			"<div class=\"x\">\n<p><strong>bold</strong></p>\n</div>\n",
		},
		{"inline raw HTML passes through", "a <kbd>Ctrl</kbd> b\n", "<p>a <kbd>Ctrl</kbd> b</p>\n"},
		{"HTML comment passes through", "<!-- note -->\n", "<!-- note -->\n"},
		{"script passes through", "<script>alert(1)</script>\n", "<script>alert(1)</script>\n"},
		{
			"GFM table",
			"| a | b |\n|---|--:|\n| 1 | 2 |\n",
			"<table>\n<thead>\n<tr>\n<th>a</th>\n<th style=\"text-align:right\">b</th>\n</tr>\n</thead>\n" +
				"<tbody>\n<tr>\n<td>1</td>\n<td style=\"text-align:right\">2</td>\n</tr>\n</tbody>\n</table>\n",
		},
		{"GFM strikethrough", "~~old~~\n", "<p><del>old</del></p>\n"},
		{
			"GFM task list",
			"- [x] done\n- [ ] todo\n",
			"<ul>\n<li><input checked=\"\" disabled=\"\" type=\"checkbox\"> done</li>\n" +
				"<li><input disabled=\"\" type=\"checkbox\"> todo</li>\n</ul>\n",
		},
		{"GFM autolink", "See https://example.com.\n", "<p>See <a href=\"https://example.com\">https://example.com</a>.</p>\n"},
		{"heading ID", "## Città di mare\n", "<h2 id=\"citta-di-mare\">Città di mare</h2>\n"},
		{"CRLF", "# Title\r\n\r\ntext\r\n", "<h1 id=\"title\">Title</h1>\n<p>text</p>\n"},
		{
			"Jet braces are plain text",
			"{{ title }}\n",
			"<p>{{ title }}</p>\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := p.Markdown([]byte(test.markdown))
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if string(got) != test.want {
				t.Errorf("got  %q\nwant %q", got, test.want)
			}
		})
	}
}

// TestMarkdownHeadingIDsPerPage checks that the ids of one conversion do not
// leak into the next, although the Goldmark instance is shared.
func TestMarkdownHeadingIDsPerPage(t *testing.T) {
	p := New(AliasFile{})
	const page = "## Notes\n\n## Notes\n"
	const want = "<h2 id=\"notes\">Notes</h2>\n<h2 id=\"notes-1\">Notes</h2>\n"
	for run := 1; run <= 2; run++ {
		got, err := p.Markdown([]byte(page))
		if err != nil {
			t.Fatalf("run %d: unexpected error %v", run, err)
		}
		if string(got) != want {
			t.Errorf("run %d:\n got %q\nwant %q", run, got, want)
		}
	}
}
