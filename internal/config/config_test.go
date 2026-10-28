package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/diag"
)

// minimal is the smallest valid gondola.yaml: the three required keys.
const minimal = `config:
  site-name: My site
  version: "0.1.0"
  canonical-url: https://example.com
`

// withMinimal returns minimal followed by more, which may continue the
// config: section (indented) or add other sections. Lines 1 to 4 are
// minimal's, so more starts on line 5.
func withMinimal(more string) string {
	return minimal + more
}

// defaultConfig is what Parse returns for minimal.
func defaultConfig() Config {
	return Config{
		Settings: Settings{
			SiteName:     "My site",
			SiteVersion:  "0.1.0",
			CanonicalURL: "https://example.com",
			Description:  "",
			Sass:         true,
			FencedBlocks: false,
			Markdown:     true,
			PluginPaths:  []string{},
			Engine:       EngineJet,
			Sitemap:      false,
			RobotsAgent:  "*",
			DateFormat:   "2006-01-02",
			Port:         1094,
		},
		Input: Input{
			ContentDir:   ".",
			TemplateDirs: DirList{Dirs: []string{"_components/includes"}, FromDefault: true},
			DataDirs:     DirList{Dirs: []string{"_data"}, FromDefault: true},
			// The defaults have no position.
			TemplateDirPositions: []Position{{}},
			DataDirPositions:     []Position{{}},
			PartialDirs:          DirList{Dirs: []string{"_components/sass"}, FromDefault: true},
		},
		Output: Output{
			Routing:             RoutingFile,
			Dir:                 "_site",
			PassthroughPatterns: []PassthroughPattern{},
		},
		Defaults: Defaults{
			Permalink: PermalinkPath,
			Variables: map[string]any{
				"layout":    "nil",
				"title":     "",
				"author":    "",
				"date":      "",
				"permalink": "path",
			},
		},
	}
}

func mustParse(t *testing.T, src string) Config {
	t.Helper()
	got, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse returned error: %v\nsource:\n%s", err, src)
	}
	return got
}

func checkConfig(t *testing.T, got, want Config) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config differs\n got %+v\nwant %+v", got, want)
	}
}

func TestParseMinimal(t *testing.T) {
	checkConfig(t, mustParse(t, minimal), defaultConfig())
}

func TestParseEveryKey(t *testing.T) {
	src := `config:
  site-name: Lindos
  version: "2.3.1"
  canonical-url: https://lindos.example.org/
  description: Notes from the island
  sass: false
  fenced-blocks: true
  markdown: true
  plugin: [scripts/minify.sh, tools/check]
  engine: jet
  sitemap: true
  robots-agent: Googlebot
  date-format: "2 January 2006"
  port: 8080

input:
  content: src
  templates: [src/_layouts, shared/templates]
  data: shared/data
  partials: [styles/partials]

output:
  routing: directory
  directory: public
  passthrough: [/robots.txt, .well-known/, "*.ico"]

default:
  layout: base
  title: Untitled
  author: Lucia
  date: "2026-01-01"
  permalink: slug
  og-image: /og.png
`
	want := Config{
		Settings: Settings{
			SiteName:     "Lindos",
			SiteVersion:  "2.3.1",
			CanonicalURL: "https://lindos.example.org",
			Description:  "Notes from the island",
			Sass:         false,
			FencedBlocks: true,
			Markdown:     true,
			PluginPaths:  []string{"scripts/minify.sh", "tools/check"},
			Engine:       EngineJet,
			Sitemap:      true,
			RobotsAgent:  "Googlebot",
			DateFormat:   "2 January 2006",
			Port:         8080,
		},
		Input: Input{
			ContentDir:           "src",
			ContentDirPosition:   Position{Line: 17, Col: 12},
			TemplateDirs:         DirList{Dirs: []string{"src/_layouts", "shared/templates"}},
			TemplateDirPositions: []Position{{Line: 18, Col: 15}, {Line: 18, Col: 29}},
			DataDirs:             DirList{Dirs: []string{"shared/data"}},
			DataDirPositions:     []Position{{Line: 19, Col: 9}},
			PartialDirs:          DirList{Dirs: []string{"styles/partials"}},
		},
		Output: Output{
			Routing: RoutingDirectory,
			Dir:     "public",
			PassthroughPatterns: []PassthroughPattern{
				{Pattern: "/robots.txt", Position: Position{Line: 25, Col: 17}},
				{Pattern: ".well-known/", Position: Position{Line: 25, Col: 30}},
				{Pattern: "*.ico", Position: Position{Line: 25, Col: 44}},
			},
		},
		Defaults: Defaults{
			Permalink: PermalinkSlug,
			Variables: map[string]any{
				"layout": "base",
				"title":  "Untitled",
				"author": "Lucia",
				// A quoted date is a date too, as in front matter.
				"date":      time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
				"permalink": "slug",
				"ogImage":   "/og.png",
			},
		},
	}
	checkConfig(t, mustParse(t, src), want)
}

func TestOutputTempDir(t *testing.T) {
	if got := (Output{Dir: "_site"}).TempDir(); got != "_site.temp" {
		t.Errorf("TempDir() = %q, want %q", got, "_site.temp")
	}
	if got := (Output{Dir: "build/public"}).TempDir(); got != "build/public.temp" {
		t.Errorf("TempDir() = %q, want %q", got, "build/public.temp")
	}
}

// emptyValues are the ways to write "use the default".
var emptyValues = []string{"", " ~", " null", ` ""`, " ''",
	// A tag does not make an empty value less empty (found by FuzzParse).
	" !custom", ` !!str ""`, " !!null"}

func TestParseEmptyMeansDefault(t *testing.T) {
	optionalKeys := map[string][]string{
		"config": {"description", "sass", "fenced-blocks", "markdown", "plugin", "engine",
			"sitemap", "robots-agent", "date-format", "port"},
		"input":   {"content", "templates", "data", "partials"},
		"output":  {"routing", "directory", "passthrough"},
		"default": {"layout", "title", "author", "date", "permalink"},
	}
	for _, section := range []string{"config", "input", "output", "default"} {
		for _, key := range optionalKeys[section] {
			for _, empty := range emptyValues {
				var src string
				if section == "config" {
					src = withMinimal("  " + key + ":" + empty + "\n")
				} else {
					src = withMinimal(section + ":\n  " + key + ":" + empty + "\n")
				}
				t.Run(section+"."+key+":"+empty, func(t *testing.T) {
					checkConfig(t, mustParse(t, src), defaultConfig())
				})
			}
		}
	}
}

func TestParseEmptySections(t *testing.T) {
	for _, section := range []string{"input", "output", "default"} {
		for _, empty := range append(emptyValues, " {}") {
			src := withMinimal(section + ":" + empty + "\n")
			t.Run(section+":"+empty, func(t *testing.T) {
				checkConfig(t, mustParse(t, src), defaultConfig())
			})
		}
	}
}

func TestParseRealValuesAreKept(t *testing.T) {
	tests := []struct {
		name   string
		more   string
		change func(*Config)
	}{
		{"false", "  sass: false\n  markdown: false\n", func(c *Config) {
			c.Settings.Sass = false
			c.Settings.Markdown = false
		}},
		{"empty template list", "input:\n  templates: []\n", func(c *Config) {
			c.Input.TemplateDirs = DirList{Dirs: []string{}}
			c.Input.TemplateDirPositions = []Position{}
		}},
		{"empty data list", "input:\n  data: []\n", func(c *Config) {
			c.Input.DataDirs = DirList{Dirs: []string{}}
			c.Input.DataDirPositions = []Position{}
		}},
		{"empty partials list", "input:\n  partials: []\n", func(c *Config) {
			c.Input.PartialDirs = DirList{Dirs: []string{}}
		}},
		{"default dir written by hand", "input:\n  data: _data\n", func(c *Config) {
			c.Input.DataDirs = DirList{Dirs: []string{"_data"}}
			c.Input.DataDirPositions = []Position{{Line: 6, Col: 9}}
		}},
		{"data dirs that share a prefix are apart", "input:\n  data: [_data, _data-extra]\n", func(c *Config) {
			c.Input.DataDirs = DirList{Dirs: []string{"_data", "_data-extra"}}
			c.Input.DataDirPositions = []Position{{Line: 6, Col: 10}, {Line: 6, Col: 17}}
		}},
		{"empty plugin list", "  plugin: []\n", func(*Config) {}},
		{"empty passthrough list", "output:\n  passthrough: []\n", func(*Config) {}},
		{"user default left empty", "default:\n  og-image:\n  subtitle: ~\n", func(c *Config) {
			c.Defaults.Variables["ogImage"] = ""
			c.Defaults.Variables["subtitle"] = ""
		}},
		{"user default zero values", "default:\n  draft: false\n  count: 0\n  tags: []\n", func(c *Config) {
			c.Defaults.Variables["draft"] = false
			c.Defaults.Variables["count"] = 0
			c.Defaults.Variables["tags"] = []string{}
		}},
		{"user default map with a number key", "default:\n  counts: {1: one, 02: two}\n", func(c *Config) {
			// Nested keys stay text as written, so templates can read the map.
			c.Defaults.Variables["counts"] = map[string]any{"1": "one", "02": "two"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := defaultConfig()
			test.change(&want)
			checkConfig(t, mustParse(t, withMinimal(test.more)), want)
		})
	}
}

func TestParseText(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		// Text is kept exactly as written, whatever YAML type it has.
		{"quoted", `"1.0"`, "1.0"},
		{"unquoted float keeps its zero", "0.10", "0.10"},
		{"integer", "3", "3"},
		{"word", "beta", "beta"},
		{"boolean-looking", "true", "true"},
		{"date-looking", "2026-10-26", "2026-10-26"},
		{"with spaces", "2.0 final", "2.0 final"},
		// A tag is ignored: the text is what counts.
		{"custom tag", "!release 2.0", "2.0"},
		{"str tag", "!!str 0.10", "0.10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			src := "config:\n  site-name: S\n  version: " + test.version + "\n  canonical-url: https://example.com\n"
			if got := mustParse(t, src).Settings.SiteVersion; got != test.want {
				t.Errorf("SiteVersion = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParseBoolAndPort(t *testing.T) {
	tests := []struct {
		more   string
		change func(*Config)
	}{
		{"  sitemap: True\n", func(c *Config) { c.Settings.Sitemap = true }},
		{"  sitemap: TRUE\n", func(c *Config) { c.Settings.Sitemap = true }},
		{"  sass: False\n", func(c *Config) { c.Settings.Sass = false }},
		{"  port: 1\n", func(c *Config) { c.Settings.Port = 1 }},
		{"  port: 65535\n", func(c *Config) { c.Settings.Port = 65535 }},
		{"  port: 0x1F90\n", func(c *Config) { c.Settings.Port = 8080 }},
		{"  port: +8080\n", func(c *Config) { c.Settings.Port = 8080 }},
	}
	for _, test := range tests {
		t.Run(strings.TrimSpace(test.more), func(t *testing.T) {
			want := defaultConfig()
			test.change(&want)
			checkConfig(t, mustParse(t, withMinimal(test.more)), want)
		})
	}
}

func TestParseCanonicalURL(t *testing.T) {
	tests := []struct {
		written string
		want    string
	}{
		{"https://example.com", "https://example.com"},
		{"https://example.com/", "https://example.com"},
		{"http://example.com/", "http://example.com"},
		{"https://example.com/blog/", "https://example.com/blog"},
		{"https://example.com/blog", "https://example.com/blog"},
		{"https://localhost:8080/", "https://localhost:8080"},
		// Kept as written, apart from the one trailing slash.
		{"HTTPS://Example.com/", "HTTPS://Example.com"},
		{"https://example.com//", "https://example.com/"},
	}
	for _, test := range tests {
		t.Run(test.written, func(t *testing.T) {
			src := "config:\n  site-name: S\n  version: 1\n  canonical-url: " + test.written + "\n"
			if got := mustParse(t, src).Settings.CanonicalURL; got != test.want {
				t.Errorf("CanonicalURL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestParsePaths(t *testing.T) {
	tests := []struct {
		name string
		more string
		want DirList
	}{
		{"one path", "input:\n  templates: layouts\n", DirList{Dirs: []string{"layouts"}}},
		{"one-item list", "input:\n  templates: [layouts]\n", DirList{Dirs: []string{"layouts"}}},
		{"list in order", "input:\n  templates:\n    - b\n    - a\n", DirList{Dirs: []string{"b", "a"}}},
		{"cleaned", "input:\n  templates: ./x//y/\n", DirList{Dirs: []string{"x/y"}}},
		{"dot inside", "input:\n  templates: x/./y\n", DirList{Dirs: []string{"x/y"}}},
		{"dots in a name are not ..", "input:\n  templates: x/..y/z...\n", DirList{Dirs: []string{"x/..y/z..."}}},
		{"unicode", "input:\n  templates: modèles\n", DirList{Dirs: []string{"modèles"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := mustParse(t, withMinimal(test.more)).Input.TemplateDirs
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("TemplateDirs = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseRoleDirectoriesInsideContent(t *testing.T) {
	// Role directories may be inside the content directory, and several
	// input roles may share one directory.
	src := withMinimal(`input:
  content: site
  templates: site/_includes
  data: [site/_includes, site/_data]
  partials: site/_includes
output:
  directory: site/_site
`)
	got := mustParse(t, src)
	if got.Input.ContentDir != "site" || got.Output.Dir != "site/_site" {
		t.Errorf("ContentDir = %q, Output.Dir = %q", got.Input.ContentDir, got.Output.Dir)
	}
	// A directory next to the output whose name only starts the same way
	// is not inside it.
	mustParse(t, withMinimal("input:\n  templates: _site-layouts\n"))
	mustParse(t, withMinimal("  plugin: [_site.sh]\n"))
}

func TestParseDefaults(t *testing.T) {
	src := withMinimal(`default:
  layout: page
  Title: Home
  og-image: /og.png
  robots_agent: none
  author: Lucia
  social:
    name: Lucia
    links: [a, b]
  città: Lindos
  tags: news
  sitemap: false
  weight: 2.5
  date: 2026-10-26
`)
	got := mustParse(t, src).Defaults
	want := Defaults{
		Permalink: PermalinkPath,
		Variables: map[string]any{
			"layout":      "page",
			"title":       "Home",
			"ogImage":     "/og.png",
			"robotsAgent": "none",
			"author":      "Lucia",
			// Nested maps are always map[string]any (see yamlnode.DecodeValue).
			"social": map[string]any{"name": "Lucia", "links": []any{"a", "b"}},
			"città":  "Lindos",
			// The special keys follow the rules of front matter: one tag
			// becomes a list.
			"tags":    []string{"news"},
			"sitemap": false,
			"weight":  2.5,
			// An unquoted date keeps its YAML type, as in frontmatter.
			"date":      time.Date(2026, time.October, 26, 0, 0, 0, 0, time.UTC),
			"permalink": "path",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Defaults differ\n got %#v\nwant %#v", got, want)
	}
}

func TestParseDefaultPermalinks(t *testing.T) {
	for _, permalink := range []Permalink{PermalinkPath, PermalinkSlug, PermalinkDate} {
		got := mustParse(t, withMinimal("default:\n  permalink: "+string(permalink)+"\n")).Defaults
		if got.Permalink != permalink || got.Variables["permalink"] != string(permalink) {
			t.Errorf("permalink %q: got %q and %#v", permalink, got.Permalink, got.Variables["permalink"])
		}
	}
}

func TestParseAliases(t *testing.T) {
	src := `config:
  site-name: &name Lindos
  version: 1
  canonical-url: https://example.com
  description: *name
  sass: &off false
  markdown: *off
input:
  templates: &shared [a, b]
  data: *shared
default:
  title: *name
`
	got := mustParse(t, src)
	if got.Settings.Description != "Lindos" || got.Settings.Markdown || got.Settings.Sass {
		t.Errorf("Settings = %+v", got.Settings)
	}
	if !reflect.DeepEqual(got.Input.DataDirs.Dirs, []string{"a", "b"}) {
		t.Errorf("DataDirs = %+v", got.Input.DataDirs)
	}
	if got.Defaults.Variables["title"] != "Lindos" {
		t.Errorf("title = %#v", got.Defaults.Variables["title"])
	}
}

func TestParseCRLF(t *testing.T) {
	src := strings.ReplaceAll(withMinimal("  port: 8080\n"), "\n", "\r\n")
	want := defaultConfig()
	want.Settings.Port = 8080
	checkConfig(t, mustParse(t, src), want)
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		// Required keys.
		{"empty file", "", `gondola.yaml: config.site-name is required; please set the name of the site`},
		{"only a comment", "# nothing yet\n", `gondola.yaml: config.site-name is required; please set the name of the site`},
		{"no config section", "input:\n  content: .\n", `gondola.yaml: config.site-name is required; please set the name of the site`},
		{"empty config section", "config:\n", `gondola.yaml: config.site-name is required; please set the name of the site`},
		{"version absent", "config:\n  site-name: S\n  canonical-url: https://example.com\n",
			`gondola.yaml: config.version is required; please set the version of the site`},
		{"canonical-url absent", "config:\n  site-name: S\n  version: 1\n",
			`gondola.yaml: config.canonical-url is required; please set the absolute URL where the site is published, such as https://example.com`},
		{"site-name empty", "config:\n  site-name:\n", `gondola.yaml:2:3: config.site-name is empty; please set the name of the site`},
		{"site-name empty string", "config:\n  site-name: \"\"\n", `gondola.yaml:2:3: config.site-name is empty; please set the name of the site`},
		// Found by FuzzParse: a tag with no text used to give an empty name.
		{"site-name only a tag", "config:\n  site-name: !0\n", `gondola.yaml:2:3: config.site-name is empty; please set the name of the site`},
		{"version only a tag", "config:\n  site-name: S\n  version: !0\n", `gondola.yaml:3:3: config.version is empty; please set the version of the site`},
		{"canonical-url only a tag", "config:\n  site-name: S\n  version: 1\n  canonical-url: !x\n",
			`gondola.yaml:4:3: config.canonical-url is empty; please set the absolute URL where the site is published, such as https://example.com`},
		{"version null", "config:\n  site-name: S\n  version: ~\n", `gondola.yaml:3:3: config.version is empty; please set the version of the site`},
		{"canonical-url empty", "config:\n  site-name: S\n  version: 1\n  canonical-url: ''\n",
			`gondola.yaml:4:3: config.canonical-url is empty; please set the absolute URL where the site is published, such as https://example.com`},

		// Unknown and repeated keys.
		{"unknown key in config", withMinimal("  sas: true\n"), `gondola.yaml:5:3: unknown key "sas" in config; please remove it or check its spelling`},
		{"unknown key in input", withMinimal("input:\n  template: x\n"), `gondola.yaml:6:3: unknown key "template" in input; please remove it or check its spelling`},
		{"unknown key in output", withMinimal("output:\n  dir: x\n"), `gondola.yaml:6:3: unknown key "dir" in output; please remove it or check its spelling`},
		{"unknown section", withMinimal("inputs:\n  content: .\n"), `gondola.yaml:5:1: unknown key "inputs" at the top level of gondola.yaml; please remove it or check its spelling`},
		{"keys are case-sensitive", withMinimal("  Sass: true\n"), `gondola.yaml:5:3: unknown key "Sass" in config; please remove it or check its spelling`},
		{"key twice", withMinimal("  sass: true\n  sass: false\n"), `gondola.yaml:6:3: key "sass" appears twice in config (first at line 5); please keep only one`},
		{"section twice", withMinimal("config:\n  port: 1\n"), `gondola.yaml:5:1: key "config" appears twice at the top level of gondola.yaml (first at line 1); please keep only one`},
		{"default key twice", withMinimal("default:\n  x: 1\n  x: 2\n"), `gondola.yaml:7:3: key "x" appears twice in default (first at line 6); please keep only one`},
		{"nested key twice", withMinimal("default:\n  social: {name: a, name: b}\n"), `gondola.yaml:6:21: key "name" appears twice in default.social (first at line 6); please keep only one`},
		{"default title not text", withMinimal("default:\n  title: [a]\n"), `gondola.yaml:6:10: default.title must be text, not a list`},
		{"default author not text", withMinimal("default:\n  author: {name: a}\n"), `gondola.yaml:6:11: default.author must be text, not a map`},
		{"default sitemap not a boolean", withMinimal("default:\n  sitemap: yes\n"), `gondola.yaml:6:12: default.sitemap must be true or false, not "yes"`},
		{"default excludeFromFleet as written", withMinimal("default:\n  exclude-from-fleet: 1\n"), `gondola.yaml:6:23: default.exclude-from-fleet must be true or false, not "1"`},
		{"default tags fleet", withMinimal("default:\n  tags: [a, fleet]\n"), `gondola.yaml:6:13: tag "fleet" is reserved by Gondola (templates use fleet for the collection of all pages); please rename it`},
		{"default tags map", withMinimal("default:\n  tags: {a: 1}\n"), `gondola.yaml:6:9: default.tags must be a tag or a list of tags, not a map`},
		{"default date not a date", withMinimal("default:\n  date: soon\n"), `gondola.yaml:6:9: default.date "soon" is not a date; please write it as YYYY-MM-DD, optionally followed by a time such as 2024-05-01T10:30:00Z`},
		{"map as key", withMinimal("default:\n  ? {a: 1}\n  : x\n"), `gondola.yaml:6:5: keys in default must be names, not a map`},

		// Shapes.
		{"file is a list", "- a\n", `gondola.yaml:1:1: gondola.yaml must be a map with the sections config, input, output and default, not a list`},
		{"file is text", "hello\n", `gondola.yaml:1:1: gondola.yaml must be a map with the sections config, input, output and default, not "hello"`},
		{"section is a list", "config: [a]\n", `gondola.yaml:1:9: config must be a map of keys, not a list`},
		{"section is text", withMinimal("output: _site\n"), `gondola.yaml:5:9: output must be a map of keys, not "_site"`},
		{"two documents", withMinimal("---\ninput: {}\n"), `gondola.yaml:5:1: a second YAML document starts here; gondola.yaml must hold only one, so please remove it`},
		{"bad indentation", "config:\n  site-name: S\n   version: 1\n", `gondola.yaml:3:1: invalid YAML: mapping values are not allowed in this context`},
		{"tab indentation", withMinimal("\tsass: true\n"), `gondola.yaml:4:1: invalid YAML: found a tab character that violates indentation`},

		// Types.
		{"text given a list", withMinimal("  description: [a]\n"), `gondola.yaml:5:16: config.description must be text, not a list`},
		{"text given a map", withMinimal("  robots-agent: {a: 1}\n"), `gondola.yaml:5:17: config.robots-agent must be text, not a map`},
		{"bool given yes", withMinimal("  sass: yes\n"), `gondola.yaml:5:9: config.sass must be true or false, not "yes"`},
		{"bool given quoted true", withMinimal("  sitemap: \"true\"\n"), `gondola.yaml:5:12: config.sitemap must be true or false, not "true"`},
		{"bool given number", withMinimal("  markdown: 1\n"), `gondola.yaml:5:13: config.markdown must be true or false, not "1"`},
		{"bool given list", withMinimal("  fenced-blocks: []\n"), `gondola.yaml:5:18: config.fenced-blocks must be true or false, not a list`},
		{"port given text", withMinimal("  port: abc\n"), `gondola.yaml:5:9: config.port must be a whole number, not "abc"`},
		{"port given float", withMinimal("  port: 1094.0\n"), `gondola.yaml:5:9: config.port must be a whole number, not "1094.0"`},
		{"port given quoted number", withMinimal("  port: \"8080\"\n"), `gondola.yaml:5:9: config.port must be a whole number, not "8080"`},
		{"port given a huge number", withMinimal("  port: 99999999999999999999\n"), `gondola.yaml:5:9: config.port must be a whole number, not "99999999999999999999"`},

		// Values.
		{"port zero", withMinimal("  port: 0\n"), `gondola.yaml:5:9: config.port is 0; it must be between 1 and 65535`},
		{"port negative", withMinimal("  port: -1\n"), `gondola.yaml:5:9: config.port is -1; it must be between 1 and 65535`},
		{"port too large", withMinimal("  port: 70000\n"), `gondola.yaml:5:9: config.port is 70000; it must be between 1 and 65535`},
		{"engine", withMinimal("  engine: go\n"), `gondola.yaml:5:11: config.engine is "go"; it must be "jet"`},
		{"engine case", withMinimal("  engine: Jet\n"), `gondola.yaml:5:11: config.engine is "Jet"; it must be "jet"`},
		{"routing", withMinimal("output:\n  routing: folder\n"), `gondola.yaml:6:12: output.routing is "folder"; it must be "file" or "directory"`},
		{"routing case", withMinimal("output:\n  routing: File\n"), `gondola.yaml:6:12: output.routing is "File"; it must be "file" or "directory"`},
		{"routing list", withMinimal("output:\n  routing: [file]\n"), `gondola.yaml:6:12: output.routing is a list; it must be "file" or "directory"`},
		{"permalink", withMinimal("default:\n  permalink: title\n"), `gondola.yaml:6:14: default.permalink is "title"; it must be "path", "slug" or "date"`},
		{"custom permalink", withMinimal("default:\n  permalink: /blog/\n"),
			`gondola.yaml:6:14: default.permalink is "/blog/"; it must be "path", "slug" or "date" (custom permalinks go in a page's front matter)`},
		{"layout given a list", withMinimal("default:\n  layout: [a]\n"), `gondola.yaml:6:11: default.layout must be text, not a list`},
		{"Jet function", withMinimal("default:\n  url: /x\n"), `gondola.yaml:6:3: default key "url" is the name of a Jet function and cannot be a template variable; please rename it`},
		{"layout with dot dot", withMinimal("default:\n  layout: ../base\n"), `gondola.yaml:6:11: default.layout "../base" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"layout with empty part", withMinimal("default:\n  layout: layouts//base\n"), `gondola.yaml:6:11: default.layout "layouts//base" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"fenced blocks without markdown", withMinimal("  fenced-blocks: true\n  markdown: false\n"),
			`gondola.yaml:5:18: config.fenced-blocks is true but config.markdown is false; fenced blocks only work in Markdown pages`},

		// Canonical URL.
		{"url without scheme", "config:\n  site-name: S\n  version: 1\n  canonical-url: example.com\n",
			`gondola.yaml:4:18: config.canonical-url "example.com" is not an absolute http or https URL`},
		{"url with another scheme", "config:\n  site-name: S\n  version: 1\n  canonical-url: ftp://example.com\n",
			`gondola.yaml:4:18: config.canonical-url "ftp://example.com" is not an absolute http or https URL`},
		{"url without host", "config:\n  site-name: S\n  version: 1\n  canonical-url: https:///blog\n",
			`gondola.yaml:4:18: config.canonical-url "https:///blog" is not an absolute http or https URL`},
		{"url opaque", "config:\n  site-name: S\n  version: 1\n  canonical-url: https:example.com\n",
			`gondola.yaml:4:18: config.canonical-url "https:example.com" is not an absolute http or https URL`},
		{"url relative", "config:\n  site-name: S\n  version: 1\n  canonical-url: /blog\n",
			`gondola.yaml:4:18: config.canonical-url "/blog" is not an absolute http or https URL`},
		{"url with space in host", "config:\n  site-name: S\n  version: 1\n  canonical-url: https://exa mple.com\n",
			`gondola.yaml:4:18: config.canonical-url "https://exa mple.com" is not an absolute http or https URL`},
		{"url with user", "config:\n  site-name: S\n  version: 1\n  canonical-url: https://me:secret@example.com\n",
			`gondola.yaml:4:18: config.canonical-url "https://me:secret@example.com" must not contain a user name or password`},
		{"url with query", "config:\n  site-name: S\n  version: 1\n  canonical-url: https://example.com/?a=1\n",
			`gondola.yaml:4:18: config.canonical-url "https://example.com/?a=1" must not have a query (the part after "?")`},
		{"url with empty query", "config:\n  site-name: S\n  version: 1\n  canonical-url: https://example.com/?\n",
			`gondola.yaml:4:18: config.canonical-url "https://example.com/?" must not have a query (the part after "?")`},
		{"url with fragment", "config:\n  site-name: S\n  version: 1\n  canonical-url: \"https://example.com/#top\"\n",
			`gondola.yaml:4:18: config.canonical-url "https://example.com/#top" must not have a fragment (the part after "#")`},
		{"url with empty fragment", "config:\n  site-name: S\n  version: 1\n  canonical-url: \"https://example.com/#\"\n",
			`gondola.yaml:4:18: config.canonical-url "https://example.com/#" must not have a fragment (the part after "#")`},
		{"url given a list", "config:\n  site-name: S\n  version: 1\n  canonical-url: [a]\n",
			`gondola.yaml:4:18: config.canonical-url must be text, not a list`},

		// Paths and lists.
		{"parent path", withMinimal("input:\n  content: ../site\n"),
			`gondola.yaml:6:12: input.content "../site" is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"parent inside path", withMinimal("input:\n  data: a/../b\n"),
			`gondola.yaml:6:9: input.data "a/../b" is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"bare parent", withMinimal("output:\n  directory: ..\n"),
			`gondola.yaml:6:14: output.directory ".." is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"absolute path", withMinimal("output:\n  directory: /var/www\n"),
			`gondola.yaml:6:14: output.directory "/var/www" is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"parent in list", withMinimal("input:\n  partials: [a, ../b]\n"),
			`gondola.yaml:6:17: input.partials "../b" is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"parent plugin", withMinimal("  plugin: [../tools/x]\n"),
			`gondola.yaml:5:12: config.plugin "../tools/x" is not allowed: paths are relative to the project directory and may not contain ".." or be absolute`},
		{"path given a map", withMinimal("input:\n  content: {a: 1}\n"), `gondola.yaml:6:12: input.content must be a path, not a map`},
		{"dir list given a map", withMinimal("input:\n  templates: {a: 1}\n"), `gondola.yaml:6:14: input.templates must be a path or a list of paths, not a map`},
		{"empty entry", withMinimal("input:\n  templates: [a, ~]\n"), `gondola.yaml:6:18: input.templates has an empty entry; please remove it`},
		{"empty string entry", withMinimal("input:\n  data:\n    - a\n    - \"\"\n"), `gondola.yaml:8:7: input.data has an empty entry; please remove it`},
		{"entry is a list", withMinimal("input:\n  data: [[a]]\n"), `gondola.yaml:6:10: each entry of input.data must be a path, not a list`},
		{"directory twice", withMinimal("input:\n  templates: [a, ./a/]\n"), `gondola.yaml:6:18: input.templates lists "a" twice; please keep only one`},
		{"plugin given text", withMinimal("  plugin: x.sh\n"), `gondola.yaml:5:11: config.plugin must be a list of paths, not "x.sh"`},
		{"plugin empty entry", withMinimal("  plugin: [\"\"]\n"), `gondola.yaml:5:12: config.plugin has an empty entry; please remove it`},
		{"passthrough given text", withMinimal("output:\n  passthrough: robots.txt\n"), `gondola.yaml:6:16: output.passthrough must be a list of patterns, not "robots.txt"`},
		{"passthrough empty entry", withMinimal("output:\n  passthrough: [a, null]\n"), `gondola.yaml:6:20: output.passthrough has an empty entry; please remove it`},
		{"passthrough entry map", withMinimal("output:\n  passthrough: [{a: 1}]\n"), `gondola.yaml:6:17: each entry of output.passthrough must be a pattern, not a map`},

		// Role directories and the content directory.
		{"templates is content", withMinimal("input:\n  templates: .\n"),
			`gondola.yaml:6:14: input.templates "." is the content directory; role directories may be inside it but must not be it or contain it`},
		{"data is content", withMinimal("input:\n  content: src\n  data: src/\n"),
			`gondola.yaml:7:9: input.data "src" is the content directory; role directories may be inside it but must not be it or contain it`},
		{"partials contains content", withMinimal("input:\n  content: site/src\n  partials: site\n"),
			`gondola.yaml:7:13: input.partials "site" contains the content directory "site/src"; role directories may be inside it but must not be it or contain it`},
		{"second template dir contains content", withMinimal("input:\n  content: src\n  templates: [layouts, .]\n"),
			`gondola.yaml:7:24: input.templates "." contains the content directory "src"; role directories may be inside it but must not be it or contain it`},
		{"default dir contains content", withMinimal("input:\n  content: _data/pages\n"),
			`gondola.yaml:6:12: input.data "_data" (the default) contains the content directory "_data/pages"; role directories may be inside it but must not be it or contain it`},
		{"output is content", withMinimal("output:\n  directory: .\n"),
			`gondola.yaml:6:14: output.directory "." is the content directory; role directories may be inside it but must not be it or contain it`},
		{"output contains content", withMinimal("input:\n  content: _site/src\n"),
			`gondola.yaml:6:12: output.directory "_site" (the default) contains the content directory "_site/src"; role directories may be inside it but must not be it or contain it`},
		{"temp is content", withMinimal("input:\n  content: _site.temp\n"),
			`gondola.yaml:6:12: the temp directory "_site.temp" is the content directory; role directories may be inside it but must not be it or contain it`},

		// A data directory inside another one would load its files twice.
		{"data dir inside the next one", withMinimal("input:\n  data: [_data/authors, _data]\n"),
			`gondola.yaml:6:10: input.data "_data/authors" is inside input.data "_data"; please list only one`},
		{"data dir inside the previous one", withMinimal("input:\n  data:\n    - shared\n    - local\n    - shared/blog/authors\n"),
			`gondola.yaml:9:7: input.data "shared/blog/authors" is inside input.data "shared"; please list only one`},

		// The directories a build deletes.
		{"output is data", withMinimal("output:\n  directory: _data\n"),
			`gondola.yaml:6:14: output.directory "_data" is also used by input.data; every build deletes it, so please choose another output directory`},
		{"output contains default templates", withMinimal("output:\n  directory: _components\n"),
			`gondola.yaml:6:14: output.directory "_components" contains input.templates "_components/includes" (the default); every build deletes it, so please choose another output directory`},
		{"default output contains templates", withMinimal("input:\n  templates: _site/layouts\n"),
			`gondola.yaml:6:14: output.directory "_site" (the default) contains input.templates "_site/layouts"; every build deletes it, so please choose another output directory`},
		{"output inside partials", withMinimal("input:\n  partials: styles\noutput:\n  directory: styles/out\n"),
			`gondola.yaml:8:14: output.directory "styles/out" is inside input.partials "styles"; please choose an output directory outside it`},
		{"temp contains data", withMinimal("input:\n  data: _site.temp/data\n"),
			`gondola.yaml:6:9: the temp directory "_site.temp" contains input.data "_site.temp/data"; every build deletes it, so please choose another output directory`},
		{"temp is templates", withMinimal("input:\n  templates: public.temp\noutput:\n  directory: public\n"),
			`gondola.yaml:8:14: the temp directory "public.temp" is also used by input.templates; every build deletes it, so please choose another output directory`},
		{"output contains plugin", withMinimal("  plugin: [_site/minify.sh]\n"),
			`gondola.yaml:5:12: output.directory "_site" (the default) contains config.plugin "_site/minify.sh"; every build deletes it, so please move the plugin or choose another output directory`},
		{"temp contains plugin", withMinimal("  plugin: [build.temp/x]\noutput:\n  directory: build\n"),
			`gondola.yaml:7:14: the temp directory "build.temp" contains config.plugin "build.temp/x"; every build deletes it, so please move the plugin or choose another output directory`},

		// Keys under default: follow the naming rule (SPEC §4).
		{"default key empty word", withMinimal("default:\n  og--image: x\n"),
			`gondola.yaml:6:3: default key "og--image" has an empty word; please remove the extra "-" or "_"`},
		{"default key leading underscore", withMinimal("default:\n  _draft: x\n"),
			`gondola.yaml:6:3: default key "_draft" has an empty word; please remove the extra "-" or "_"`},
		{"default key empty", withMinimal("default:\n  \"\": x\n"), `gondola.yaml:6:3: default key "" is empty; please give it a name`},
		{"default key starts with digit", withMinimal("default:\n  2-col: x\n"),
			`gondola.yaml:6:3: default key "2-col" becomes "2Col" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"default key with dot", withMinimal("default:\n  og.image: x\n"),
			`gondola.yaml:6:3: default key "og.image" becomes "og.image" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"default key keyword", withMinimal("default:\n  range: x\n"),
			`gondola.yaml:6:3: default key "range" is a Jet keyword and cannot be a template variable; please rename it`},
		{"default keys collide", withMinimal("default:\n  robots-agent: a\n  robotsAgent: b\n"),
			`gondola.yaml:7:3: default keys "robots-agent" and "robotsAgent" both become "robotsAgent" in templates; please rename one`},
		{"default key reserved", withMinimal("default:\n  site: x\n"),
			`gondola.yaml:6:3: default key "site" is reserved by Gondola (templates use it for the site settings); please rename it`},
		{"default key becomes reserved", withMinimal("default:\n  Page: x\n"),
			`gondola.yaml:6:3: default key "Page" becomes "page" in templates, which is reserved by Gondola (templates use it for the page's URL and metadata); please rename it`},
		{"default key merge", withMinimal("default:\n  <<: {a: 1}\n"),
			`gondola.yaml:6:3: default key "<<" becomes "<<" in templates, which is not a valid name (names must start with a letter); please rename it`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.src))
			if err == nil {
				t.Fatalf("Parse returned no error, want:\n%s\nsource:\n%s", test.want, test.src)
			}
			if err.Error() != test.want {
				t.Errorf("error:\n got %s\nwant %s\nsource:\n%s", err, test.want, test.src)
			}
			// Every error from Parse names the file, so the console can
			// show the line.
			var diagErr *diag.Error
			if !errors.As(err, &diagErr) || diagErr.File != FileName {
				t.Errorf("error %v is not a diag.Error naming %s", err, FileName)
			}
		})
	}
}

func TestParseErrorColumnCountsBytes(t *testing.T) {
	// yaml.v3 counts columns in characters; diag counts bytes. "à" takes
	// two bytes, so the second "città" starts at byte 22, character 21.
	src := withMinimal("default: {città: 1, città: 2}\n")
	_, err := Parse([]byte(src))
	want := `gondola.yaml:5:22: key "città" appears twice in default (first at line 5); please keep only one`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}

func TestParseErrorPositionWithUnusualLineBreaks(t *testing.T) {
	// yaml.v3 starts a new line at these, the console does not: the error
	// must name the line the console will print.
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"lone CR", "config:\r  sas: 1\r", `gondola.yaml:1:11: unknown key "sas" in config; please remove it or check its spelling`},
		{"LS", "config:\n  site-name: S\u2028  sas: 1\n", `gondola.yaml:2:20: unknown key "sas" in config; please remove it or check its spelling`},
		{"NEL", "config:\u0085  sas: 1\n", `gondola.yaml:1:12: unknown key "sas" in config; please remove it or check its spelling`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.src))
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, FileName), []byte(minimal), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(projectDir)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	want := defaultConfig()
	// True on macOS and Windows by default; see lettercase_test.go.
	want.IgnoresLetterCase = fileSystemIgnoresCase(t, t.TempDir())
	checkConfig(t, got, want)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(t.TempDir())
	want := `gondola.yaml not found in the current directory; it holds the site settings and must set at least config.site-name, config.version and config.canonical-url`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	// A directory named gondola.yaml exists but cannot be read as a file.
	projectDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(projectDir, FileName), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load(projectDir)
	if err == nil || !strings.HasPrefix(err.Error(), "reading gondola.yaml: ") {
		t.Errorf("error = %v, want one starting with %q", err, "reading gondola.yaml: ")
	}
}

func TestLoadErrorNamesFile(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, FileName), []byte(withMinimal("  sas: true\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(projectDir)
	want := `gondola.yaml:5:3: unknown key "sas" in config; please remove it or check its spelling`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}

func TestParseExcessiveAliasing(t *testing.T) {
	// A "billion laughs" document: ten aliases of ten aliases, nine times
	// over. yaml.v3 refuses to expand it, so Parse fails at once instead
	// of filling memory.
	var src strings.Builder
	src.WriteString(withMinimal("default:\n  a0: &a0 [x, x, x, x, x, x, x, x, x, x]\n"))
	for level := 1; level <= 9; level++ {
		previous := fmt.Sprintf("*a%d", level-1)
		fmt.Fprintf(&src, "  a%d: &a%d [%s]\n", level, level, strings.Repeat(previous+", ", 9)+previous)
	}
	_, err := Parse([]byte(src.String()))
	want := `gondola.yaml:8:7: invalid YAML: document contains excessive aliasing`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}

func TestParseRefusesUTF16(t *testing.T) {
	// Windows PowerShell 5.1 writes UTF-16 by default. yaml.v3 would read
	// it, but its positions would not match the bytes of the file.
	want := `gondola.yaml: the file is saved as UTF-16; please save it as UTF-8`
	tests := []struct {
		name string
		src  string
	}{
		{"little-endian", "\xff\xfec\x00o\x00n\x00f\x00i\x00g\x00:\x00\n\x00"},
		{"big-endian", "\xfe\xff\x00c\x00o\x00n\x00f\x00i\x00g\x00:\x00\n"},
		{"mark only", "\xff\xfe"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.src))
			if err == nil || err.Error() != want {
				t.Errorf("error:\n got %v\nwant %s", err, want)
			}
		})
	}
}

func TestParseAcceptsUTF8ByteOrderMark(t *testing.T) {
	// yaml.v3 skips a UTF-8 byte order mark, which some Windows editors write.
	const utf8ByteOrderMark = string(rune(0xFEFF))
	checkConfig(t, mustParse(t, utf8ByteOrderMark+minimal), defaultConfig())
}

// TestParseByteOrderMarkPositions checks that columns on line 1 do not count
// a byte order mark. yaml.v3 counts it as one character, which once moved
// the column 2 bytes to the right.
func TestParseByteOrderMarkPositions(t *testing.T) {
	const utf8ByteOrderMark = string(rune(0xFEFF))
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"line 1", utf8ByteOrderMark + "config: [1]\n", "gondola.yaml:1:9: config must be a map of keys, not a list"},
		{"line 1 without the mark", "config: [1]\n", "gondola.yaml:1:9: config must be a map of keys, not a list"},
		{"line 2", utf8ByteOrderMark + "config:\n  sass: yes\n", `gondola.yaml:2:9: config.sass must be true or false, not "yes"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse([]byte(test.src))
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}
