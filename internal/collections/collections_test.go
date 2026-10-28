package collections

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/frontmatter"
)

const canonicalURL = "https://example.com"

// builtInDefaults are the entries of the built-in default: section.
func builtInDefaults() map[string]any {
	return map[string]any{"layout": "nil", "title": "", "author": "", "date": "", "permalink": "path"}
}

// newPage prepares a page as build will: its front matter, written without
// the "---" lines, is split and merged with defaults (the built-in ones plus
// extraDefaults). The URL path is made from the source path.
func newPage(t *testing.T, sourcePath, frontMatter string, extraDefaults map[string]any) Page {
	t.Helper()
	document, err := frontmatter.Split(sourcePath, []byte("---\n"+frontMatter+"---\nbody\n"))
	if err != nil {
		t.Fatalf("splitting %s: %v", sourcePath, err)
	}
	defaultVariables := builtInDefaults()
	for name, value := range extraDefaults {
		defaultVariables[name] = value
	}
	defaults := frontmatter.NewDefaults(defaultVariables, "2006-01-02")
	return Page{
		SourcePath: sourcePath,
		URLPath:    "/" + strings.TrimSuffix(sourcePath, ".md") + ".html",
		Merged:     frontmatter.Merge(defaults, nil, document),
	}
}

func sources(entries []map[string]any) []string {
	result := []string{}
	for _, entry := range entries {
		result = append(result, entry[sourceKey].(string))
	}
	return result
}

func TestBuildEntry(t *testing.T) {
	page := newPage(t, "blog/post.md", "title: First post\ndate: 2024-05-01T10:30:00+02:00\ntags: [news, Machine Learning]\nog-image: /a.png\n", nil)
	got := Build([]Page{page}, canonicalURL)
	want := map[string]any{
		"url":       "/blog/post.html",
		"canonical": "https://example.com/blog/post.html",
		"source":    "blog/post.md",
		"title":     "First post",
		"date":      "2024-05-01",
		"tags":      []string{"news", "Machine Learning"},
		"meta":      page.Merged.Variables,
	}
	if len(got.Fleet) != 1 {
		t.Fatalf("fleet has %d entries, want 1", len(got.Fleet))
	}
	if !reflect.DeepEqual(got.Fleet[0], want) {
		t.Errorf("entry = %#v\n want %#v", got.Fleet[0], want)
	}
	meta := got.Fleet[0]["meta"].(map[string]any)
	for name, value := range map[string]any{"ogImage": "/a.png", "author": "", "sitemap": true, "excludeFromFleet": false} {
		if !reflect.DeepEqual(meta[name], value) {
			t.Errorf("meta[%q] = %#v, want %#v", name, meta[name], value)
		}
	}
}

func TestBuildEntryWithoutTitleOrDate(t *testing.T) {
	got := Build([]Page{newPage(t, "about.md", "", nil)}, canonicalURL+"/blog")
	want := map[string]any{
		"url":       "/about.html",
		"canonical": "https://example.com/blog/about.html",
		"source":    "about.md",
		"title":     "",
		"date":      "",
		"tags":      []string{},
	}
	entry := got.Fleet[0]
	delete(entry, "meta")
	if !reflect.DeepEqual(entry, want) {
		t.Errorf("entry = %#v\n want %#v", entry, want)
	}
}

func TestBuildOrder(t *testing.T) {
	type source struct {
		path        string
		frontMatter string
	}
	tests := []struct {
		name  string
		pages []source
		want  []string
	}{
		{"by date from the oldest", []source{
			{"c.md", "date: 2024-03-01\n"},
			{"a.md", "date: 2024-05-01\n"},
			{"b.md", "date: 2023-12-31\n"},
		}, []string{"b.md", "c.md", "a.md"}},
		{"pages without a date come first, by source path", []source{
			{"dated.md", "date: 2000-01-01\n"},
			{"z.md", ""},
			{"m.md", "title: M\n"},
		}, []string{"m.md", "z.md", "dated.md"}},
		{"the same date is broken by source path", []source{
			{"blog/b.md", "date: 2024-05-01\n"},
			{"blog/a.md", "date: 2024-05-01\n"},
			{"a.md", "date: 2024-05-01\n"},
		}, []string{"a.md", "blog/a.md", "blog/b.md"}},
		{"times on the same day", []source{
			{"evening.md", "date: 2024-05-01 18:00:00\n"},
			{"day.md", "date: 2024-05-01\n"},
			{"morning.md", "date: 2024-05-01T08:00:00Z\n"},
		}, []string{"day.md", "morning.md", "evening.md"}},
		{"the same moment in two offsets is a tie", []source{
			{"b.md", "date: 2024-05-01T10:00:00+02:00\n"},
			{"a.md", "date: 2024-05-01T08:00:00Z\n"},
		}, []string{"a.md", "b.md"}},
		{"moments are compared, not the dates as written", []source{
			// 23:30 at -05:00 is 04:30 on May 2 in UTC, later than 01:00Z.
			{"a.md", "date: 2024-05-01T23:30:00-05:00\n"},
			{"b.md", "date: 2024-05-02T01:00:00Z\n"},
		}, []string{"b.md", "a.md"}},
		{"source paths are compared byte by byte", []source{
			{"about.md", ""},
			{"a/b.md", ""},
			{"Blog/x.md", ""},
			{"a-b.md", ""},
			{"_x.md", ""},
		}, []string{"Blog/x.md", "_x.md", "a-b.md", "a/b.md", "about.md"}},
		{"a quoted date counts", []source{
			{"a.md", "date: \"2024-06-01\"\n"},
			{"b.md", "date: 2024-05-01\n"},
		}, []string{"b.md", "a.md"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var pages []Page
			for _, page := range test.pages {
				pages = append(pages, newPage(t, page.path, page.frontMatter, nil))
			}
			if got := sources(Build(pages, canonicalURL).Fleet); !slices.Equal(got, test.want) {
				t.Errorf("fleet = %q, want %q", got, test.want)
			}
			// The order of the input does not matter.
			slices.Reverse(pages)
			if got := sources(Build(pages, canonicalURL).Fleet); !slices.Equal(got, test.want) {
				t.Errorf("fleet from reversed pages = %q, want %q", got, test.want)
			}
		})
	}
}

func TestBuildDateFromDefault(t *testing.T) {
	// A page without its own date takes default.date, so it is no longer
	// among the pages without a date.
	// The value is read as package config reads default.date, by the rules
	// of front matter.
	document, err := frontmatter.Split("gondola.yaml", []byte("---\ndate: 2024-01-01\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]any{"date": document.Variables["date"]}
	pages := []Page{
		newPage(t, "a.md", "date: 2024-06-01\n", defaults),
		newPage(t, "b.md", "", defaults),
		newPage(t, "c.md", "date: 2023-06-01\n", defaults),
	}
	got := Build(pages, canonicalURL).Fleet
	if want := []string{"c.md", "b.md", "a.md"}; !slices.Equal(sources(got), want) {
		t.Errorf("fleet = %q, want %q", sources(got), want)
	}
	if date := got[1]["date"]; date != "2024-01-01" {
		t.Errorf("date of b.md = %#v, want %q", date, "2024-01-01")
	}
}

func TestBuildTags(t *testing.T) {
	pages := []Page{
		newPage(t, "one.md", "date: 2024-03-01\ntags: news\n", nil),
		newPage(t, "two.md", "date: 2024-01-01\ntags: [news, Machine Learning, 2024]\n", nil),
		newPage(t, "three.md", "tags: [News]\n", nil),
		newPage(t, "four.md", "title: no tags\n", nil),
		newPage(t, "five.md", "tags: []\n", nil),
	}
	got := Build(pages, canonicalURL)
	want := map[string][]string{
		"news":             {"two.md", "one.md"},
		"News":             {"three.md"},
		"Machine Learning": {"two.md"},
		"2024":             {"two.md"},
	}
	if len(got.Tags) != len(want) {
		t.Errorf("collections has %d tags, want %d: %v", len(got.Tags), len(want), got.Tags)
	}
	for tag, wantSources := range want {
		if gotSources := sources(got.Tags[tag]); !slices.Equal(gotSources, wantSources) {
			t.Errorf("collections[%q] = %q, want %q", tag, gotSources, wantSources)
		}
	}
	if wantFleet := []string{"five.md", "four.md", "three.md", "two.md", "one.md"}; !slices.Equal(sources(got.Fleet), wantFleet) {
		t.Errorf("fleet = %q, want %q", sources(got.Fleet), wantFleet)
	}
}

func TestBuildTagsFromDefault(t *testing.T) {
	// default.tags reaches every page that has no tags of its own; a page's
	// own tags replace it, as any variable does.
	defaults := map[string]any{"tags": []string{"site"}}
	pages := []Page{
		newPage(t, "a.md", "", defaults),
		newPage(t, "b.md", "tags: [news]\n", defaults),
	}
	got := Build(pages, canonicalURL)
	if gotSources := sources(got.Tags["site"]); !slices.Equal(gotSources, []string{"a.md"}) {
		t.Errorf(`collections["site"] = %q, want ["a.md"]`, gotSources)
	}
	if gotSources := sources(got.Tags["news"]); !slices.Equal(gotSources, []string{"b.md"}) {
		t.Errorf(`collections["news"] = %q, want ["b.md"]`, gotSources)
	}
}

func TestBuildExcludeFromFleet(t *testing.T) {
	pages := []Page{
		newPage(t, "a.md", "tags: [news]\n", nil),
		newPage(t, "hidden.md", "excludeFromFleet: true\ntags: [news]\n", nil),
		newPage(t, "spelled.md", "exclude-from-fleet: true\n", nil),
		newPage(t, "kept.md", "excludeFromFleet: false\n", nil),
	}
	got := Build(pages, canonicalURL)
	if want := []string{"a.md", "kept.md"}; !slices.Equal(sources(got.Fleet), want) {
		t.Errorf("fleet = %q, want %q", sources(got.Fleet), want)
	}
	// The page stays in the collections of its tags.
	if want := []string{"a.md", "hidden.md"}; !slices.Equal(sources(got.Tags["news"]), want) {
		t.Errorf(`collections["news"] = %q, want %q`, sources(got.Tags["news"]), want)
	}
}

func TestBuildSharesEntries(t *testing.T) {
	got := Build([]Page{newPage(t, "a.md", "tags: [x, y]\n", nil)}, canonicalURL)
	fleetEntry := reflect.ValueOf(got.Fleet[0]).Pointer()
	for _, tag := range []string{"x", "y"} {
		if reflect.ValueOf(got.Tags[tag][0]).Pointer() != fleetEntry {
			t.Errorf("collections[%q] holds another map than fleet", tag)
		}
	}
}

func TestBuildNoPages(t *testing.T) {
	got := Build(nil, canonicalURL)
	if got.Fleet == nil || len(got.Fleet) != 0 {
		t.Errorf("Fleet = %#v, want an empty list", got.Fleet)
	}
	if got.Tags == nil || len(got.Tags) != 0 {
		t.Errorf("Tags = %#v, want an empty map", got.Tags)
	}
}

func TestBuildLeavesPagesAlone(t *testing.T) {
	pages := []Page{
		newPage(t, "b.md", "date: 2024-01-01\n", nil),
		newPage(t, "a.md", "date: 2023-01-01\n", nil),
	}
	Build(pages, canonicalURL)
	if pages[0].SourcePath != "b.md" || pages[1].SourcePath != "a.md" {
		t.Errorf("Build reordered its input: %q, %q", pages[0].SourcePath, pages[1].SourcePath)
	}
}
