// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package naming turns the keys that users write in YAML (kebab-case or
// snake_case) into the camelCase names that templates use, and checks that
// the result can be used as a Jet variable. This is the only place where the
// conversion happens.
package naming

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// wordSeparators are the characters that split a key into words.
const wordSeparators = "-_"

// jetKeywords are the words that Jet's lexer treats as keywords, so a
// variable with one of these names could never be read in a template. Copied
// from the key map in lex.go of github.com/CloudyKit/jet/v6 v6.2.0 (the
// same in v6.3.3), plus "true" and "false", which the lexer reads as
// booleans. Check this list when Jet is upgraded.
var jetKeywords = map[string]bool{
	"extends": true, "import": true, "include": true, "block": true,
	"end": true, "yield": true, "content": true, "if": true, "else": true,
	"range": true, "try": true, "catch": true, "return": true, "and": true,
	"or": true, "not": true, "nil": true, "msg": true, "trans": true,
	"true": true, "false": true,
}

// jetFunctions are the functions that Jet defines for every template, from
// defaultVariables in default.go of github.com/CloudyKit/jet/v6 v6.3.3. A
// top-level variable with one of these names hides the function in that
// page: front matter "raw: true" would make every layout fail at
// {{ copy|raw }}. A nested name, such as data.map, hides nothing. Check this
// list when Jet is upgraded.
var jetFunctions = map[string]bool{
	"lower": true, "upper": true, "hasPrefix": true, "hasSuffix": true,
	"repeat": true, "replace": true, "split": true, "trimSpace": true,
	"html": true, "url": true, "safeHtml": true, "safeJs": true, "raw": true,
	"unsafe": true, "writeJson": true, "json": true, "map": true,
	"slice": true, "array": true, "isset": true, "len": true,
	"includeIfExists": true, "exec": true, "ints": true, "dump": true,
}

// reservedPurposes maps the template variables that Gondola itself defines to
// what templates use them for, as the error message explains it. A front
// matter key or a default: key with one of these names would hide Gondola's
// variable.
var reservedPurposes = map[string]string{
	"copy":        "the page's HTML",
	"site":        "the site settings",
	"data":        "the data files",
	"fleet":       "the collection of all pages",
	"collections": "the collections of tagged pages",
	"page":        "the page's URL and metadata",
}

// CheckReserved returns an error if name, the template name of key, cannot
// be a top-level template variable: it is one of the variables that Gondola
// reserves for itself (copy, site, data, fleet, collections, page), or the
// name of one of Jet's built-in functions (raw, len, isset, …). noun is used
// as for TemplateName.
func CheckReserved(key, name, noun string) error {
	if jetFunctions[name] {
		if name == key {
			return fmt.Errorf("%s %q is the name of a Jet function and cannot be a template variable; please rename it", noun, key)
		}
		return fmt.Errorf("%s %q becomes %q in templates, which is the name of a Jet function; please rename it", noun, key, name)
	}
	purpose, reserved := reservedPurposes[name]
	if !reserved {
		return nil
	}
	if name == key {
		return fmt.Errorf("%s %q is reserved by Gondola (templates use it for %s); please rename it", noun, key, purpose)
	}
	return fmt.Errorf("%s %q becomes %q in templates, which is reserved by Gondola (templates use it for %s); please rename it", noun, key, name, purpose)
}

// TemplateName converts key to the name templates use for it and checks that
// the name is a valid, non-keyword Jet identifier.
//
// The key is split on "-" and "_". A first word written in capitals is
// lowercased entirely, as Go does with initialisms: "URL-path" becomes
// "urlPath". Any other first word only gets its first letter lowercased:
// "ExcludeFromFleet" becomes "excludeFromFleet". Every following word gets
// its first letter uppercased and keeps the rest as written, so "og-image"
// and "og_image" both become "ogImage".
//
// noun names what the key is in error messages, such as "key" or "front
// matter key".
func TemplateName(key, noun string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("%s %q is empty; please give it a name", noun, key)
	}
	words := strings.FieldsFunc(key, isWordSeparator)
	// FieldsFunc drops empty words, so comparing counts finds leading,
	// trailing and doubled separators.
	if len(words) != strings.Count(key, "-")+strings.Count(key, "_")+1 {
		return "", fmt.Errorf("%s %q has an empty word; please remove the extra \"-\" or \"_\"", noun, key)
	}

	var name strings.Builder
	for i, word := range words {
		if i == 0 {
			name.WriteString(lowerFirstWord(word))
		} else {
			name.WriteString(withFirstRune(word, unicode.ToUpper))
		}
	}
	return checkName(key, name.String(), noun)
}

func isWordSeparator(r rune) bool {
	return strings.ContainsRune(wordSeparators, r)
}

// lowerFirstWord lowercases the first word of a key: entirely when it is
// written in capitals, otherwise only its first letter.
func lowerFirstWord(word string) string {
	if isAllUppercase(word) {
		return strings.ToLower(word)
	}
	return withFirstRune(word, unicode.ToLower)
}

// isAllUppercase reports whether word is written in capitals: it starts with
// an uppercase letter and contains no lowercase letter. Digits and letters
// without case do not count, so "HTML5" is written in capitals. Requiring an
// uppercase first letter keeps a word such as "日本X" out, and keeps the
// conversion stable when a converted name is converted again. Invalid UTF-8
// is left to checkName, which reports it as written.
func isAllUppercase(word string) bool {
	first, _ := utf8.DecodeRuneInString(word)
	if !unicode.IsUpper(first) || !utf8.ValidString(word) {
		return false
	}
	for _, r := range word {
		if unicode.IsLower(r) {
			return false
		}
	}
	return true
}

// withFirstRune returns word with its first rune passed through change.
func withFirstRune(word string, change func(rune) rune) string {
	first, size := utf8.DecodeRuneInString(word)
	if first == utf8.RuneError {
		// Invalid UTF-8 is kept as it is; checkName rejects it.
		return word
	}
	return string(change(first)) + word[size:]
}

// checkName rejects a converted name that Jet could not read as a variable.
// Jet accepts any Unicode letter or digit in identifiers, so this does too.
func checkName(key, name, noun string) (string, error) {
	first, _ := utf8.DecodeRuneInString(name)
	if !unicode.IsLetter(first) {
		return "", fmt.Errorf("%s %q becomes %q in templates, which is not a valid name (names must start with a letter); please rename it", noun, key, name)
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", fmt.Errorf("%s %q becomes %q in templates, which is not a valid name (names may contain only letters and digits); please rename it", noun, key, name)
		}
	}
	if jetKeywords[name] {
		if name == key {
			return "", fmt.Errorf("%s %q is a Jet keyword and cannot be a template variable; please rename it", noun, key)
		}
		return "", fmt.Errorf("%s %q becomes %q in templates, which is a Jet keyword; please rename it", noun, key, name)
	}
	return name, nil
}

// Scope converts the keys found in one place (one frontmatter block, the
// default: section, one data directory) and reports two keys that become the
// same template name, such as "robots-agent" and "robotsAgent".
type Scope struct {
	noun      string
	keyByName map[string]string
}

// NewScope returns an empty Scope. noun is used in error messages as for
// TemplateName; its plural is formed by adding "s", so it must be a regular
// noun such as "key" or "data file name".
func NewScope(noun string) *Scope {
	return &Scope{noun: noun, keyByName: make(map[string]string)}
}

// Add converts key with TemplateName and records it. It returns an error if
// the key cannot be converted or if an earlier key became the same name.
// Callers add keys in a stable order (the order of the file, or sorted when
// the keys come from a Go map) so that the error names them in a stable order.
func (s *Scope) Add(key string) (string, error) {
	name, err := TemplateName(key, s.noun)
	if err != nil {
		return "", err
	}
	if earlierKey, taken := s.keyByName[name]; taken {
		return "", fmt.Errorf("%ss %q and %q both become %q in templates; please rename one", s.noun, earlierKey, key, name)
	}
	s.keyByName[name] = key
	return name, nil
}
