package frontmatter

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The permalink values that choose how Gondola names a page's output file
// (SPEC §5.7). Package config gives them their type, config.Permalink; they
// are defined here because front matter accepts them too, and config
// imports this package.
const (
	PermalinkPath = "path"
	PermalinkSlug = "slug"
	PermalinkDate = "date"
)

// customPermalinkPrefix starts every custom permalink, such as "/about-us/".
// It also tells a custom permalink apart from the three choices.
const customPermalinkPrefix = "/"

// permalinkCharacterProblems are the characters refused in a custom
// permalink, with the reason given in the message. A permalink names a file
// in the output, so it is a path, not a URL, and it must be a valid file
// name on every system the site may be built on.
var permalinkCharacterProblems = []struct {
	character string
	problem   string
}{
	{`\`, `please separate directories with "/"`},
	{"?", "a permalink names a file in the output, so it cannot have a query (the part after \"?\")"},
	{"#", "a permalink names a file in the output, so it cannot have a fragment (the part after \"#\")"},
	{":", "Windows does not allow it in file names; please remove it"},
}

// IsCustomPermalink reports whether permalink is a path written by the user,
// such as "/about-us/", rather than one of the choices "path", "slug" and
// "date".
func IsCustomPermalink(permalink string) bool {
	return strings.HasPrefix(permalink, customPermalinkPrefix)
}

// CheckPermalink returns an error when permalink is neither one of the
// choices "path", "slug" and "date" nor a valid custom permalink: a path that
// begins with "/", contains no ".." and none of the characters refused by
// permalinkCharacterProblems or control characters. Every permalink must be
// valid UTF-8. The error has no position and names the key "permalink".
//
// Front matter checks every permalink when it is read, where the position is
// known; package routing checks again, so that no other caller can make a
// page escape the output directory.
func CheckPermalink(permalink string) error {
	return checkPermalink(permalinkKey, permalink)
}

// checkPermalink is CheckPermalink with the key named by label in messages,
// as the user wrote it ("permalink" or "Permalink").
func checkPermalink(label, permalink string) error {
	switch permalink {
	case PermalinkPath, PermalinkSlug, PermalinkDate:
		return nil
	}
	// yaml.v3 refuses text that is not UTF-8, so front matter never gets
	// here with one; the check keeps CheckPermalink in step with routing,
	// which accepts only UTF-8 paths.
	if !utf8.ValidString(permalink) {
		return fmt.Errorf("%s %q is not valid UTF-8; please write it in UTF-8", label, permalink)
	}
	if !IsCustomPermalink(permalink) {
		return fmt.Errorf(`%s %q must be "path", "slug" or "date", or a path that begins with "/"`, label, permalink)
	}
	for _, character := range permalink {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s %q contains a control character (%U); please remove it", label, permalink, character)
		}
	}
	for _, refused := range permalinkCharacterProblems {
		if strings.Contains(permalink, refused.character) {
			return fmt.Errorf("%s %q contains %q; %s", label, permalink, refused.character, refused.problem)
		}
	}
	if slices.Contains(strings.Split(permalink, "/"), "..") {
		return fmt.Errorf(`%s %q is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`, label, permalink)
	}
	return nil
}
