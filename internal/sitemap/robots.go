// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package sitemap

import (
	"bytes"
	"strings"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// RobotsFileName is the name of the robots.txt file at the root of the
// output, the only place where crawlers look for it.
const RobotsFileName = "robots.txt"

// globalAgent is the user-agent value of the group that applies to every
// crawler without a group of its own (RFC 9309, section 2.2.1).
const globalAgent = "*"

// endAnchor at the end of a rule's path pattern means that the pattern must
// match the whole URL path, not only its beginning (RFC 9309, 2.2.3).
const endAnchor = '$'

// wildcard in a path pattern matches any run of characters, including none.
const wildcard = '*'

// Rules are the Allow and Disallow rules of robots.txt that apply to one
// user-agent. The zero value has no rules and allows every URL path, as a
// missing robots.txt does.
type Rules struct {
	rules []rule
}

// rule is one Allow or Disallow line, with its path pattern already
// normalized (see normalizePercentEncoding).
type rule struct {
	allow   bool
	pattern string
}

// group is a run of User-agent lines and the rules that follow them.
type group struct {
	agents []string
	rules  []rule
}

// ParseRobots reads robots.txt and returns the rules for agent, the value
// of config.robots-agent. sourcePath names the file in errors: it is the
// source that wrote robots.txt into the output, relative to the project
// root.
//
// The file is read as RFC 9309 says and as crawlers do: lines that are not
// understood (unknown or misspelled keys, lines without ":", rules before
// any User-agent line) are ignored without an error, so that the sitemap
// agrees with what crawlers will do with the same file (M10, Q6). The only
// error is a file saved as UTF-16 (SPEC §4).
//
// The group chosen is every group whose User-agent matches agent ignoring
// letter case, merged into one; otherwise every "*" group, merged;
// otherwise there are no rules and everything is allowed.
func ParseRobots(sourcePath string, src []byte, agent string) (Rules, error) {
	if err := sourcetext.CheckEncoding(src); err != nil {
		return Rules{}, diag.WithFile(err, sourcePath)
	}
	groups := parseGroups(sourcetext.TrimByteOrderMark(src))
	var specific, global []rule
	// foundSpecific is needed besides specific: a matching group without
	// rules ("Disallow:" alone) allows everything, and must still win over
	// the "*" group.
	foundSpecific := false
	for _, g := range groups {
		matchesAgent, matchesGlobal := false, false
		for _, groupAgent := range g.agents {
			if groupAgent == globalAgent {
				matchesGlobal = true
			} else if strings.EqualFold(groupAgent, agent) {
				matchesAgent = true
			}
		}
		if matchesAgent {
			foundSpecific = true
			specific = append(specific, g.rules...)
		}
		if matchesGlobal {
			global = append(global, g.rules...)
		}
	}
	if foundSpecific {
		return Rules{rules: specific}, nil
	}
	return Rules{rules: global}, nil
}

// parseGroups splits robots.txt into groups. A group starts with one or
// more User-agent lines; a User-agent line that follows a rule starts the
// next group. Lines of other kinds (Sitemap, Crawl-delay, anything unknown)
// neither end a group nor belong to it.
func parseGroups(text []byte) []group {
	var groups []group
	// collectingAgents is true when no rule has been read since the last
	// User-agent line, so that the next User-agent line joins its group.
	collectingAgents := false
	for _, line := range splitLines(text) {
		key, value, ok := splitLine(line)
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "user-agent":
			if !collectingAgents {
				groups = append(groups, group{})
				collectingAgents = true
			}
			current := &groups[len(groups)-1]
			current.agents = append(current.agents, agentToken(value))
		case "allow", "disallow":
			// A rule before the first User-agent line belongs to no group
			// and is ignored, as RFC 9309 says. An empty path matches
			// nothing ("Disallow:" allows everything), so it adds no rule.
			collectingAgents = false
			if len(groups) == 0 || value == "" {
				continue
			}
			current := &groups[len(groups)-1]
			current.rules = append(current.rules, rule{
				allow:   strings.EqualFold(key, "allow"),
				pattern: normalizePercentEncoding(value),
			})
		}
		// Any other key (Sitemap, Crawl-delay, Host, a misspelling) is
		// ignored (M10, Q6).
	}
	return groups
}

// splitLines splits text at every line break: LF, CRLF or a lone CR, all of
// which RFC 9309 allows.
func splitLines(text []byte) []string {
	normalized := bytes.ReplaceAll(text, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	return strings.Split(string(normalized), "\n")
}

// splitLine returns the key and the value of a line, without the comment
// and the spaces around both, and false for a line without ":" (blank lines
// and comments included).
func splitLine(line string) (key, value string, ok bool) {
	if commentStart := strings.IndexByte(line, '#'); commentStart >= 0 {
		line = line[:commentStart]
	}
	key, value, found := strings.Cut(line, ":")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(key), strings.TrimSpace(value), true
}

// agentToken returns the product token of a User-agent value: the leading
// letters, "_" and "-", which RFC 9309 allows in a token, so that
// "Googlebot/2.1" is the group of "Googlebot". "*" stays as it is. A value
// that starts with another character gives "", which matches no agent.
func agentToken(value string) string {
	if value == globalAgent || strings.HasPrefix(value, globalAgent+" ") || strings.HasPrefix(value, globalAgent+"\t") {
		return globalAgent
	}
	end := 0
	for end < len(value) && isTokenByte(value[end]) {
		end++
	}
	return value[:end]
}

func isTokenByte(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || b == '_' || b == '-'
}

// Allows reports whether the rules allow urlPath, a percent-encoded URL
// path that begins with "/". The longest matching pattern wins, counted in
// bytes after normalization, and Allow wins a tie between patterns of the
// same length; a path that no rule matches is allowed (RFC 9309, 2.2.2).
func (r Rules) Allows(urlPath string) bool {
	normalizedPath := normalizePercentEncoding(urlPath)
	bestLength := -1
	allowed := true
	for _, candidate := range r.rules {
		if !matchesPattern(candidate.pattern, normalizedPath) {
			continue
		}
		length := len(candidate.pattern)
		if length > bestLength || length == bestLength && candidate.allow {
			bestLength = length
			allowed = candidate.allow
		}
	}
	return allowed
}

// matchesPattern reports whether a path pattern matches urlPath, both
// normalized. Without a final "$" the pattern only has to match the
// beginning of the path; "$" elsewhere is an ordinary character, as for
// Google's crawler.
func matchesPattern(pattern, urlPath string) bool {
	if strings.HasSuffix(pattern, string(endAnchor)) {
		return matchesWhole(pattern[:len(pattern)-1], urlPath)
	}
	// Matching the beginning is matching the whole with a "*" at the end.
	return matchesWhole(pattern+string(wildcard), urlPath)
}

// matchesWhole reports whether pattern, in which only "*" is special,
// matches the whole of text.
//
// It walks both strings once and, on a mismatch, returns to the last "*"
// and lets it swallow one more byte of text. Going back to the last "*"
// only is enough, because a later "*" can swallow anything an earlier one
// could; so the time is at most the product of the two lengths, never
// exponential.
func matchesWhole(pattern, text string) bool {
	patternIndex, textIndex := 0, 0
	// lastStar is the index in pattern of the last "*" seen, or -1, and
	// textAfterStar the index in text where that "*" stopped swallowing.
	lastStar, textAfterStar := -1, 0
	for textIndex < len(text) {
		switch {
		case patternIndex < len(pattern) && pattern[patternIndex] == wildcard:
			lastStar, textAfterStar = patternIndex, textIndex
			patternIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == text[textIndex]:
			patternIndex++
			textIndex++
		case lastStar >= 0:
			textAfterStar++
			patternIndex, textIndex = lastStar+1, textAfterStar
		default:
			return false
		}
	}
	// What is left of the pattern must be only "*", which match nothing.
	for patternIndex < len(pattern) && pattern[patternIndex] == wildcard {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

// normalizePercentEncoding writes a path or a path pattern in the form in
// which RFC 9309 (section 2.2.2) compares them, so that "/città",
// "/citt%c3%a0" and "/citt%C3%A0" are the same path, and so are "/%62az"
// and "/baz":
//
//   - every byte outside printable ASCII (letters with accents, spaces,
//     control characters) is percent-encoded;
//   - a percent-encoded unreserved character (a letter, a digit, "-", ".",
//     "_" or "~") is decoded, and any other percent-encoding is kept with
//     its hexadecimal digits in capitals;
//   - a "%" that does not begin a percent-encoding stands for itself, so it
//     is encoded as "%25".
//
// "*" and "$" are left as they are: in a pattern they are special.
func normalizePercentEncoding(text string) string {
	var normalized strings.Builder
	for i := 0; i < len(text); i++ {
		b := text[i]
		switch {
		case b == '%' && i+2 < len(text) && isHexDigit(text[i+1]) && isHexDigit(text[i+2]):
			decoded := hexValue(text[i+1])<<4 | hexValue(text[i+2])
			if isUnreserved(decoded) {
				normalized.WriteByte(decoded)
			} else {
				writePercentEncoded(&normalized, decoded)
			}
			i += 2
		case b == '%' || b <= ' ' || b >= 0x7F:
			writePercentEncoded(&normalized, b)
		default:
			normalized.WriteByte(b)
		}
	}
	return normalized.String()
}

// upperHexDigits are the digits of a percent-encoding, in capitals as
// RFC 3986 recommends.
const upperHexDigits = "0123456789ABCDEF"

func writePercentEncoded(normalized *strings.Builder, b byte) {
	normalized.WriteByte('%')
	normalized.WriteByte(upperHexDigits[b>>4])
	normalized.WriteByte(upperHexDigits[b&0x0F])
}

func isHexDigit(b byte) bool {
	return '0' <= b && b <= '9' || 'a' <= b && b <= 'f' || 'A' <= b && b <= 'F'
}

func hexValue(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	default:
		return b - 'A' + 10
	}
}

// isUnreserved reports whether b is an unreserved character of RFC 3986,
// which means the same whether it is percent-encoded or not.
func isUnreserved(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' ||
		b == '-' || b == '.' || b == '_' || b == '~'
}
