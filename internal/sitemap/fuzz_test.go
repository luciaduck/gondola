package sitemap

import (
	"encoding/xml"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/routing"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// referenceMatch is a second, independent way to match a normalized path
// pattern: it translates the pattern into a regular expression, where "*"
// becomes ".*" and a final "$" becomes the end of the text. The fuzzer
// compares it with matchesPattern.
func referenceMatch(t *testing.T, pattern, urlPath string) bool {
	t.Helper()
	anchored := strings.HasSuffix(pattern, "$")
	body := strings.TrimSuffix(pattern, "$")
	parts := strings.Split(body, "*")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	expression := `\A(?s)` + strings.Join(parts, ".*")
	if anchored {
		expression += `\z`
	}
	compiled, err := regexp.Compile(expression)
	if err != nil {
		t.Fatalf("pattern %q: %v", pattern, err)
	}
	return compiled.MatchString(urlPath)
}

// isPrintableASCII reports whether text holds only the bytes that a
// normalized path may hold.
func isPrintableASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] <= ' ' || text[i] >= 0x7F {
			return false
		}
	}
	return true
}

// FuzzRobotsPattern compares the hand-written pattern matcher with
// referenceMatch, and checks that normalizing gives printable ASCII and
// changes nothing the second time.
func FuzzRobotsPattern(f *testing.F) {
	seeds := []struct{ pattern, urlPath string }{
		{"/", "/"},
		{"/fish", "/fish.html"},
		{"/fish*.php", "/fishheads/catfish.php?parameters"},
		{"/*.php$", "/filename.php"},
		{"/*.php$", "/filename.php5"},
		{"/a$b", "/a$b"},
		{"/a$$", "/a$"},
		{"*", ""},
		{"$", ""},
		{"/*a*b*c$", "/xaybzc"},
		{"/**", "/x"},
		{"/città", "/citt%C3%A0.html"},
		{"/%62az", "/baz"},
		{"/a%2Fb", "/a/b"},
		{"/100%", "/100%25"},
		{"/a%4", "/a%254"},
		{"/\xff", "/%ff"},
	}
	for _, seed := range seeds {
		f.Add(seed.pattern, seed.urlPath)
	}
	f.Fuzz(func(t *testing.T, pattern, urlPath string) {
		normalizedPattern := normalizePercentEncoding(pattern)
		normalizedPath := normalizePercentEncoding(urlPath)
		for _, normalized := range []string{normalizedPattern, normalizedPath} {
			if !isPrintableASCII(normalized) {
				t.Errorf("normalized text %q is not printable ASCII", normalized)
			}
			if again := normalizePercentEncoding(normalized); again != normalized {
				t.Errorf("normalizing %q again gives %q", normalized, again)
			}
		}
		got := matchesPattern(normalizedPattern, normalizedPath)
		if want := referenceMatch(t, normalizedPattern, normalizedPath); got != want {
			t.Fatalf("matchesPattern(%q, %q) = %v, the reference says %v", normalizedPattern, normalizedPath, got, want)
		}
		// Allows normalizes the path itself and applies the only rule.
		rules := Rules{rules: []rule{{allow: false, pattern: normalizedPattern}}}
		if allowed := rules.Allows(urlPath); allowed == got {
			t.Errorf("Disallow %q: Allows(%q) = %v, but the pattern matches: %v", normalizedPattern, urlPath, allowed, got)
		}
	})
}

// unrelatedAgent is the agent of a group that FuzzRobotsFile adds in front
// of the fuzzed file: it must change nothing for any other agent.
const unrelatedAgent = "gondolaunrelatedbot"

// FuzzRobotsFile parses any text as robots.txt and checks what must hold
// whatever the text: it never panics; the only error is UTF-16; the answer
// is the same on a second parse and with a byte order mark in front; a
// group for another agent added in front changes nothing; and a file
// without the word "allow" (which "disallow" contains) allows everything.
func FuzzRobotsFile(f *testing.F) {
	seeds := []struct{ robots, agent, urlPath string }{
		{"", "*", "/"},
		{rfcExample, "foobot", "/example/page.html"},
		{rfcExample, "barbot", "/example/page.html"},
		{rfcExample, "otherbot", "/x.gif"},
		{"User-agent: *\nDisallow: /\n", "*", "/a"},
		{"User-agent: a\nUser-agent: b\nDisallow: /x\n\nUser-agent: a\nAllow: /x/y\n", "A", "/x/y/z"},
		{"User-agent: Googlebot/2.1\r\nDisallow: /p # private\r\nSitemap: https://example.com/s.xml\r\n", "googlebot", "/p"},
		{"Disallow: /\nUser-agent: *\nCrawl-delay: 5\nAllow: /$\nDisallow: /\n", "x", "/"},
		{"User-agent: *\rDisallow: /città\r", "*", "/citt%C3%A0.html"},
		{"\xEF\xBB\xBFUser-agent: *\nDisallow: /\n", "*", "/"},
		{"\xFF\xFEU\x00", "*", "/"},
		{"User-agent: * all\nDisalow: /x\nDisallow\n", "*", "/x"},
	}
	for _, seed := range seeds {
		f.Add(seed.robots, seed.agent, seed.urlPath)
	}
	f.Fuzz(func(t *testing.T, robots, agent, urlPath string) {
		rules, err := ParseRobots("robots.txt", []byte(robots), agent)
		if err != nil {
			if !strings.HasPrefix(robots, "\xFF\xFE") && !strings.HasPrefix(robots, "\xFE\xFF") {
				t.Fatalf("error for a file that is not UTF-16: %v", err)
			}
			return
		}
		allowed := rules.Allows(urlPath)
		again, _ := ParseRobots("robots.txt", []byte(robots), agent)
		if again.Allows(urlPath) != allowed {
			t.Error("two parses of the same file disagree")
		}
		// A byte order mark means something only at the start of the file,
		// so the changed files below are made from the text without it.
		text := string(sourcetext.TrimByteOrderMark([]byte(robots)))
		withMark, err := ParseRobots("robots.txt", []byte("\xEF\xBB\xBF"+text), agent)
		if err != nil || withMark.Allows(urlPath) != allowed {
			t.Errorf("a byte order mark changes the answer (error %v)", err)
		}
		if !strings.EqualFold(agentToken(agent), unrelatedAgent) {
			prefixed, err := ParseRobots("robots.txt", []byte("User-agent: "+unrelatedAgent+"\nDisallow: /\n"+text), agent)
			if err != nil || prefixed.Allows(urlPath) != allowed {
				t.Errorf("a group for %s changes the answer for %q (error %v)", unrelatedAgent, agent, err)
			}
		}
		if !strings.Contains(strings.ToLower(robots), "allow") && !allowed {
			t.Errorf("a file without rules disallows %q", urlPath)
		}
	})
}

// fuzzCanonicalURL holds the characters that XML must escape, so that every
// fuzzed sitemap exercises the escaping.
const fuzzCanonicalURL = "https://example.com/a&b'c"

// parsedSitemap is the shape of sitemap.xml for encoding/xml.
type parsedSitemap struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []struct {
		Loc     string `xml:"loc"`
		Lastmod string `xml:"lastmod"`
	} `xml:"url"`
}

// FuzzSitemap routes up to three pages with fuzzed permalinks, as the build
// does, and checks that the sitemap is well-formed XML that holds exactly
// the listed pages, in order, with their URLs intact after escaping.
func FuzzSitemap(f *testing.F) {
	seeds := []struct {
		permalinkA, permalinkB, permalinkC string
		flags                              uint8
		robots                             string
	}{
		{"path", "path", "path", 0b111111, ""},
		{"/", "/fish&chips's.html", "/città/", 0b000111, ""},
		{"/feed.xml", "/about", "/a.HTM", 0b101111, ""},
		{"/<b>\"q\".html", "/a b.html", "/100%.html", 0b111111, "User-agent: *\nDisallow: /a\n"},
		{"/x.html", "/X.html", "/y/", 0b000101, "User-agent: *\nDisallow: /\nAllow: /y/$\n"},
		{"/a/../b", "/a\\b", "/a?b", 0b111111, ""},
	}
	for _, seed := range seeds {
		f.Add(seed.permalinkA, seed.permalinkB, seed.permalinkC, seed.flags, seed.robots)
	}
	f.Fuzz(func(t *testing.T, permalinkA, permalinkB, permalinkC string, flags uint8, robots string) {
		// Bits 0 to 2 say whether each page is listed, bits 3 to 5 whether
		// it has a date.
		date := time.Date(2024, 5, 1, 23, 30, 0, 0, time.FixedZone("", -5*60*60))
		var pages []Page
		claimed := map[string]bool{}
		for i, permalink := range []string{permalinkA, permalinkB, permalinkC} {
			sourcePath := string(rune('a'+i)) + ".md"
			location, err := routing.Route(routing.Page{SourcePath: sourcePath, ContentPath: sourcePath, Permalink: permalink}, config.RoutingFile)
			// The build refuses a second page with the same output path,
			// in any letter case.
			if err != nil || claimed[strings.ToLower(location.OutputPath)] {
				continue
			}
			claimed[strings.ToLower(location.OutputPath)] = true
			pages = append(pages, Page{
				OutputPath: location.OutputPath,
				URLPath:    location.URLPath,
				Listed:     flags&(1<<i) != 0,
				Date:       date,
				HasDate:    flags&(1<<(i+3)) != 0,
			})
		}
		rules, err := ParseRobots("robots.txt", []byte(robots), "*")
		if err != nil {
			rules = Rules{}
		}
		text, count, err := Build(pages, fuzzCanonicalURL, rules)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		var parsed parsedSitemap
		if err := xml.Unmarshal(text, &parsed); err != nil {
			t.Fatalf("the sitemap is not well-formed XML: %v\n%s", err, text)
		}
		var want []Page
		for _, page := range pages {
			if page.Listed && isHTMLOutput(page.OutputPath) && rules.Allows(page.URLPath) {
				want = append(want, page)
			}
		}
		sort.Slice(want, func(i, j int) bool { return want[i].URLPath < want[j].URLPath })
		if count != len(want) || len(parsed.URLs) != len(want) {
			t.Fatalf("count %d and %d <url> elements, want %d:\n%s", count, len(parsed.URLs), len(want), text)
		}
		for i, page := range want {
			if parsed.URLs[i].Loc != fuzzCanonicalURL+page.URLPath {
				t.Errorf("loc %d is %q, want %q", i, parsed.URLs[i].Loc, fuzzCanonicalURL+page.URLPath)
			}
			wantLastmod := ""
			if page.HasDate {
				wantLastmod = "2024-05-01"
			}
			if parsed.URLs[i].Lastmod != wantLastmod {
				t.Errorf("lastmod %d is %q, want %q", i, parsed.URLs[i].Lastmod, wantLastmod)
			}
		}
	})
}

// FuzzCanaryRobots is written to fail: it claims that, with the rules
// below, no URL path that begins with "/a" is allowed. "/a/b" is, because
// the longer Allow wins, and so is "/a/%62", which normalizes to it. It
// shows that the fuzzer reaches the end of a long match. It is skipped
// unless GONDOLA_FUZZ_CANARY is set, so that go test stays green with the
// failing input saved under testdata/fuzz:
//
//	GONDOLA_FUZZ_CANARY=1 go test ./internal/sitemap -run '^$' -fuzz FuzzCanaryRobots -fuzztime 1m
func FuzzCanaryRobots(f *testing.F) {
	if os.Getenv("GONDOLA_FUZZ_CANARY") == "" {
		f.Skip("the canary fails on purpose; set GONDOLA_FUZZ_CANARY to run it")
	}
	// Only seeds that keep the claim true: the fuzzer has to find the
	// counterexample itself.
	for _, seed := range []string{"/", "/a", "/a/c", "/a/bc"} {
		f.Add(seed)
	}
	rules, err := ParseRobots("robots.txt", []byte("User-agent: *\nDisallow: /a\nAllow: /a/b$\n"), "*")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, urlPath string) {
		if strings.HasPrefix(urlPath, "/a") && rules.Allows(urlPath) {
			t.Errorf("%q begins with /a and is allowed", urlPath)
		}
	})
}
