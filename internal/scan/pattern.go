// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package scan

import (
	"path"
	"slices"
	"strings"

	"github.com/luciaduck/gondola/internal/config"
)

// passthroughLabel names the key of the patterns in messages.
const passthroughLabel = "output.passthrough"

// pattern is a checked passthrough pattern (SPEC §5.6). Every pattern is
// anchored at the project root, whether or not it starts with "/".
type pattern struct {
	// written is the pattern exactly as written, for messages.
	written  string
	position config.Position
	// levels are the path levels to match, one path.Match pattern each,
	// without the leading and trailing "/". When the file system ignores
	// letter case they are in small letters, and so is every name compared
	// with them.
	levels []string
	// coversDirectory is true for a pattern that ends in "/": it matches the
	// files of the directories it names, at any depth.
	coversDirectory   bool
	ignoresLetterCase bool
}

// compilePattern checks a pattern as written in gondola.yaml. Errors point
// at the pattern there.
func compilePattern(written config.PassthroughPattern, ignoresLetterCase bool) (pattern, error) {
	text := written.Pattern
	at := written.Position
	// path.Match reads "\" as an escape, but a Windows user who writes one
	// means a directory separator. Refusing it keeps both readings out.
	if problem := config.PathCharacterProblem(text); problem != "" {
		return pattern{}, at.Errorf("%s %q %s", passthroughLabel, text, problem)
	}
	relative := strings.TrimPrefix(text, "/")
	// ".." is looked for before cleaning, which would remove "a/../b", and a
	// drive after it, which turns "./C:x" into "C:x". When letter case is
	// ignored, the pattern is matched in small letters, so that form is
	// checked too: the Kelvin sign (U+212A) becomes an ASCII "k".
	cleaned := path.Clean(relative)
	isDrive := hasDriveLetter(cleaned) || ignoresLetterCase && hasDriveLetter(strings.ToLower(cleaned))
	if strings.HasPrefix(relative, "/") || isDrive || slices.Contains(strings.Split(relative, "/"), "..") {
		return pattern{}, at.Errorf(`%s %q is not allowed: patterns are relative to the project directory and may not contain ".." or be absolute`, passthroughLabel, text)
	}
	if cleaned == "." {
		return pattern{}, at.Errorf("%s %q is the whole project directory; please name the files or directories to copy", passthroughLabel, text)
	}
	// path.Match reads "**" as "*", which would quietly match one level
	// where a .gitignore user expects any number of them.
	if strings.Contains(text, "**") {
		return pattern{}, at.Errorf(`%s %q uses "**", which Gondola does not support; to copy a directory and everything inside it, end the pattern with "/", as in "assets/"`, passthroughLabel, text)
	}
	compiled := pattern{
		written:           text,
		position:          at,
		coversDirectory:   strings.HasSuffix(relative, "/"),
		ignoresLetterCase: ignoresLetterCase,
	}
	for _, level := range strings.Split(cleaned, "/") {
		level = compiled.comparable(level)
		// path.Match checks the whole pattern whatever the name, so an empty
		// name is enough to find a malformed "[...]".
		if _, err := path.Match(level, ""); err != nil {
			return pattern{}, at.Errorf(`%s %q is not a valid pattern; please check the characters written between "[" and "]"`, passthroughLabel, text)
		}
		compiled.levels = append(compiled.levels, level)
	}
	return compiled, nil
}

// hasDriveLetter reports whether text starts with a Windows drive, such as
// "C:" in "C:/site" or "C:site".
func hasDriveLetter(text string) bool {
	return len(text) >= 2 && text[1] == ':' && ('a' <= text[0] && text[0] <= 'z' || 'A' <= text[0] && text[0] <= 'Z')
}

// comparable returns a name or pattern level in the form that is compared.
func (p pattern) comparable(name string) string {
	if p.ignoresLetterCase {
		return strings.ToLower(name)
	}
	return name
}

// matchesFile reports whether the file at the path levels (relative to the
// project root) is to be passed through.
func (p pattern) matchesFile(levels []string) bool {
	if p.coversDirectory {
		return len(levels) > len(p.levels) && p.matchesLevels(levels[:len(p.levels)])
	}
	return len(levels) == len(p.levels) && p.matchesLevels(levels)
}

// matchesDirectoryItself reports whether a pattern without a trailing "/"
// names the directory at levels. It copies nothing, but explains a pattern
// that matches no file.
func (p pattern) matchesDirectoryItself(levels []string) bool {
	return !p.coversDirectory && len(levels) == len(p.levels) && p.matchesLevels(levels)
}

// mayMatchBelow reports whether a file inside the directory at levels can
// match: either the directory is inside one that the pattern covers, or its
// levels match the start of the pattern. The walk enters no other directory
// for passthrough, so .git stays closed unless a pattern points into it.
func (p pattern) mayMatchBelow(dirLevels []string) bool {
	if p.coversDirectory && len(dirLevels) >= len(p.levels) {
		return p.matchesLevels(dirLevels[:len(p.levels)])
	}
	return len(dirLevels) < len(p.levels) && p.matchesLevels(dirLevels)
}

// matchesLevels matches names against the first len(names) levels of the
// pattern.
func (p pattern) matchesLevels(names []string) bool {
	for i, name := range names {
		// The error is ignored because compilePattern has checked every level.
		matched, _ := path.Match(p.levels[i], p.comparable(name))
		if !matched {
			return false
		}
	}
	return true
}
