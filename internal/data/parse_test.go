package data

import (
	"reflect"
	"testing"
	"time"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		src        string
		want       any
	}{
		// YAML.
		{"yaml map", "_data/site.yaml", "name: Lindos\nfounded: 1999\n", map[string]any{"name": "Lindos", "founded": 1999}},
		{"yaml keys kept as written", "_data/site.yaml", "first-name: Ada\nog_image: /a.png\n1: one\n",
			map[string]any{"first-name": "Ada", "og_image": "/a.png", "1": "one"}},
		{"yaml nested", "_data/site.yaml", "social:\n  first-name: Ada\n  links: [a, b]\n",
			map[string]any{"social": map[string]any{"first-name": "Ada", "links": []any{"a", "b"}}}},
		{"yaml list", "_data/nav.yml", "- home\n- about\n", []any{"home", "about"}},
		{"yaml scalar", "_data/motto.yaml", "Sail on\n", "Sail on"},
		{"yaml nested null stays nil", "_data/site.yaml", "a: ~\nb:\n", map[string]any{"a": nil, "b": nil}},
		{"yaml date is UTC", "_data/site.yaml", "since: 2024-05-01\n", map[string]any{"since": time.Date(2024, time.May, 1, 0, 0, 0, 0, time.UTC)}},
		{"yaml CRLF", "_data/site.yaml", "a: 1\r\nb: 2\r\n", map[string]any{"a": 1, "b": 2}},
		{"yaml byte order mark", "_data/site.yaml", "\xEF\xBB\xBFa: 1\n", map[string]any{"a": 1}},
		{"yaml extension in capitals", "_data/SITE.YML", "a: 1\n", map[string]any{"a": 1}},

		// An empty YAML file is a placeholder.
		{"yaml empty file", "_data/x.yaml", "", ""},
		{"yaml only spaces and lines", "_data/x.yaml", "  \n\n", ""},
		{"yaml only comments", "_data/x.yaml", "# to do\n", ""},
		{"yaml only null", "_data/x.yaml", "null\n", ""},
		{"yaml only tilde", "_data/x.yaml", "~\n", ""},
		{"yaml only empty string", "_data/x.yaml", "\"\"\n", ""},
		{"yaml only a tag", "_data/x.yaml", "!x\n", ""},
		{"yaml only a document marker", "_data/x.yaml", "---\n", ""},
		{"yaml only a byte order mark", "_data/x.yaml", "\xEF\xBB\xBF", ""},

		// JSON.
		{"json object", "_data/site.json", `{"name": "Lindos", "first-name": "Ada"}`, map[string]any{"name": "Lindos", "first-name": "Ada"}},
		{"json nested", "_data/site.json", `{"social": {"links": ["a", "b"], "on": true, "off": null}}`,
			map[string]any{"social": map[string]any{"links": []any{"a", "b"}, "on": true, "off": nil}}},
		{"json list", "_data/nav.json", `["home", "about"]`, []any{"home", "about"}},
		{"json string", "_data/motto.json", `"Sail on"`, "Sail on"},
		{"json number", "_data/count.json", `42`, 42},
		{"json whole numbers are int", "_data/n.json", `{"a": 1, "b": -3, "c": 0, "d": -0, "e": 1000000}`,
			map[string]any{"a": 1, "b": -3, "c": 0, "d": 0, "e": 1000000}},
		{"json other numbers are float64", "_data/n.json", `{"a": 1.5, "b": 1.0, "c": 1e6, "d": 2E-3, "e": 99999999999999999999}`,
			map[string]any{"a": 1.5, "b": 1.0, "c": 1e6, "d": 2e-3, "e": 1e20}},
		{"json numbers in lists", "_data/n.json", `[1, [2.5, {"x": 3}]]`, []any{1, []any{2.5, map[string]any{"x": 3}}}},
		{"json key twice keeps the last", "_data/site.json", `{"a": 1, "a": 2}`, map[string]any{"a": 2}},
		{"json byte order mark", "_data/site.json", "\xEF\xBB\xBF{\"a\": 1}", map[string]any{"a": 1}},
		{"json CRLF", "_data/site.json", "{\r\n  \"a\": 1\r\n}\r\n", map[string]any{"a": 1}},
		{"json extension in capitals", "_data/SITE.JSON", `{"a": 1}`, map[string]any{"a": 1}},
		{"json escapes", "_data/s.json", `"caf\u00e9 \ud83d\ude00"`, "caf\xc3\xa9 \xf0\x9f\x98\x80"},

		// An empty JSON file is a placeholder too.
		{"json empty file", "_data/x.json", "", ""},
		{"json only whitespace", "_data/x.json", " \t\r\n", ""},
		{"json only null", "_data/x.json", "null\n", ""},
		{"json only a byte order mark", "_data/x.json", "\xEF\xBB\xBF", ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parse, ok := parserFor(test.sourcePath)
			if !ok {
				t.Fatalf("parserFor(%q) found no parser", test.sourcePath)
			}
			got, err := parseFile(test.sourcePath, []byte(test.src), parse)
			if err != nil {
				t.Fatalf("parseFile returned error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestParseFileErrors(t *testing.T) {
	tests := []struct {
		name       string
		sourcePath string
		src        string
		want       string
	}{
		// YAML, with the messages of front matter.
		{"yaml key twice", "_data/site.yaml", "name: a\nother: b\nname: c\n",
			`_data/site.yaml:3:1: key "name" appears twice in the data file (first at line 1); please keep only one`},
		{"yaml nested key twice", "_data/site.yaml", "social:\n  a: 1\n  a: 2\n",
			`_data/site.yaml:3:3: key "a" appears twice in the data file (first at line 2); please keep only one`},
		{"yaml list as key", "_data/site.yaml", "? [a]\n: 1\n", `_data/site.yaml:1:3: keys in the data file must be text, not a list`},
		{"yaml second document", "_data/site.yaml", "a: 1\n---\nb: 2\n",
			`_data/site.yaml:2:1: a second YAML document starts here; a data file must hold only one, so please remove it`},
		{"yaml syntax", "_data/site.yaml", "a: 1\nb: [\n", `_data/site.yaml:2:1: invalid YAML: did not find expected node content`},
		{"yaml tab indentation", "_data/site.yaml", "a:\n\tb: 1\n", `_data/site.yaml:2:1: invalid YAML: found character that cannot start any token`},
		{"yaml after byte order mark", "_data/site.yaml", "\xEF\xBB\xBFa: 1\na: 2\n",
			`_data/site.yaml:2:1: key "a" appears twice in the data file (first at line 1); please keep only one`},
		{"yaml in a subdirectory", "_data/blog/authors.yml", "x: [\n", `_data/blog/authors.yml:1:1: invalid YAML: did not find expected node content`},
		{"yaml UTF-16", "_data/site.yaml", "\xFF\xFEa\x00", `_data/site.yaml: the file is saved as UTF-16; please save it as UTF-8`},

		// JSON. The column is that of the offending character.
		{"json invalid value", "_data/site.json", `{"a": x}`, `_data/site.json:1:7: invalid JSON: invalid character 'x' looking for beginning of value`},
		{"json trailing comma", "_data/site.json", "{\n  \"a\": 1,\n}\n",
			`_data/site.json:3:1: invalid JSON: invalid character '}' looking for beginning of object key string`},
		{"json trailing comma in a list", "_data/nav.json", `[1, 2,]`, `_data/nav.json:1:7: invalid JSON: invalid character ']' looking for beginning of value`},
		{"json missing colon", "_data/site.json", `{"a" 1}`, `_data/site.json:1:6: invalid JSON: invalid character '1' after object key`},
		{"json bad escape", "_data/site.json", `{"a": "\x01"}`, "_data/site.json:1:9: invalid JSON: invalid escape sequence `\\x` in string"},
		{"json single quotes", "_data/site.json", `{'a': 1}`, `_data/site.json:1:2: invalid JSON: invalid character '\'' looking for beginning of object key string`},
		{"json comment", "_data/site.json", "{\n  // name\n  \"a\": 1\n}", `_data/site.json:2:3: invalid JSON: invalid character '/' looking for beginning of object key string`},
		{"json two values", "_data/site.json", "{\"a\": 1}\n{\"b\": 2}\n", `_data/site.json:2:1: invalid JSON: invalid character '{' after top-level value`},
		{"json extra brace", "_data/site.json", `{"a":1}}`, `_data/site.json:1:8: invalid JSON: invalid character '}' after top-level value`},
		{"json multibyte character", "_data/site.json", "{\"a\": \xc3\xa9}", `_data/site.json:1:7: invalid JSON: invalid character '` + "\xc3\xa9" + `' looking for beginning of value`},
		{"json after byte order mark", "_data/site.json", "\xEF\xBB\xBF{x}", `_data/site.json:1:2: invalid JSON: invalid character 'x' looking for beginning of object key string`},
		{"json yaml written in a json file", "_data/site.json", "name: Lindos\n", `_data/site.json:1:2: invalid JSON: invalid character 'a' in literal null (expecting 'u')`},
		{"json only a space that is not JSON whitespace", "_data/x.json", "\xc2\xa0", `_data/x.json:1:1: invalid JSON: invalid character '\u00a0' looking for beginning of value`},

		// A file cut short: the error points at its last character.
		{"json cut short", "_data/site.json", `{"a": 1`, `_data/site.json:1:7: invalid JSON: unexpected end of JSON input`},
		{"json cut short before a line break", "_data/site.json", "{\"a\": 1\n", `_data/site.json:1:8: invalid JSON: unexpected end of JSON input`},
		{"json cut short in a string", "_data/site.json", "{\n\"a\": \"b", `_data/site.json:2:7: invalid JSON: unexpected end of JSON input`},

		// Numbers too large for a float64 point at the start of the number.
		{"json number out of range", "_data/n.json", `{"a": 1e999}`, `_data/n.json:1:7: invalid JSON: number 1e999 is out of range`},
		{"json negative number out of range", "_data/n.json", "[1,\n -1e400, 2]", `_data/n.json:2:2: invalid JSON: number -1e400 is out of range`},

		{"json not UTF-8", "_data/site.json", "{\"a\": \"caf\xe9\"}", `_data/site.json: the file is not valid UTF-8; please save it as UTF-8`},
		{"json UTF-16", "_data/site.json", "\xFE\xFF\x00{", `_data/site.json: the file is saved as UTF-16; please save it as UTF-8`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parse, ok := parserFor(test.sourcePath)
			if !ok {
				t.Fatalf("parserFor(%q) found no parser", test.sourcePath)
			}
			_, err := parseFile(test.sourcePath, []byte(test.src), parse)
			if err == nil {
				t.Fatal("parseFile returned no error")
			}
			if got := err.Error(); got != test.want {
				t.Errorf("error = %s\n         want %s", got, test.want)
			}
		})
	}
}

func TestParserFor(t *testing.T) {
	tests := []struct {
		name       string
		isDataFile bool
	}{
		{"a.json", true},
		{"a.yaml", true},
		{"a.yml", true},
		{"A.JSON", true},
		{"a.Yaml", true},
		{"a.b.yml", true},
		{"a.toml", false},
		{"a.jet", false},
		{"a.html", false},
		{"README.md", false},
		{"a.json.bak", false},
		{"json", false},
		{"a", false},
	}
	for _, test := range tests {
		if _, got := parserFor(test.name); got != test.isDataFile {
			t.Errorf("parserFor(%q) found a parser: %v, want %v", test.name, got, test.isDataFile)
		}
	}
}
