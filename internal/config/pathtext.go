package config

import (
	"fmt"
	"unicode"
)

// separatorAdvice ends the messages about characters that stand for a
// directory separator; it is the wording of the template-name and
// passthrough messages too.
const separatorAdvice = `please separate directories with "/"`

// PathCharacterProblem returns why text cannot be a path or a passthrough
// pattern of gondola.yaml, or "" when its characters are fine. The reason
// follows the label and the quoted text in messages, as in
// `input.templates "src\templates" contains a tab; ...`.
//
// "\" is refused on every system, so that a gondola.yaml written on Windows
// does not name a different directory when the site is built on Linux. A
// control character usually comes from the same habit: inside double quotes
// YAML reads "src\templates" as "src", a tab and "emplates", without an
// error.
func PathCharacterProblem(text string) string {
	for _, character := range text {
		if character == '\\' {
			return `contains "\"; ` + separatorAdvice
		}
		if !unicode.IsControl(character) {
			continue
		}
		if escape, name, ok := yamlEscapeOf(character); ok {
			return fmt.Sprintf(`contains %s; inside double quotes YAML reads "%s" as %s; %s`, name, escape, name, separatorAdvice)
		}
		return fmt.Sprintf("contains a control character (%U); please remove it", character)
	}
	return ""
}

// yamlEscapeOf returns the escape that makes character inside double quotes
// in YAML when "\" is followed by a letter or a digit, and the character's
// name in messages. Only these can come from a Windows path; ok is false for
// the other control characters.
func yamlEscapeOf(character rune) (escape, name string, ok bool) {
	switch character {
	case '\x00':
		return `\0`, "a NUL character", true
	case '\a':
		return `\a`, "a bell character", true
	case '\b':
		return `\b`, "a backspace", true
	case '\t':
		return `\t`, "a tab", true
	case '\n':
		return `\n`, "a line break", true
	case '\v':
		return `\v`, "a vertical tab", true
	case '\f':
		return `\f`, "a form feed", true
	case '\r':
		return `\r`, "a carriage return", true
	case '\x1b':
		return `\e`, "an escape character", true
	case '\u0085':
		return `\N`, "a next-line character", true
	}
	return "", "", false
}
