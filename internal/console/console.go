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
	"time"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
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
	stepPrefix            = "> "
	takingRideFormat      = "%s is taking a gondola ride"
	craftingFencedBlock   = "squero is crafting any fenced block"
	askingGondolierFormat = "asking a gondolier to take %s to port %d"
	// The server's lines are written in SPEC §5.16, and are in the default
	// color. The first comes from sandolo, the library that internal/server
	// was copied from (M12, Q7).
	serverReadyFormat = "Server ready at %s"
	serverHelp        = "Type g and Enter to rebuild, or q and Enter to close."
	errorPrefix       = "error: "
	// gondolaPrefix begins the lines about the files of a build. The
	// backslash is part of the text.
	gondolaPrefix   = `\gondola/`
	shapingFormat   = "Shaping ./%s from ./%s"
	shippingFormat  = "Shipping %s to %s"
	executingFormat = "Executing %s in %s"
	builtFormat     = "Built %d %s and Copied %d in %.2f seconds (%s)"
	builtEachFormat = "%.2fms each, %s"
	// The sitemap line has a singular and a plural form, and another text
	// when there is no robots.txt (M10, Q2).
	sitemapAllowedSingular  = "(sitemap) 1 page was allowed by robots.txt"
	sitemapAllowedPlural    = "(sitemap) %d pages were allowed by robots.txt"
	sitemapNoRobotsSingular = "(sitemap) 1 page was listed (there is no robots.txt)"
	sitemapNoRobotsPlural   = "(sitemap) %d pages were listed (there is no robots.txt)"
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

// AskingGondolier prints the line that starts the development server of
// gondola serve. outputDir is the output directory as configured, written
// with "/".
func (p *Printer) AskingGondolier(outputDir string, port int) {
	p.step(fmt.Sprintf(askingGondolierFormat, outputDir, port))
}

// ServerReady prints the address where gondola serve is listening, such as
// http://127.0.0.1:1094.
func (p *Printer) ServerReady(siteURL string) {
	io.WriteString(p.out.Writer, fmt.Sprintf(serverReadyFormat, siteURL)+"\n")
}

// ServerHelp prints the commands that gondola serve reads from standard
// input.
func (p *Printer) ServerHelp() {
	io.WriteString(p.out.Writer, serverHelp+"\n")
}

// Shaping prints the line for a page that was just written. outputFilePath
// is the page's file in the temp output and sourcePath its source, both
// relative to the project root and written with "/":
//
//	\gondola/ Shaping ./_site.temp/404.html from ./404.html (jet)
//
// engineName is the template engine that ran on the page, or "" for a page
// with layout: nil, whose line has no parentheses.
func (p *Printer) Shaping(outputFilePath, sourcePath, engineName string) {
	text := fmt.Sprintf(shapingFormat, outputFilePath, sourcePath)
	if engineName != "" {
		text += " (" + engineName + ")"
	}
	p.gondolaLine(text)
}

// Shipping prints the line that publishes the build: tempDir is renamed to
// outputDir. Both are relative to the project root, written with "/".
func (p *Printer) Shipping(tempDir, outputDir string) {
	p.gondolaLine(fmt.Sprintf(shippingFormat, tempDir, outputDir))
}

// Executing prints the line written before a plugin runs. pluginPath is
// the entry of config.plugin and tempDir the temp output, both relative to
// the project root and written with "/":
//
//	\gondola/ Executing scripts/minify.sh in _site.temp
func (p *Printer) Executing(pluginPath, tempDir string) {
	p.gondolaLine(fmt.Sprintf(executingFormat, pluginPath, tempDir))
}

// Built prints the last line of a successful build:
//
//	\gondola/ Built 4 files and Copied 3 in 0.04 seconds (5.71ms each, v0.0.1)
//
// pageCount is the number of pages written and copiedCount the number of
// passthrough files; Sass files count in neither. elapsed is the time of the
// whole build, and "each" is elapsed divided by both counts together. It is
// left out when both are 0, which would divide by zero. "file" is singular
// for exactly one page; "Copied" has no noun, so it needs no plural.
func (p *Printer) Built(pageCount, copiedCount int, elapsed time.Duration, gondolaVersion string) {
	noun := "files"
	if pageCount == 1 {
		noun = "file"
	}
	details := gondolaVersion
	if fileCount := pageCount + copiedCount; fileCount > 0 {
		millisecondsEach := float64(elapsed) / float64(time.Millisecond) / float64(fileCount)
		details = fmt.Sprintf(builtEachFormat, millisecondsEach, gondolaVersion)
	}
	p.gondolaLine(fmt.Sprintf(builtFormat, pageCount, noun, copiedCount, elapsed.Seconds(), details))
}

// Sitemap prints the line written after sitemap.xml, in the default color:
//
//	(sitemap) 37 pages were allowed by robots.txt
//
// listedCount is the number of URLs in the sitemap, so pages with sitemap:
// false are not counted. robotsFound is false when the output has no
// robots.txt, and the line says so instead. "page" is singular for exactly
// one, as "file" is on the last line.
func (p *Printer) Sitemap(listedCount int, robotsFound bool) {
	var text string
	switch {
	case robotsFound && listedCount == 1:
		text = sitemapAllowedSingular
	case robotsFound:
		text = fmt.Sprintf(sitemapAllowedPlural, listedCount)
	case listedCount == 1:
		text = sitemapNoRobotsSingular
	default:
		text = fmt.Sprintf(sitemapNoRobotsPlural, listedCount)
	}
	io.WriteString(p.out.Writer, text+"\n")
}

// Warning prints a warning that does not stop the build, such as one from
// Dart Sass, to standard error in the default color. text is one message
// without a final line break.
func (p *Printer) Warning(text string) {
	io.WriteString(p.errOut.Writer, text+"\n")
}

// gondolaLine prints a line that begins with "\gondola/". Only the prefix is
// cyan; the rest is in the default color.
func (p *Printer) gondolaLine(text string) {
	io.WriteString(p.out.Writer, paint(p.out.Color, ansiCyan, gondolaPrefix)+" "+text+"\n")
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

// Failure prints the errors of a failed build in the order they were found,
// then the line that counts them.
func (p *Printer) Failure(errs []error, limitReached bool) {
	for _, err := range errs {
		p.Error(err)
	}
	p.BuildFailed(len(errs), limitReached)
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
	// Columns do not count a byte order mark, so it is not printed either.
	content = sourcetext.TrimByteOrderMark(content)
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
