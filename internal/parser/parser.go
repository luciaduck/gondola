// Package parser turns the body of a Markdown page into HTML in two steps:
// Squero, which rewrites ":::" fenced blocks into HTML, and Goldmark, which
// converts the Markdown. The two steps are separate functions because the
// first Jet round runs between them (SPEC §3.2).
//
// The parser knows nothing about Jet, layouts or pages on disk: it receives
// text and returns text or *diag.Error values. The only file it reads is
// alias.squero (LoadAliasFile), once per build.
package parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/squero"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// squeroUnknownAliasPrefix starts Squero's message for a fence that names an
// alias that is not defined, as in `unknown alias "note"`. Squero has no
// error kinds, so the message is the only way to recognize it;
// TestSqueroUnknownAliasMessage fails if a new Squero version changes it.
const squeroUnknownAliasPrefix = "unknown alias "

// noAliasFileHint is added to the unknown-alias message when the project has
// no alias.squero at all, which is the likeliest reason for the error.
const noAliasFileHint = " (there is no " + AliasFileName + " in the project directory)"

// Parser runs Squero and Goldmark. Build one with New, once per build, and
// share it: the engine's template loader uses the same Parser for the
// Markdown fragments that pages include (SPEC §5.11).
type Parser struct {
	fencedBlocks   squero.Processor
	aliasFileFound bool
	markdown       goldmark.Markdown
}

// New returns a Parser that expands the aliases of aliasFile. Pass the zero
// AliasFile when fenced blocks are off or the project has no alias.squero.
func New(aliasFile AliasFile) *Parser {
	return &Parser{
		fencedBlocks:   squero.Processor{Aliases: aliasFile.Aliases},
		aliasFileFound: aliasFile.Found,
		markdown: goldmark.New(
			// GFM and automatic heading IDs keep CommonMark's rules for HTML
			// blocks, which Squero relies on. Do not add extensions that
			// change them.
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(goldmarkparser.WithAutoHeadingID()),
			// Without WithUnsafe, Goldmark drops the raw HTML that Squero
			// writes (and any HTML the author writes).
			goldmark.WithRendererOptions(html.WithUnsafe()),
		),
	}
}

// Squero rewrites the fenced blocks of body, the body of the page or
// fragment file, into HTML. Call it only when fenced-blocks is on.
//
// file is the path printed in errors, relative to the project directory and
// written with "/". body must not start with a byte order mark (front matter
// removes it), and bodyLineOffset is the number of lines of the file before
// body (frontmatter.Document.BodyLineOffset, 0 for a fragment). An error in
// a fence is a *diag.Error whose line counts the whole file.
func (p *Parser) Squero(file string, body []byte, bodyLineOffset int) ([]byte, error) {
	processed, err := p.fencedBlocks.Process(body)
	if err == nil {
		return processed, nil
	}
	positioned, ok := p.fenceError(err)
	if !ok {
		return nil, fmt.Errorf("processing the fenced blocks of %s: %w", file, err)
	}
	// ShiftLines and WithFile must come before any wrapping (package diag).
	return nil, diag.WithFile(diag.ShiftLines(positioned, bodyLineOffset), file)
}

// fenceError turns a Squero error in a page into a *diag.Error without a
// file name, with Squero's message unchanged except for the unknown-alias
// hint. ok is false for an error that is not a *squero.ParseError.
func (p *Parser) fenceError(err error) (positioned error, ok bool) {
	diagErr, ok := fromSqueroError(err)
	if !ok {
		return nil, false
	}
	if !p.aliasFileFound && strings.HasPrefix(diagErr.Msg, squeroUnknownAliasPrefix) {
		diagErr.Msg += noAliasFileHint
	}
	return diagErr, true
}

// fromSqueroError converts a *squero.ParseError into a *diag.Error at the
// same position. Squero, like package diag, counts lines separated by "\n"
// and columns in bytes, so the position needs no conversion. Squero uses
// line 0 for an unknown position, which diag treats the same way.
func fromSqueroError(err error) (*diag.Error, bool) {
	// errors.As finds a *squero.ParseError even inside a wrapping error and
	// stores it in parseErr.
	var parseErr *squero.ParseError
	if !errors.As(err, &parseErr) {
		return nil, false
	}
	return &diag.Error{Line: parseErr.Line(), Col: parseErr.Column(), Msg: parseErr.Message()}, true
}

// Markdown converts src, a Markdown text whose fenced blocks and Jet code
// have already been processed, into HTML.
func (p *Parser) Markdown(src []byte) ([]byte, error) {
	var converted bytes.Buffer
	// Each conversion gets its own heading IDs, so that the first "## Notes"
	// of every page is "notes", not "notes-1".
	parseContext := goldmarkparser.NewContext(goldmarkparser.WithIDs(newHeadingIDs()))
	if err := p.markdown.Convert(src, &converted, goldmarkparser.WithContext(parseContext)); err != nil {
		return nil, fmt.Errorf("converting Markdown to HTML: %w", err)
	}
	return converted.Bytes(), nil
}
