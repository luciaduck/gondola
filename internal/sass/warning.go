package sass

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/bep/godartsass/v2"
)

// loggedPositionPattern matches the start of a log event's message when
// Dart Sass gave a position: godartsass writes "<url>:<line>:<col>: <text>",
// with the URL unescaped and the line and column counted from 0. @warn
// events have no position (Dart Sass gives a stack trace instead, which
// godartsass drops), so their message is the text alone.
var loggedPositionPattern = regexp.MustCompile(`(?s)^(file://.*?):(\d+):(\d+): (.*)$`)

// printWarnings prints the events of one compilation that this build has
// not printed yet. entryFilePath and entryText are the entry point that
// was compiled, whose text Dart Sass got from Gondola.
func (c *Compiler) printWarnings(events []godartsass.LogEvent, entryFilePath, entryText string) {
	if c.printWarning == nil {
		return
	}
	for _, event := range events {
		text := c.formatEvent(event, entryFilePath, entryText)
		if c.printedWarnings[text] {
			continue
		}
		c.printedWarnings[text] = true
		c.printWarning(text)
	}
}

// formatEvent writes a log event as "warning: file:line:col: message", or
// "debug: …" for @debug, with the position counted as diag counts it.
func (c *Compiler) formatEvent(event godartsass.LogEvent, entryFilePath, entryText string) string {
	label := "warning"
	if event.Type == godartsass.LogEventTypeDebug {
		label = "debug"
	}
	position, message := c.eventPosition(event.Message, entryFilePath, entryText)
	if position == "" {
		return fmt.Sprintf("%s: %s", label, message)
	}
	return fmt.Sprintf("%s: %s: %s", label, position, message)
}

// eventPosition splits a log event's message into the position, written as
// "file:line:col", and the text. The position is empty when the message has
// none.
func (c *Compiler) eventPosition(eventMessage, entryFilePath, entryText string) (position, message string) {
	match := loggedPositionPattern.FindStringSubmatch(eventMessage)
	if match == nil {
		return "", eventMessage
	}
	lineIndex, lineErr := strconv.Atoi(match[2])
	utf16Column, colErr := strconv.Atoi(match[3])
	if lineErr != nil || colErr != nil {
		return "", eventMessage
	}
	// godartsass has already unescaped the URL, so only "file://" is cut.
	// It unescapes as a query string, so a "+" in a file name has become a
	// space; such a file is not found below and its column is left out.
	filePath := pathFromURLPath(match[1][len("file://"):])
	position = fmt.Sprintf("%s:%d", c.displayedPath(filePath), lineIndex+1)

	text, ok := c.fileText(filePath, entryFilePath, entryText)
	if !ok {
		return position, match[4]
	}
	if col, ok := lineColumn(text, lineIndex, utf16Column); ok {
		position += ":" + strconv.Itoa(col)
	}
	return position, match[4]
}
