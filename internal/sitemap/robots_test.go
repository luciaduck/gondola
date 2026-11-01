package sitemap

import (
	"testing"
)

// rfcExample is the example file of RFC 9309, section 5.1.
const rfcExample = `User-Agent: *
Disallow: *.gif$
Disallow: /example/
Allow: /publications/

User-Agent: foobot
Disallow:/
Allow:/example/page.html
Allow:/example/allowed.gif

User-Agent: barbot
User-Agent: bazbot
Disallow: /example/page.html

User-Agent: quxbot
`

// robotsCase is one question to a robots.txt file: does it allow urlPath
// for agent?
type robotsCase struct {
	name    string
	robots  string
	agent   string
	urlPath string
	want    bool
}

func runRobotsCases(t *testing.T, tests []robotsCase) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rules, err := ParseRobots("robots.txt", []byte(test.robots), test.agent)
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if got := rules.Allows(test.urlPath); got != test.want {
				t.Errorf("Allows(%q) for agent %q is %v, want %v", test.urlPath, test.agent, got, test.want)
			}
		})
	}
}

func TestRobotsRFCExample(t *testing.T) {
	runRobotsCases(t, []robotsCase{
		{"foobot: an allowed page", rfcExample, "foobot", "/example/page.html", true},
		{"foobot: an allowed gif", rfcExample, "foobot", "/example/allowed.gif", true},
		{"foobot: everything else", rfcExample, "foobot", "/other.html", false},
		{"foobot: the root", rfcExample, "foobot", "/", false},
		{"barbot: the disallowed page", rfcExample, "barbot", "/example/page.html", false},
		{"barbot does not fall back to *", rfcExample, "barbot", "/example/other.html", true},
		{"bazbot shares barbot's group", rfcExample, "bazbot", "/example/page.html", false},
		{"quxbot: an empty group allows everything", rfcExample, "quxbot", "/example/page.html", true},
		{"quxbot: an empty group wins over *", rfcExample, "quxbot", "/x.gif", true},
		{"other agents: * forbids gif files", rfcExample, "otherbot", "/images/x.gif", false},
		{"other agents: $ anchors the end", rfcExample, "otherbot", "/images/x.gif.html", true},
		{"other agents: a disallowed directory", rfcExample, "otherbot", "/example/", false},
		{"other agents: an allowed directory", rfcExample, "otherbot", "/publications/a.html", true},
		{"other agents: a page without rules", rfcExample, "otherbot", "/a.html", true},
		{"the configured agent *", rfcExample, "*", "/example/", false},
		{"RFC 5.2: the longest match wins (allow)", "User-Agent: foobot\nAllow: /example/page/\nDisallow: /example/page/disallowed.gif\n", "foobot", "/example/page/", true},
		{"RFC 5.2: the longest match wins (disallow)", "User-Agent: foobot\nAllow: /example/page/\nDisallow: /example/page/disallowed.gif\n", "foobot", "/example/page/disallowed.gif", false},
	})
}

// TestRobotsGoogleExamples uses the examples of Google's documentation of
// robots.txt ("URL matching based on path values" and "Order of precedence
// for rules").
func TestRobotsGoogleExamples(t *testing.T) {
	disallow := func(pattern string) string { return "User-agent: *\nDisallow: " + pattern + "\n" }
	tests := []robotsCase{}
	add := func(pattern string, matching, notMatching []string) {
		for _, urlPath := range matching {
			tests = append(tests, robotsCase{pattern + " matches " + urlPath, disallow(pattern), "*", urlPath, false})
		}
		for _, urlPath := range notMatching {
			tests = append(tests, robotsCase{pattern + " does not match " + urlPath, disallow(pattern), "*", urlPath, true})
		}
	}
	add("/", []string{"/", "/a.html"}, nil)
	add("/*", []string{"/", "/a.html"}, nil)
	add("/$", []string{"/"}, []string{"/page.htm"})
	add("/fish", []string{"/fish", "/fish.html", "/fish/salmon.html", "/fishheads", "/fishheads/yummy.html", "/fish.php?id=anything"},
		[]string{"/Fish.asp", "/catfish", "/?id=fish", "/desert/fish"})
	add("/fish*", []string{"/fish", "/fish.html", "/fish/salmon.html", "/fishheads"}, []string{"/Fish.asp", "/catfish"})
	add("/fish/", []string{"/fish/", "/fish/?id=anything", "/fish/salmon.htm"}, []string{"/fish", "/fish.html", "/animals/fish/", "/Fish/Salmon.asp"})
	add("/*.php", []string{"/index.php", "/filename.php", "/folder/filename.php", "/folder/filename.php?parameters", "/folder/any.php.file.html", "/filename.php/"},
		[]string{"/", "/windows.PHP"})
	add("/*.php$", []string{"/filename.php", "/folder/filename.php"}, []string{"/filename.php?parameters", "/filename.php/", "/filename.php5", "/windows.PHP"})
	add("/fish*.php", []string{"/fish.php", "/fishheads/catfish.php?parameters"}, []string{"/Fish.PHP"})
	tests = append(tests,
		robotsCase{"a longer allow wins", "User-agent: *\nallow: /p\ndisallow: /\n", "*", "/page", true},
		robotsCase{"allow wins a tie", "User-agent: *\nallow: /folder\ndisallow: /folder\n", "*", "/folder/page", true},
		robotsCase{"a longer pattern with * wins", "User-agent: *\nallow: /page\ndisallow: /*.htm\n", "*", "/page.htm", false},
		robotsCase{"allow /$ keeps the home page", "User-agent: *\nallow: /$\ndisallow: /\n", "*", "/", true},
		robotsCase{"allow /$ does not keep other pages", "User-agent: *\nallow: /$\ndisallow: /\n", "*", "/page.htm", false},
	)
	runRobotsCases(t, tests)
}

func TestRobotsGroups(t *testing.T) {
	runRobotsCases(t, []robotsCase{
		{"no file: the zero Rules", "", "*", "/a", true},
		{"no group for the agent and no * group", "User-agent: other\nDisallow: /\n", "gondolabot", "/a", true},
		{"agents are compared ignoring letter case", "User-agent: GoogleBot\nDisallow: /\n", "googlebot", "/a", false},
		{"the configured agent ignores letter case too", "User-agent: googlebot\nDisallow: /\n", "GOOGLEBOT", "/a", false},
		{"a version after the token", "User-agent: Googlebot/2.1\nDisallow: /\n", "Googlebot", "/a", false},
		{"a token with - and _", "User-agent: Google-Image_bot\nDisallow: /\n", "google-image_bot", "/a", false},
		{"a longer token is another agent", "User-agent: Googlebot-News\nDisallow: /\n", "Googlebot", "/a", true},
		{"a value that is not a token matches nothing", "User-agent: /bot\nDisallow: /\n", "/bot", "/a", true},
		{"* followed by a comment", "User-agent: * # everyone\nDisallow: /\n", "gondolabot", "/a", false},
		{"* followed by text", "User-agent: * all\nDisallow: /\n", "gondolabot", "/a", false},
		{"a specific group wins over *", "User-agent: *\nDisallow: /\n\nUser-agent: gondolabot\nDisallow: /private/\n", "gondolabot", "/a", true},
		{"groups of the same agent are merged", "User-agent: a\nDisallow: /x\n\nUser-agent: b\nDisallow: /\n\nUser-agent: a\nDisallow: /y\n", "a", "/y", false},
		{"merged groups keep the first rules", "User-agent: a\nDisallow: /x\n\nUser-agent: a\nDisallow: /y\n", "a", "/x", false},
		{"* groups are merged", "User-agent: *\nDisallow: /x\nUser-agent: other\nDisallow: /\nUser-agent: *\nDisallow: /y\n", "a", "/y", false},
		{"a group for the agent and for *", "User-agent: a\nUser-agent: *\nDisallow: /x\n", "b", "/x", false},
		{"an empty group for the agent wins over *", "User-agent: a\nDisallow:\n\nUser-agent: *\nDisallow: /\n", "a", "/x", true},
		{"a rule ends the run of agents", "User-agent: a\nDisallow: /x\nUser-agent: b\nDisallow: /y\n", "b", "/x", true},
		{"a blank line does not end the run of agents", "User-agent: a\n\nUser-agent: b\nDisallow: /\n", "a", "/x", false},
		{"an unknown line does not end the run of agents", "User-agent: a\nCrawl-delay: 1\nUser-agent: b\nDisallow: /\n", "a", "/x", false},
		{"an unknown line does not end a group", "User-agent: a\nDisallow: /x\nSitemap: https://example.com/sitemap.xml\nDisallow: /y\n", "a", "/y", false},
		{"a rule before any User-agent is ignored", "Disallow: /\nUser-agent: *\nDisallow: /x\n", "a", "/y", true},
		{"only rules before any User-agent", "Disallow: /\n", "a", "/y", true},
	})
}

func TestRobotsLines(t *testing.T) {
	runRobotsCases(t, []robotsCase{
		{"keys ignore letter case", "USER-AGENT: *\nDISALLOW: /x\n", "a", "/x", false},
		{"spaces around the colon and the value", "  User-agent :  *  \n\tDisallow\t:\t/x\t\n", "a", "/x", false},
		{"no space after the colon", "User-agent:*\nDisallow:/x\n", "a", "/x", false},
		{"CRLF line breaks", "User-agent: *\r\nDisallow: /x\r\n", "a", "/x", false},
		{"lone CR line breaks", "User-agent: *\rDisallow: /x\r", "a", "/x", false},
		{"no final line break", "User-agent: *\nDisallow: /x", "a", "/x", false},
		{"a comment after a rule", "User-agent: *\nDisallow: /x # private\n", "a", "/x", false},
		{"a comment is not part of the pattern", "User-agent: *\nDisallow: /x# private\n", "a", "/x", false},
		{"a commented rule", "User-agent: *\n# Disallow: /x\n", "a", "/x", true},
		{"a misspelled key is ignored", "User-agent: *\nDisalow: /x\n", "a", "/x", true},
		{"a line without colon is ignored", "User-agent: *\nDisallow /x\nDisallow: /y\n", "a", "/x", true},
		{"a line without colon does not hide the next one", "User-agent: *\nDisallow /x\nDisallow: /y\n", "a", "/y", false},
		{"an empty Disallow allows everything", "User-agent: *\nDisallow:\n", "a", "/x", true},
		{"an empty Allow adds nothing", "User-agent: *\nAllow:\nDisallow: /\n", "a", "/x", false},
		{"Sitemap with a URL is ignored", "Sitemap: https://example.com/sitemap.xml\nUser-agent: *\nDisallow: /x\n", "a", "/x", false},
		{"a byte order mark is skipped", "\xEF\xBB\xBFUser-agent: *\nDisallow: /\n", "a", "/x", false},
		{"a pattern that does not begin with / matches nothing", "User-agent: *\nDisallow: x\n", "a", "/x", true},
		{"a pattern that begins with *", "User-agent: *\nDisallow: *x\n", "a", "/ax", false},
		{"$ in the middle is a character", "User-agent: *\nDisallow: /a$b\n", "a", "/a$b.html", false},
		{"$ in the middle does not anchor", "User-agent: *\nDisallow: /a$b\n", "a", "/a", true},
		{"$$ at the end: the first is a character", "User-agent: *\nDisallow: /a$$\n", "a", "/a$", false},
		{"$ alone matches only the empty path", "User-agent: *\nDisallow: $\n", "a", "/", true},
		{"several *", "User-agent: *\nDisallow: /*a*b*c$\n", "a", "/xaybzc", false},
		{"several * that do not match", "User-agent: *\nDisallow: /*a*b*c$\n", "a", "/xcybza", true},
		{"paths are case-sensitive", "User-agent: *\nDisallow: /Private\n", "a", "/private", true},
	})
}

func TestRobotsPercentEncoding(t *testing.T) {
	runRobotsCases(t, []robotsCase{
		{"a pattern with an accent matches the encoded path", "User-agent: *\nDisallow: /città\n", "a", "/citt%C3%A0.html", false},
		{"lowercase hexadecimal digits in the pattern", "User-agent: *\nDisallow: /citt%c3%a0\n", "a", "/citt%C3%A0.html", false},
		{"lowercase hexadecimal digits in the path", "User-agent: *\nDisallow: /citt%C3%A0\n", "a", "/citt%c3%a0.html", false},
		{"an encoded unreserved character in the pattern", "User-agent: *\nDisallow: /%62az\n", "a", "/baz", false},
		{"an encoded unreserved character in the path", "User-agent: *\nDisallow: /baz\n", "a", "/%62az", false},
		{"an encoded / is not a /", "User-agent: *\nDisallow: /a%2Fb\n", "a", "/a/b", true},
		{"an encoded / matches itself", "User-agent: *\nDisallow: /a%2fb\n", "a", "/a%2Fb", false},
		{"an encoded * is not a wildcard", "User-agent: *\nDisallow: /a%2Ab\n", "a", "/axb", true},
		{"an encoded space", "User-agent: *\nDisallow: /my page\n", "a", "/my%20page.html", false},
		{"a lone % in the pattern", "User-agent: *\nDisallow: /100%\n", "a", "/100%25.html", false},
		{"a % before one hexadecimal digit", "User-agent: *\nDisallow: /a%4\n", "a", "/a%254", false},
	})
}

func TestRobotsUTF16(t *testing.T) {
	for _, src := range []string{"\xFF\xFEU\x00", "\xFE\xFF\x00U"} {
		_, err := ParseRobots("static/robots.txt", []byte(src), "*")
		want := "static/robots.txt: the file is saved as UTF-16; please save it as UTF-8"
		if err == nil || err.Error() != want {
			t.Errorf("ParseRobots(%q): error %v, want %q", src, err, want)
		}
	}
}

func TestNormalizePercentEncoding(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"", ""},
		{"/plain/path.html", "/plain/path.html"},
		{"/città", "/citt%C3%A0"},
		{"/citt%c3%a0", "/citt%C3%A0"},
		{"/%62%61%7A", "/baz"},
		{"/%7e%2D%2e%5F", "/~-._"},
		{"/a%2fb", "/a%2Fb"},
		{"/a b", "/a%20b"},
		{"/a\tb\x7F", "/a%09b%7F"},
		{"%", "%25"},
		{"%4", "%254"},
		{"%zz", "%25zz"},
		{"%%41", "%25A"},
		{"/*.php$", "/*.php$"},
		{"/a%2A$", "/a%2A$"},
		{"/\xff", "/%FF"},
	}
	for _, test := range tests {
		got := normalizePercentEncoding(test.text)
		if got != test.want {
			t.Errorf("normalizePercentEncoding(%q) = %q, want %q", test.text, got, test.want)
		}
		if again := normalizePercentEncoding(got); again != got {
			t.Errorf("normalizing %q twice gives %q", got, again)
		}
	}
}

func TestMatchesWhole(t *testing.T) {
	tests := []struct {
		pattern string
		text    string
		want    bool
	}{
		{"", "", true},
		{"", "a", false},
		{"*", "", true},
		{"*", "abc", true},
		{"a", "a", true},
		{"a", "ab", false},
		{"ab", "a", false},
		{"a*", "a", true},
		{"*a", "ba", true},
		{"*a", "ab", false},
		{"a*c", "abbbc", true},
		{"a*c", "abbbcd", false},
		{"a*b*c", "aXbYbZc", true},
		{"a**c", "ac", true},
		{"*ab*", "aab", true},
		{"*aab", "aaab", true},
	}
	for _, test := range tests {
		if got := matchesWhole(test.pattern, test.text); got != test.want {
			t.Errorf("matchesWhole(%q, %q) = %v, want %v", test.pattern, test.text, got, test.want)
		}
	}
}
