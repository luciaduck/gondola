package frontmatter

import (
	"reflect"
	"testing"
	"time"
)

// variablesOf splits a file made of the front matter lines in block and a
// short body, and returns its variables.
func variablesOf(t *testing.T, block string) map[string]any {
	t.Helper()
	document, err := Split(testFile, []byte("---\n"+block+"---\nbody\n"))
	if err != nil {
		t.Fatal(err)
	}
	return document.Variables
}

func TestVariables(t *testing.T) {
	may1 := time.Date(2024, time.May, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		block string
		want  map[string]any
	}{
		{"names are converted", "og-image: /a.png\nexclude_from_fleet: true\n", map[string]any{"ogImage": "/a.png", "excludeFromFleet": true}},
		{"user values keep their types", "count: 3\nratio: 0.5\ndraft: false\nlist: [a, 2]\n", map[string]any{"count": 3, "ratio": 0.5, "draft": false, "list": []any{"a", 2}}},
		{"real values are kept", "count: 0\ndraft: false\nlist: []\nsettings: {}\n", map[string]any{"count": 0, "draft": false, "list": []any{}, "settings": map[string]any{}}},
		{"nested map", "social:\n  first-name: Lucia\n  links: {site: x}\n", map[string]any{"social": map[string]any{"first-name": "Lucia", "links": map[string]any{"site": "x"}}}},
		{"nested number keys become text", "counts: {1: one, 2.50: two}\n", map[string]any{"counts": map[string]any{"1": "one", "2.50": "two"}}},
		{"unquoted date under a user key", "updated: 2024-05-01\n", map[string]any{"updated": may1}},
		{"empty values", "a:\nb: ~\nc: null\nd: \"\"\ne: ''\nf: !x\n", map[string]any{"a": "", "b": "", "c": "", "d": "", "e": "", "f": ""}},
		{"empty special keys", "title:\ntags: ~\ndate: \"\"\nsitemap:\nlayout: ''\n", map[string]any{"title": "", "tags": "", "date": "", "sitemap": "", "layout": ""}},
		{"text kept as written", "title: 0.10\nauthor: 2024\npermalink: /a/\nlayout: nil\n", map[string]any{"title": "0.10", "author": "2024", "permalink": "/a/", "layout": "nil"}},
		{"tag on text is ignored", "title: !fancy Home\n", map[string]any{"title": "Home"}},
		{"permalink choice", "permalink: slug\n", map[string]any{"permalink": "slug"}},
		{"custom permalink kept as written", "permalink: /Città/My page.html\n", map[string]any{"permalink": "/Città/My page.html"}},
		{"custom permalink to be cleaned", "permalink: //a/./b/\n", map[string]any{"permalink": "//a/./b/"}},
		{"multi-line title", "title: |\n  Two\n  lines\n", map[string]any{"title": "Two\nlines\n"}},
		{"alias", "base: &t Home\ntitle: *t\n", map[string]any{"base": "Home", "title": "Home"}},
		{"booleans", "sitemap: false\nexcludeFromFleet: True\n", map[string]any{"sitemap": false, "excludeFromFleet": true}},
		{"one tag", "tags: news\n", map[string]any{"tags": []string{"news"}}},
		{"tags are trimmed and keep their case", "tags: [\" Machine Learning \", web-dev, Fleet, 2024]\n", map[string]any{"tags": []string{"Machine Learning", "web-dev", "Fleet", "2024"}}},
		{"no tags", "tags: []\n", map[string]any{"tags": []string{}}},
		{"date", "date: 2024-05-01\n", map[string]any{"date": may1}},
		{"quoted date", "date: \"2024-05-01\"\n", map[string]any{"date": may1}},
		{"short date fields", "date: 2024-5-1\n", map[string]any{"date": may1}},
		{"date and time in UTC", "date: 2024-05-01T10:30:00Z\n", map[string]any{"date": time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)}},
		{"date and time without zone", "date: 2024-05-01 10:30:00\n", map[string]any{"date": time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)}},
		{"lower-case t", "date: 2024-05-01t10:30:00Z\n", map[string]any{"date": time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)}},
		{"date with fractions of a second", "date: 2024-05-01T10:30:00.5Z\n", map[string]any{"date": time.Date(2024, time.May, 1, 10, 30, 0, 500000000, time.UTC)}},
		{"Unicode key", "città: Lindos\n", map[string]any{"città": "Lindos"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := variablesOf(t, test.block)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got  %#v\nwant %#v", got, test.want)
			}
		})
	}
}

func TestDateWithOffset(t *testing.T) {
	// The offset is kept, in a zone without a name, so formatting does not
	// depend on the computer's zone (see yamlnode.PortableTime).
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	time.Local = time.FixedZone("CEST", 2*60*60)

	for _, block := range []string{"date: 2024-05-01T10:30:00+02:00\n", "date: \"2024-05-01T10:30:00+02:00\"\n"} {
		date := variablesOf(t, block)["date"].(time.Time)
		if formatted := date.Format(time.RFC1123Z + " MST"); formatted != "Wed, 01 May 2024 10:30:00 +0200 +0200" {
			t.Errorf("%q formatted as %q", block, formatted)
		}
	}
}

func TestVariablesErrors(t *testing.T) {
	// Lines count the whole file: the opening "---" is line 1, so the first
	// key of the block is on line 2.
	tests := []struct {
		name  string
		block string
		want  string
	}{
		// Shape of the block.
		{"list", "- a\n- b\n", `2:1: front matter must be a map of keys, not a list`},
		{"text", "hello\n", `2:1: front matter must be a map of keys, not "hello"`},
		{"invalid YAML", "title: Home\n  bad: indent\n", `3:1: invalid YAML: mapping values are not allowed in this context`},
		{"invalid YAML without a column", "title: [\n", `2:1: invalid YAML: did not find expected node content`},
		{"tab indentation", "a:\n\tb: 1\n", `3:1: invalid YAML: found character that cannot start any token`},
		{"second document", "a: 1\n...\n--- x\n", `4:1: a second YAML document starts here; front matter must hold only one, so please remove it`},
		// Keys.
		{"key twice", "title: a\nauthor: b\ntitle: c\n", `4:1: front matter key "title" appears twice (first at line 2); please keep only one`},
		{"keys that become the same name", "og-image: a\nogImage: b\n", `3:1: front matter keys "og-image" and "ogImage" both become "ogImage" in templates; please rename one`},
		{"list as key", "? [a]\n: 1\n", `2:3: keys in front matter must be names, not a list`},
		{"empty word", "og--image: a\n", `2:1: front matter key "og--image" has an empty word; please remove the extra "-" or "_"`},
		{"leading underscore", "_draft: true\n", `2:1: front matter key "_draft" has an empty word; please remove the extra "-" or "_"`},
		{"number key", "2024: a\n", `2:1: front matter key "2024" becomes "2024" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"invalid character", "og.image: a\n", `2:1: front matter key "og.image" becomes "og.image" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"Jet keyword", "range: 1\n", `2:1: front matter key "range" is a Jet keyword and cannot be a template variable; please rename it`},
		{"empty key", "\"\": 1\n", `2:1: front matter key "" is empty; please give it a name`},
		// Reserved names (SPEC §4).
		{"reserved site", "title: a\nsite: b\n", `3:1: front matter key "site" is reserved by Gondola (templates use it for the site settings); please rename it`},
		{"reserved copy", "copy: x\n", `2:1: front matter key "copy" is reserved by Gondola (templates use it for the page's HTML); please rename it`},
		{"reserved data", "data: x\n", `2:1: front matter key "data" is reserved by Gondola (templates use it for the data files); please rename it`},
		{"reserved fleet", "fleet: x\n", `2:1: front matter key "fleet" is reserved by Gondola (templates use it for the collection of all pages); please rename it`},
		{"reserved collections", "collections: x\n", `2:1: front matter key "collections" is reserved by Gondola (templates use it for the collections of tagged pages); please rename it`},
		{"reserved page", "page: x\n", `2:1: front matter key "page" is reserved by Gondola (templates use it for the page's URL and metadata); please rename it`},
		{"becomes reserved", "Site: x\n", `2:1: front matter key "Site" becomes "site" in templates, which is reserved by Gondola (templates use it for the site settings); please rename it`},
		{"Jet function", "raw: true\n", `2:1: front matter key "raw" is the name of a Jet function and cannot be a template variable; please rename it`},
		{"becomes a Jet function", "Raw: 1\n", `2:1: front matter key "Raw" becomes "raw" in templates, which is the name of a Jet function; please rename it`},
		{"reserved even when empty", "page:\n", `2:1: front matter key "page" is reserved by Gondola (templates use it for the page's URL and metadata); please rename it`},
		// Text keys.
		{"title list", "title: [a]\n", `2:8: front matter key "title" must be text, not a list`},
		{"author map", "author: {name: Lucia}\n", `2:9: front matter key "author" must be text, not a map`},
		{"permalink list", "permalink:\n  - /a/\n", `3:3: front matter key "permalink" must be text, not a list`},
		{"layout map", "layout: {a: 1}\n", `2:9: front matter key "layout" must be text, not a map`},
		// Layout names (SPEC §5.11).
		{"layout with dot dot", "layout: ../base\n", `2:9: layout "../base" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"layout absolute", "layout: /base\n", `2:9: layout "/base" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"layout named as written", "Layout: ./base\n", `2:9: Layout "./base" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"layout with backslash", "layout: a\\b\n", `2:9: layout "a\\b" contains "\\"; please separate directories with "/"`},
		// Permalinks (SPEC §5.7).
		{"permalink without slash", "permalink: about-us/\n", `2:12: permalink "about-us/" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"permalink choice in capitals", "permalink: Slug\n", `2:12: permalink "Slug" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"permalink named as written", "Permalink: x\n", `2:12: Permalink "x" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"permalink number", "permalink: 2024\n", `2:12: permalink "2024" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"permalink with dot dot", "permalink: /a/../b/\n", `2:12: permalink "/a/../b/" is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"permalink with backslash", "permalink: /a\\b.html\n", `2:12: permalink "/a\\b.html" contains "\\"; please separate directories with "/"`},
		{"permalink with colon", "permalink: \"/a:b.html\"\n", `2:12: permalink "/a:b.html" contains ":"; Windows does not allow it in file names; please remove it`},
		{"permalink with query", "permalink: /feed?x=1\n", `2:12: permalink "/feed?x=1" contains "?"; a permalink names a file in the output, so it cannot have a query (the part after "?")`},
		// In double quotes, \x8d is a YAML escape for U+008D, which is UTF-8.
		{"permalink with YAML escape", "permalink: \"/a\\x8d\"\n", `2:12: permalink "/a\u008d" contains a control character (U+008D); please remove it`},
		{"permalink with tab", "permalink: \"/a\\tb/\"\n", `2:12: permalink "/a\tb/" contains a control character (U+0009); please remove it`},
		// Booleans.
		{"sitemap yes", "sitemap: yes\n", `2:10: front matter key "sitemap" must be true or false, not "yes"`},
		{"sitemap quoted", "sitemap: \"true\"\n", `2:10: front matter key "sitemap" must be true or false, not "true"`},
		{"excludeFromFleet as written", "exclude-from-fleet: 1\n", `2:21: front matter key "exclude-from-fleet" must be true or false, not "1"`},
		{"tagged boolean", "sitemap: !x true\n", `2:10: front matter key "sitemap" must be true or false, not "true"`},
		// Tags.
		{"tags map", "tags: {a: 1}\n", `2:7: front matter key "tags" must be a tag or a list of tags, not a map`},
		{"tag list in list", "tags: [a, [b]]\n", `2:11: each entry of front matter key "tags" must be a tag, not a list`},
		{"empty tag", "tags: [a, \"\"]\n", `2:11: front matter key "tags" has an empty tag; please remove it`},
		{"blank tag", "tags: \"  \"\n", `2:7: front matter key "tags" has an empty tag; please remove it`},
		{"null tag", "tags:\n  - a\n  - ~\n", `4:5: front matter key "tags" has an empty tag; please remove it`},
		{"fleet tag", "tags: [news, fleet]\n", `2:14: tag "fleet" is reserved by Gondola (templates use fleet for the collection of all pages); please rename it`},
		{"fleet tag after trimming", "tags: \" fleet\"\n", `2:7: tag "fleet" is reserved by Gondola (templates use fleet for the collection of all pages); please rename it`},
		{"tag twice", "tags: [news, a, news]\n", `2:17: front matter key "tags" lists the tag "news" twice; please keep only one`},
		{"tag twice after trimming", "tags: [news, \"news \"]\n", `2:14: front matter key "tags" lists the tag "news" twice; please keep only one`},
		// Dates.
		{"date text", "date: next tuesday\n", `2:7: date "next tuesday" is not a date; please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z`},
		{"date as written", "Date: 2024-13-01\n", `2:7: Date "2024-13-01" is not a date; please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z`},
		{"date number", "date: 2024\n", `2:7: date "2024" is not a date; please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z`},
		{"date with T and no zone", "date: 2024-05-01T10:30:00\n", `2:7: date "2024-05-01T10:30:00" is not a date; please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z`},
		{"date list", "date: [2024-05-01]\n", `2:7: front matter key "date" must be a date, not a list`},
		// Nested values.
		{"nested key twice", "social: {name: a, name: b}\n", `2:19: key "name" appears twice in front matter key "social" (first at line 2); please keep only one`},
		{"nested key twice on two lines", "social:\n  name: a\n  name: b\n", `4:3: key "name" appears twice in front matter key "social" (first at line 3); please keep only one`},
		{"nested list as key", "social: {[a]: 1}\n", `2:10: keys in front matter key "social" must be text, not a list`},
		{"wrong explicit tag", "count: !!int many\n", "2:8: invalid YAML: cannot decode !!str `many` as a !!int"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Split(testFile, []byte("---\n"+test.block+"---\nbody\n"))
			want := testFile + ":" + test.want
			if err == nil || err.Error() != want {
				t.Errorf("error:\n got %v\nwant %s", err, want)
			}
		})
	}
}

func TestVariablesErrorPositions(t *testing.T) {
	// Positions are converted to the console's way of counting: lines
	// split at "\n", columns in bytes.
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"CRLF", "---\r\ntitle: a\r\nsitemap: yes\r\n---\r\nbody\r\n", `3:10: front matter key "sitemap" must be true or false, not "yes"`},
		{"accents count as bytes", "---\ncittà: {è: 1, è: 2}\n---\n", `2:17: key "è" appears twice in front matter key "città" (first at line 2); please keep only one`},
		// yaml.v3 starts a new line at a lone "\r"; the console does not.
		{"lone CR", "---\na: 1\rsitemap: yes\n---\n", `2:15: front matter key "sitemap" must be true or false, not "yes"`},
		{"line separator", "---\na: 1\u2028title: [x]\n---\n", `2:15: front matter key "title" must be text, not a list`},
		{"after a byte order mark", byteOrderMark + "---\nsitemap: yes\n---\n", `2:10: front matter key "sitemap" must be true or false, not "yes"`},
		{"first line of a repeated key after a lone CR", "---\na: 1\rb: 2\ntitle: x\ntitle: y\n---\n", `4:1: front matter key "title" appears twice (first at line 3); please keep only one`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Split(testFile, []byte(test.src))
			want := testFile + ":" + test.want
			if err == nil || err.Error() != want {
				t.Errorf("error:\n got %v\nwant %s", err, want)
			}
		})
	}
}

func TestVariablesNotUTF8(t *testing.T) {
	// yaml.v3 refuses a byte that is not UTF-8 before any value is read, so
	// CheckPermalink never sees one from front matter. Its message has no
	// line.
	_, err := Split(testFile, []byte("---\npermalink: /a\x8d\n---\n"))
	want := testFile + ": invalid YAML: invalid leading UTF-8 octet"
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}
