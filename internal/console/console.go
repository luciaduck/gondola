// Package console writes everything Gondola prints to the terminal: progress
// messages, colors, and errors with the source line they point at.
package console

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"

	"github.com/luciaduck/gondola/internal/diag"
)

// ANSI escape sequences, written by hand. Only cyan and red are used;
// everything else is printed in the terminal's default color (never black,
// which is invisible on dark terminals).
const (
	ansiCyan  = "\x1b[36m"
	ansiRed   = "\x1b[31m"
	ansiReset = "\x1b[0m"
)

// Messages printed exactly as SPEC §5.14 writes them.
const (
	stepPrefix          = "> "
	takingRideFormat    = "%s is taking a gondola ride"
	craftingFencedBlock = "squero is crafting any fenced block"
	askingSandolo       = "asking sandolo to serve"
	errorPrefix         = "error: "
)

// minLineNumberWidth is the width of the line number in the gutter of an
// error excerpt; "   14 | " in SPEC §4 is a width of 5. Longer numbers widen
// the gutter.
const minLineNumberWidth = 5

// Stream is an output stream and whether ANSI colors may be written to it.
// Use ColorFor to decide Color for a real file.
type Stream struct {
	Writer io.Writer
	Color  bool
}

// Printer writes Gondola's messages to standard output and its errors to
// standard error.
//
// Write errors are ignored on purpose: there is nowhere left to report a
// failure to write to the terminal.
type Printer struct {
	out     Stream
	errOut  Stream
	sources fs.FS
}

// New returns a Printer that writes messages to out and errors to errOut.
// sources is the project root; error excerpts are read from it using the
// file name of the diag.Error. An fs.FS is used instead of the disk so that
// tests can give an in-memory file system (testing/fstest.MapFS).
func New(out, errOut Stream, sources fs.FS) *Printer {
	return &Printer{out: out, errOut: errOut, sources: sources}
}

// Print writes text to standard output as it is, in the default color.
func (p *Printer) Print(text string) {
	io.WriteString(p.out.Writer, text)
}

// PrintStderr writes text to standard error as it is, in the default color.
func (p *Printer) PrintStderr(text string) {
	io.WriteString(p.errOut.Writer, text)
}

// TakingRide prints the first line of every build. directoryName is the base
// name of the working directory.
func (p *Printer) TakingRide(directoryName string) {
	p.step(fmt.Sprintf(takingRideFormat, directoryName))
}

// CraftingFencedBlocks prints the line that says Squero is on.
func (p *Printer) CraftingFencedBlocks() {
	p.step(craftingFencedBlock)
}

// AskingSandolo prints the line that starts the development server.
func (p *Printer) AskingSandolo() {
	p.step(askingSandolo)
}

// step prints a line that begins with "> ", all of it in cyan.
func (p *Printer) step(text string) {
	io.WriteString(p.out.Writer, paint(p.out.Color, ansiCyan, stepPrefix+text)+"\n")
}

// Error prints err to standard error in red as "error: <message>". When err
// contains a diag.Error that names a file and a line, the source line follows,
// with a caret under the column if the column is known:
//
//	error: blog/first-post.md:14:3: unterminated value for attribute "data-x": missing ']'
//	   14 | ::: +note ?data-x[abc
//	      |   ^
//
// If the source line cannot be read, only the first line is printed.
func (p *Printer) Error(err error) {
	lines := []string{errorPrefix + err.Error()}
	var diagErr *diag.Error
	// errors.As finds the diag.Error even when err wraps it.
	if errors.As(err, &diagErr) {
		lines = append(lines, p.excerpt(diagErr)...)
	}
	p.errorLines(lines)
}

// BuildFailed prints the last line of a failed build. limitReached says that
// the build stopped collecting errors at its limit, so errorCount is a lower
// bound.
func (p *Printer) BuildFailed(errorCount int, limitReached bool) {
	var line string
	switch {
	case limitReached:
		line = fmt.Sprintf("build failed: stopped after %d errors", errorCount)
	case errorCount == 1:
		line = "build failed: 1 error"
	default:
		line = fmt.Sprintf("build failed: %d errors", errorCount)
	}
	p.errorLines([]string{line})
}

// errorLines writes lines to standard error, each painted red on its own so
// that the color never leaks past the end of a line.
func (p *Printer) errorLines(lines []string) {
	var text strings.Builder
	for _, line := range lines {
		text.WriteString(paint(p.errOut.Color, ansiRed, line))
		text.WriteString("\n")
	}
	io.WriteString(p.errOut.Writer, text.String())
}

// excerpt returns the gutter lines that show where diagErr points, or nothing
// if the position is incomplete or the line cannot be read.
func (p *Printer) excerpt(diagErr *diag.Error) []string {
	if diagErr.File == "" || diagErr.Line <= 0 {
		return nil
	}
	sourceLine, ok := p.readLine(diagErr.File, diagErr.Line)
	if !ok {
		return nil
	}
	lineNumber := strconv.Itoa(diagErr.Line)
	width := max(minLineNumberWidth, len(lineNumber))
	lines := []string{fmt.Sprintf("%*s | %s", width, lineNumber, sourceLine)}
	if diagErr.Col > 0 {
		lines = append(lines, fmt.Sprintf("%*s | %s^", width, "", caretPadding(sourceLine, diagErr.Col)))
	}
	return lines
}

// readLine returns line number lineNumber (starting at 1) of the file, without
// its line ending.
func (p *Printer) readLine(file string, lineNumber int) (string, bool) {
	if p.sources == nil {
		return "", false
	}
	content, err := fs.ReadFile(p.sources, file)
	if err != nil {
		return "", false
	}
	scanner := bufio.NewScanner(bytes.NewReader(content))
	// A line can be longer than bufio.Scanner's default limit of 64 KiB;
	// the whole file is already in memory, so allow up to its size.
	scanner.Buffer(nil, len(content)+1)
	for current := 1; scanner.Scan(); current++ {
		if current == lineNumber {
			return strings.TrimSuffix(scanner.Text(), "\r"), true
		}
	}
	return "", false
}

// caretPadding returns the text to put before the caret so that it sits under
// byte column col (starting at 1) of sourceLine. Tabs are copied so that the
// caret moves as far as the tab does; any other character becomes one space,
// whatever its length in bytes. A column past the end of the line continues
// with spaces.
func caretPadding(sourceLine string, col int) string {
	before := sourceLine
	if col-1 < len(before) {
		before = before[:col-1]
	}
	var padding strings.Builder
	for _, r := range before {
		if r == '\t' {
			padding.WriteRune('\t')
		} else {
			padding.WriteRune(' ')
		}
	}
	padding.WriteString(strings.Repeat(" ", col-1-len(before)))
	return padding.String()
}

// paint wraps text in color when enabled is true.
func paint(enabled bool, color, text string) string {
	if !enabled {
		return text
	}
	return color + text + ansiReset
}
