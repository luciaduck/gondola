package console

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/luciaduck/gondola/internal/diag"
)

// newTestPrinter returns a Printer that writes into two buffers.
func newTestPrinter(color bool, sources fstest.MapFS) (*Printer, *strings.Builder, *strings.Builder) {
	var out, errOut strings.Builder
	printer := New(Stream{Writer: &out, Color: color}, Stream{Writer: &errOut, Color: color}, sources)
	return printer, &out, &errOut
}

// manyLines returns a file of count lines, each holding its own number.
func manyLines(count int) []byte {
	var text strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&text, "line %d\n", i)
	}
	return []byte(text.String())
}

func TestError(t *testing.T) {
	sources := fstest.MapFS{
		"blog/first-post.md": {Data: []byte(strings.Repeat("\n", 13) + "::: +note ?data-x[abc\n:::\n")},
		"crlf.md":            {Data: []byte("---\r\ntitle: x\r\n---\r\n::: +note ?data-x[abc\r\n")},
		"tabs.md":            {Data: []byte("\t\t::: +x\n")},
		"accents.md":         {Data: []byte("perché è ?x[\n")},
		"no-newline.md":      {Data: []byte("first\nlast")},
		"long.md":            {Data: manyLines(123456)},
		"bom.md":             {Data: []byte("\xEF\xBB\xBF::: +note ?x[\nsecond\n")},
	}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "example of SPEC §4",
			err: &diag.Error{File: "blog/first-post.md", Line: 14, Col: 3,
				Msg: `unterminated value for attribute "data-x": missing ']'`},
			want: "error: blog/first-post.md:14:3: unterminated value for attribute \"data-x\": missing ']'\n" +
				"   14 | ::: +note ?data-x[abc\n" +
				"      |   ^\n",
		},
		{
			name: "column 1",
			err:  &diag.Error{File: "blog/first-post.md", Line: 14, Col: 1, Msg: "bad"},
			want: "error: blog/first-post.md:14:1: bad\n" +
				"   14 | ::: +note ?data-x[abc\n" +
				"      | ^\n",
		},
		{
			name: "column just past the end of the line",
			err:  &diag.Error{File: "blog/first-post.md", Line: 14, Col: 22, Msg: "missing ']'"},
			want: "error: blog/first-post.md:14:22: missing ']'\n" +
				"   14 | ::: +note ?data-x[abc\n" +
				"      |                      ^\n",
		},
		{
			name: "column far past the end of the line",
			err:  &diag.Error{File: "no-newline.md", Line: 1, Col: 8, Msg: "bad"},
			want: "error: no-newline.md:1:8: bad\n" +
				"    1 | first\n" +
				"      |        ^\n",
		},
		{
			name: "no column: no caret",
			err:  &diag.Error{File: "blog/first-post.md", Line: 14, Msg: "bad"},
			want: "error: blog/first-post.md:14: bad\n" +
				"   14 | ::: +note ?data-x[abc\n",
		},
		{
			name: "CRLF line endings are not printed",
			err:  &diag.Error{File: "crlf.md", Line: 4, Col: 11, Msg: "bad"},
			want: "error: crlf.md:4:11: bad\n" +
				"    4 | ::: +note ?data-x[abc\n" +
				"      |           ^\n",
		},
		{
			name: "tabs are copied before the caret",
			err:  &diag.Error{File: "tabs.md", Line: 1, Col: 7, Msg: "bad"},
			want: "error: tabs.md:1:7: bad\n" +
				"    1 | \t\t::: +x\n" +
				"      | \t\t    ^\n",
		},
		{
			// Col counts bytes; "é" and "è" are two bytes each but take one
			// place on screen.
			name: "multi-byte characters take one place",
			err:  &diag.Error{File: "accents.md", Line: 1, Col: 13, Msg: "bad"},
			want: "error: accents.md:1:13: bad\n" +
				"    1 | perché è ?x[\n" +
				"      |           ^\n",
		},
		{
			// Columns do not count a byte order mark (package sourcetext),
			// and the mark is not printed.
			name: "byte order mark on line 1 is not printed",
			err:  &diag.Error{File: "bom.md", Line: 1, Col: 11, Msg: "bad"},
			want: "error: bom.md:1:11: bad\n" +
				"    1 | ::: +note ?x[\n" +
				"      |           ^\n",
		},
		{
			name: "byte order mark does not change line 2",
			err:  &diag.Error{File: "bom.md", Line: 2, Col: 2, Msg: "bad"},
			want: "error: bom.md:2:2: bad\n" +
				"    2 | second\n" +
				"      |  ^\n",
		},
		{
			name: "last line without a final newline",
			err:  &diag.Error{File: "no-newline.md", Line: 2, Col: 2, Msg: "bad"},
			want: "error: no-newline.md:2:2: bad\n" +
				"    2 | last\n" +
				"      |  ^\n",
		},
		{
			name: "line numbers longer than the gutter widen it",
			err:  &diag.Error{File: "long.md", Line: 123456, Col: 6, Msg: "bad"},
			want: "error: long.md:123456:6: bad\n" +
				"123456 | line 123456\n" +
				"       |      ^\n",
		},
		{
			name: "wrapped diag error keeps the wrapping message",
			err: fmt.Errorf("rendering blog/first-post.md: %w",
				&diag.Error{File: "blog/first-post.md", Line: 14, Col: 3, Msg: "bad"}),
			want: "error: rendering blog/first-post.md: blog/first-post.md:14:3: bad\n" +
				"   14 | ::: +note ?data-x[abc\n" +
				"      |   ^\n",
		},
		{
			name: "line past the end of the file: no excerpt",
			err:  &diag.Error{File: "no-newline.md", Line: 3, Col: 1, Msg: "bad"},
			want: "error: no-newline.md:3:1: bad\n",
		},
		{
			name: "missing file: no excerpt",
			err:  &diag.Error{File: "gone.md", Line: 1, Col: 1, Msg: "bad"},
			want: "error: gone.md:1:1: bad\n",
		},
		{
			name: "no line: no excerpt",
			err:  &diag.Error{File: "no-newline.md", Msg: "bad"},
			want: "error: no-newline.md: bad\n",
		},
		{
			name: "no file: no excerpt",
			err:  &diag.Error{Line: 1, Col: 1, Msg: "bad"},
			want: "error: 1:1: bad\n",
		},
		{
			name: "plain error",
			err:  errors.New("reading gondola.yaml: permission denied"),
			want: "error: reading gondola.yaml: permission denied\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			printer, out, errOut := newTestPrinter(false, sources)
			printer.Error(test.err)
			if errOut.String() != test.want {
				t.Errorf("standard error:\n got %q\nwant %q", errOut.String(), test.want)
			}
			if out.Len() != 0 {
				t.Errorf("standard output is not empty: %q", out.String())
			}
		})
	}
}

func TestErrorWithoutSources(t *testing.T) {
	printer, _, errOut := newTestPrinter(false, nil)
	printer.Error(&diag.Error{File: "a.md", Line: 1, Col: 1, Msg: "bad"})
	if want := "error: a.md:1:1: bad\n"; errOut.String() != want {
		t.Errorf("got %q, want %q", errOut.String(), want)
	}
}

func TestErrorInColor(t *testing.T) {
	sources := fstest.MapFS{"a.md": {Data: []byte("abc\n")}}
	printer, _, errOut := newTestPrinter(true, sources)
	printer.Error(&diag.Error{File: "a.md", Line: 1, Col: 2, Msg: "bad"})
	want := "\x1b[31merror: a.md:1:2: bad\x1b[0m\n" +
		"\x1b[31m    1 | abc\x1b[0m\n" +
		"\x1b[31m      |  ^\x1b[0m\n"
	if errOut.String() != want {
		t.Errorf("got %q, want %q", errOut.String(), want)
	}
}

func TestBuildFailed(t *testing.T) {
	tests := []struct {
		name         string
		errorCount   int
		limitReached bool
		color        bool
		want         string
	}{
		{"one error", 1, false, false, "build failed: 1 error\n"},
		{"several errors", 3, false, false, "build failed: 3 errors\n"},
		{"limit reached", 20, true, false, "build failed: stopped after 20 errors\n"},
		{"in color", 2, false, true, "\x1b[31mbuild failed: 2 errors\x1b[0m\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			printer, out, errOut := newTestPrinter(test.color, nil)
			printer.BuildFailed(test.errorCount, test.limitReached)
			if errOut.String() != test.want {
				t.Errorf("got %q, want %q", errOut.String(), test.want)
			}
			if out.Len() != 0 {
				t.Errorf("standard output is not empty: %q", out.String())
			}
		})
	}
}

func TestSteps(t *testing.T) {
	tests := []struct {
		name  string
		print func(*Printer)
		color bool
		want  string
	}{
		{"taking a ride", func(p *Printer) { p.TakingRide("my-site") }, false, "> my-site is taking a gondola ride\n"},
		{"crafting fenced blocks", (*Printer).CraftingFencedBlocks, false, "> squero is crafting any fenced block\n"},
		{"asking sandolo", (*Printer).AskingSandolo, false, "> asking sandolo to serve\n"},
		{"the whole line is cyan", func(p *Printer) { p.TakingRide("my-site") }, true, "\x1b[36m> my-site is taking a gondola ride\x1b[0m\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			printer, out, errOut := newTestPrinter(test.color, nil)
			test.print(printer)
			if out.String() != test.want {
				t.Errorf("got %q, want %q", out.String(), test.want)
			}
			if errOut.Len() != 0 {
				t.Errorf("standard error is not empty: %q", errOut.String())
			}
		})
	}
}

func TestBuildLines(t *testing.T) {
	built := func(pages, copied int, elapsed time.Duration) func(*Printer) {
		return func(p *Printer) { p.Built(pages, copied, elapsed, "v0.0.1") }
	}
	tests := []struct {
		name  string
		print func(*Printer)
		color bool
		want  string
	}{
		{"shaping with an engine, example of SPEC §5.14",
			func(p *Printer) { p.Shaping("_site.temp/404.html", "404.html", "jet") }, false,
			"\\gondola/ Shaping ./_site.temp/404.html from ./404.html (jet)\n"},
		{"shaping a page in a directory, example of SPEC §5.14",
			func(p *Printer) { p.Shaping("_site.temp/pag/geo/orders.html", "pag/geo/orders.htm", "jet") }, false,
			"\\gondola/ Shaping ./_site.temp/pag/geo/orders.html from ./pag/geo/orders.htm (jet)\n"},
		{"shaping a page with layout: nil has no parentheses",
			func(p *Printer) { p.Shaping("_site.temp/raw.html", "src/raw.md", "") }, false,
			"\\gondola/ Shaping ./_site.temp/raw.html from ./src/raw.md\n"},
		{"shipping, example of SPEC §5.14",
			func(p *Printer) { p.Shipping("_site.temp", "_site") }, false,
			"\\gondola/ Shipping _site.temp to _site\n"},
		{"executing, example of SPEC §5.13",
			func(p *Printer) { p.Executing("scripts/minify.sh", "_site.temp") }, false,
			"\\gondola/ Executing scripts/minify.sh in _site.temp\n"},
		{"executing, only the prefix is cyan",
			func(p *Printer) { p.Executing("minify", "public.temp") }, true,
			"\x1b[36m\\gondola/\x1b[0m Executing minify in public.temp\n"},
		{"built, example of SPEC §5.14",
			built(4, 3, 40*time.Millisecond), false,
			"\\gondola/ Built 4 files and Copied 3 in 0.04 seconds (5.71ms each, v0.0.1)\n"},
		{"one page is one file",
			built(1, 3, 40*time.Millisecond), false,
			"\\gondola/ Built 1 file and Copied 3 in 0.04 seconds (10.00ms each, v0.0.1)\n"},
		{"one page and one copied file",
			built(1, 1, 3*time.Second), false,
			"\\gondola/ Built 1 file and Copied 1 in 3.00 seconds (1500.00ms each, v0.0.1)\n"},
		{"no pages is plural",
			built(0, 2, 10*time.Millisecond), false,
			"\\gondola/ Built 0 files and Copied 2 in 0.01 seconds (5.00ms each, v0.0.1)\n"},
		{"no files at all leaves out each",
			built(0, 0, 10*time.Millisecond), false,
			"\\gondola/ Built 0 files and Copied 0 in 0.01 seconds (v0.0.1)\n"},
		{"rounding",
			built(2, 1, 4567*time.Microsecond), false,
			"\\gondola/ Built 2 files and Copied 1 in 0.00 seconds (1.52ms each, v0.0.1)\n"},
		{"only the prefix is cyan",
			func(p *Printer) { p.Shipping("_site.temp", "_site") }, true,
			"\x1b[36m\\gondola/\x1b[0m Shipping _site.temp to _site\n"},
		{"sitemap, example of SPEC §5.14",
			func(p *Printer) { p.Sitemap(37, true) }, false,
			"(sitemap) 37 pages were allowed by robots.txt\n"},
		{"sitemap with one page",
			func(p *Printer) { p.Sitemap(1, true) }, false,
			"(sitemap) 1 page was allowed by robots.txt\n"},
		{"sitemap with no page is plural",
			func(p *Printer) { p.Sitemap(0, true) }, false,
			"(sitemap) 0 pages were allowed by robots.txt\n"},
		{"sitemap without robots.txt",
			func(p *Printer) { p.Sitemap(12, false) }, false,
			"(sitemap) 12 pages were listed (there is no robots.txt)\n"},
		{"sitemap without robots.txt, one page",
			func(p *Printer) { p.Sitemap(1, false) }, false,
			"(sitemap) 1 page was listed (there is no robots.txt)\n"},
		{"sitemap without robots.txt, no page",
			func(p *Printer) { p.Sitemap(0, false) }, false,
			"(sitemap) 0 pages were listed (there is no robots.txt)\n"},
		{"the sitemap line is never colored",
			func(p *Printer) { p.Sitemap(37, true) }, true,
			"(sitemap) 37 pages were allowed by robots.txt\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			printer, out, errOut := newTestPrinter(test.color, nil)
			test.print(printer)
			if out.String() != test.want {
				t.Errorf("got %q, want %q", out.String(), test.want)
			}
			if errOut.Len() != 0 {
				t.Errorf("standard error is not empty: %q", errOut.String())
			}
		})
	}
}

func TestWarningIsNotColored(t *testing.T) {
	printer, out, errOut := newTestPrinter(true, nil)
	printer.Warning("warning: style/main.scss:3:5: this is deprecated")
	if want := "warning: style/main.scss:3:5: this is deprecated\n"; errOut.String() != want {
		t.Errorf("standard error = %q, want %q", errOut.String(), want)
	}
	if out.Len() != 0 {
		t.Errorf("standard output is not empty: %q", out.String())
	}
}

func TestPrintIsNeverColored(t *testing.T) {
	printer, out, errOut := newTestPrinter(true, nil)
	printer.Print("to stdout\n")
	printer.PrintStderr("to stderr\n")
	if out.String() != "to stdout\n" {
		t.Errorf("standard output = %q", out.String())
	}
	if errOut.String() != "to stderr\n" {
		t.Errorf("standard error = %q", errOut.String())
	}
}

func TestUseColor(t *testing.T) {
	tests := []struct {
		name       string
		isTerminal bool
		noColor    string
		want       bool
	}{
		{"terminal, NO_COLOR unset or empty", true, "", true},
		{"terminal, NO_COLOR set", true, "1", false},
		{"terminal, NO_COLOR set to any text", true, "false", false},
		{"not a terminal", false, "", false},
		{"not a terminal, NO_COLOR set", false, "1", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := useColor(test.isTerminal, test.noColor); got != test.want {
				t.Errorf("useColor(%v, %q) = %v, want %v", test.isTerminal, test.noColor, got, test.want)
			}
		})
	}
}

func TestColorForRegularFile(t *testing.T) {
	file, err := os.Create(filepath.Join(t.TempDir(), "output.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if ColorFor(file, "") {
		t.Errorf("ColorFor(regular file) = true, want false")
	}
}

func TestColorForPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if ColorFor(writer, "") {
		t.Errorf("ColorFor(pipe) = true, want false")
	}
}
