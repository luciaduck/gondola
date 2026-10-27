package frontmatter

import (
	"reflect"
	"testing"
	"time"
)

// builtInDefaults are the entries of default: when gondola.yaml leaves the
// section out, as package config gives them.
func builtInDefaults() map[string]any {
	return map[string]any{"layout": NoLayout, "title": "", "author": "", "date": "", "permalink": "path"}
}

// documentOf returns a Document with only variables, as Split would give
// it.
func documentOf(variables map[string]any) Document {
	return Document{Variables: variables}
}

func TestMerge(t *testing.T) {
	may1 := time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)
	tests := []struct {
		name     string
		defaults map[string]any
		// layouts lists the chain nearest first, as Merge takes it.
		layouts []map[string]any
		page    map[string]any
		want    map[string]any
	}{
		{
			name:     "built-in defaults only",
			defaults: builtInDefaults(),
			page:     map[string]any{},
			want:     map[string]any{"layout": "nil", "title": "", "author": "", "date": "", "permalink": "path", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "the page wins",
			defaults: map[string]any{"title": "Default", "author": "Default"},
			layouts:  []map[string]any{{"title": "Layout"}},
			page:     map[string]any{"title": "Page"},
			want:     map[string]any{"title": "Page", "author": "Default", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "the nearest layout wins over the farthest",
			defaults: map[string]any{"color": "default"},
			layouts:  []map[string]any{{"color": "near"}, {"color": "far", "font": "far"}},
			page:     map[string]any{},
			want:     map[string]any{"color": "near", "font": "far", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "empty falls through to the next weaker level",
			defaults: map[string]any{"title": "Default"},
			layouts:  []map[string]any{{"title": ""}, {"title": "Far"}},
			page:     map[string]any{"title": ""},
			want:     map[string]any{"title": "Far", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "empty everywhere still exists",
			defaults: map[string]any{},
			layouts:  []map[string]any{{"subtitle": ""}},
			page:     map[string]any{"note": ""},
			want:     map[string]any{"subtitle": "", "note": "", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "real values are kept",
			defaults: map[string]any{"count": 5, "draft": true, "list": []any{"x"}},
			page:     map[string]any{"count": 0, "draft": false, "list": []any{}, "tags": []string{}, "sitemap": false},
			want:     map[string]any{"count": 0, "draft": false, "list": []any{}, "tags": []string{}, "sitemap": false, "excludeFromFleet": false},
		},
		{
			name:     "empty tags take the default list",
			defaults: map[string]any{"tags": []string{"blog"}},
			page:     map[string]any{"tags": ""},
			want:     map[string]any{"tags": []string{"blog"}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "a default left empty keeps the built-in value",
			defaults: map[string]any{"tags": "", "sitemap": "", "excludeFromFleet": ""},
			page:     map[string]any{},
			want:     map[string]any{"tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "a layout's own layout key is not merged",
			defaults: map[string]any{"layout": "page"},
			layouts:  []map[string]any{{"layout": "base"}, {"layout": "nil"}},
			page:     map[string]any{},
			want:     map[string]any{"layout": "page", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "the page's layout wins over the default",
			defaults: map[string]any{"layout": "page"},
			layouts:  []map[string]any{{"layout": "base"}},
			page:     map[string]any{"layout": "post"},
			want:     map[string]any{"layout": "post", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "date from the page is formatted",
			defaults: map[string]any{"date": ""},
			page:     map[string]any{"date": may1},
			want:     map[string]any{"date": "1 May 2024", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
		{
			name:     "date from a layout",
			defaults: map[string]any{"date": ""},
			layouts:  []map[string]any{{"date": may1}},
			page:     map[string]any{"date": ""},
			want:     map[string]any{"date": "1 May 2024", "tags": []string{}, "sitemap": true, "excludeFromFleet": false},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layouts := make([]Document, 0, len(test.layouts))
			for _, variables := range test.layouts {
				layouts = append(layouts, documentOf(variables))
			}
			defaults := NewDefaults(test.defaults, "2 January 2006")
			got := Merge(defaults, layouts, documentOf(test.page))
			if !reflect.DeepEqual(got.Variables, test.want) {
				t.Errorf("got  %#v\nwant %#v", got.Variables, test.want)
			}
		})
	}
}

func TestMergeDate(t *testing.T) {
	may1 := time.Date(2024, time.May, 1, 10, 30, 0, 0, time.UTC)
	defaults := NewDefaults(builtInDefaults(), "2006-01-02 15:04")

	dated := Merge(defaults, nil, documentOf(map[string]any{"date": may1}))
	if !dated.HasDate || !dated.Date.Equal(may1) || dated.Variables["date"] != "2024-05-01 10:30" {
		t.Errorf("with a date: %+v", dated)
	}
	undated := Merge(defaults, nil, documentOf(map[string]any{}))
	if undated.HasDate || !undated.Date.IsZero() || undated.Variables["date"] != "" {
		t.Errorf("without a date: %+v", undated)
	}
	// A date under default: applies to pages that have none.
	withDefaultDate := NewDefaults(map[string]any{"date": may1}, "2006-01-02")
	if merged := Merge(withDefaultDate, nil, documentOf(map[string]any{"date": ""})); !merged.HasDate || merged.Variables["date"] != "2024-05-01" {
		t.Errorf("default date: %+v", merged)
	}
}

func TestMergeLeavesItsInputsAlone(t *testing.T) {
	may1 := time.Date(2024, time.May, 1, 0, 0, 0, 0, time.UTC)
	defaultVariables := map[string]any{"title": "Default"}
	defaults := NewDefaults(defaultVariables, "2006")
	page := documentOf(map[string]any{"date": may1})
	layout := documentOf(map[string]any{"layout": "base"})

	first := Merge(defaults, []Document{layout}, page)
	first.Variables["title"] = "changed"
	second := Merge(defaults, []Document{layout}, page)

	if second.Variables["title"] != "Default" {
		t.Errorf("a change to one page's variables reached the next page: %#v", second.Variables)
	}
	if !reflect.DeepEqual(defaultVariables, map[string]any{"title": "Default"}) {
		t.Errorf("NewDefaults changed its input: %#v", defaultVariables)
	}
	if page.Variables["date"] != may1 || layout.Variables["layout"] != "base" {
		t.Errorf("Merge changed a document: %#v, %#v", page.Variables, layout.Variables)
	}
}

func TestMergeSplitDocuments(t *testing.T) {
	// The whole path: a page and a layout read by Split, merged with the
	// defaults as config gives them.
	page, err := Split("index.md", []byte("---\ntitle: Home\ntags: news\ndate: 2024-05-01\n---\n# Hi\n"))
	if err != nil {
		t.Fatal(err)
	}
	layout, err := Split("page.jet", []byte("---\nlayout: base\ntitle: Untitled\nbanner: /b.png\n---\n{{ copy|raw }}\n"))
	if err != nil {
		t.Fatal(err)
	}
	defaults := builtInDefaults()
	defaults["layout"] = "page"
	merged := Merge(NewDefaults(defaults, "2 January 2006"), []Document{layout}, page)
	want := map[string]any{
		"layout": "page", "title": "Home", "author": "", "date": "1 May 2024", "permalink": "path",
		"tags": []string{"news"}, "sitemap": true, "excludeFromFleet": false, "banner": "/b.png",
	}
	if !reflect.DeepEqual(merged.Variables, want) {
		t.Errorf("got  %#v\nwant %#v", merged.Variables, want)
	}
}

func TestParentLayout(t *testing.T) {
	tests := []struct {
		name       string
		variables  map[string]any
		wantParent string
		wantOK     bool
	}{
		{"parent", map[string]any{"layout": "base"}, "base", true},
		{"no layout key", map[string]any{}, "", false},
		{"empty", map[string]any{"layout": ""}, "", false},
		{"nil", map[string]any{"layout": "nil"}, "", false},
		{"Nil is a name", map[string]any{"layout": "Nil"}, "Nil", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parent, ok := ParentLayout(documentOf(test.variables))
			if parent != test.wantParent || ok != test.wantOK {
				t.Errorf("ParentLayout = %q, %v; want %q, %v", parent, ok, test.wantParent, test.wantOK)
			}
		})
	}
}
