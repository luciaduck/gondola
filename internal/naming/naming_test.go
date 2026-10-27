package naming

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestTemplateName(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		// The examples of SPEC §4.
		{"og-image", "ogImage"},
		{"canonical-url", "canonicalUrl"},
		{"og_image", "ogImage"},
		// Already camelCase: unchanged.
		{"excludeFromFleet", "excludeFromFleet"},
		{"robotsAgent", "robotsAgent"},
		// A first word that is not all capitals only gets its first letter
		// lowercased.
		{"Title", "title"},
		{"ExcludeFromFleet", "excludeFromFleet"},
		{"X-ray", "xRay"},
		// There is no acronym detection inside a word: only a first word
		// written entirely in capitals is lowercased entirely.
		{"URLPath", "uRLPath"},
		// A first word written in capitals is lowercased entirely.
		{"URL-path", "urlPath"},
		{"ID", "id"},
		{"HTML5-page", "html5Page"},
		{"OG-IMAGE", "ogIMAGE"},
		{"ÉCOLE-name", "écoleName"},
		// Letters without case do not make a word capitals.
		{"日本-x", "日本X"},
		{"日本X", "日本X"},
		// Following words keep the rest as written; no acronym rules.
		{"og-IMAGE", "ogIMAGE"},
		{"canonical-URL", "canonicalURL"},
		{"x-y-z", "xYZ"},
		{"og-image_url", "ogImageUrl"},
		// Digits.
		{"item-2", "item2"},
		{"h1", "h1"},
		{"col-2-wide", "col2Wide"},
		// Single letters.
		{"a", "a"},
		{"A", "a"},
		// Unicode letters, as Jet accepts them.
		{"città", "città"},
		{"über-cool", "überCool"},
		{"année-scolaire", "annéeScolaire"},
		// Words that only contain a keyword are fine.
		{"ranges", "ranges"},
		{"range-start", "rangeStart"},
		{"is-true", "isTrue"},
		{"nil-value", "nilValue"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			got, err := TemplateName(test.key, "key")
			if err != nil {
				t.Fatalf("TemplateName(%q) returned error: %v", test.key, err)
			}
			if got != test.want {
				t.Errorf("TemplateName(%q) = %q, want %q", test.key, got, test.want)
			}
		})
	}
}

func TestTemplateNameErrors(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"", `key "" is empty; please give it a name`},
		{"-", `key "-" has an empty word; please remove the extra "-" or "_"`},
		{"-foo", `key "-foo" has an empty word; please remove the extra "-" or "_"`},
		{"_draft", `key "_draft" has an empty word; please remove the extra "-" or "_"`},
		{"og-", `key "og-" has an empty word; please remove the extra "-" or "_"`},
		{"og--image", `key "og--image" has an empty word; please remove the extra "-" or "_"`},
		{"og-_image", `key "og-_image" has an empty word; please remove the extra "-" or "_"`},
		{"2-col", `key "2-col" becomes "2Col" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"404", `key "404" becomes "404" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"og.image", `key "og.image" becomes "og.image" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"og image", `key "og image" becomes "og image" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"og-$image", `key "og-$image" becomes "og$image" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"\xff", `key "\xff" becomes "\xff" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"a\xff", `key "a\xff" becomes "a\xff" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"A\xff", `key "A\xff" becomes "a\xff" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"range", `key "range" is a Jet keyword and cannot be a template variable; please rename it`},
		{"nil", `key "nil" is a Jet keyword and cannot be a template variable; please rename it`},
		{"include", `key "include" is a Jet keyword and cannot be a template variable; please rename it`},
		{"content", `key "content" is a Jet keyword and cannot be a template variable; please rename it`},
		{"true", `key "true" is a Jet keyword and cannot be a template variable; please rename it`},
		{"false", `key "false" is a Jet keyword and cannot be a template variable; please rename it`},
		{"Range", `key "Range" becomes "range" in templates, which is a Jet keyword; please rename it`},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			got, err := TemplateName(test.key, "key")
			if err == nil {
				t.Fatalf("TemplateName(%q) = %q, want an error", test.key, got)
			}
			if err.Error() != test.want {
				t.Errorf("TemplateName(%q) error:\n got %s\nwant %s", test.key, err, test.want)
			}
		})
	}
}

func TestTemplateNameEveryKeyword(t *testing.T) {
	for keyword := range jetKeywords {
		if _, err := TemplateName(keyword, "key"); err == nil {
			t.Errorf("TemplateName(%q) accepted a Jet keyword", keyword)
		}
	}
}

func TestTemplateNameNoun(t *testing.T) {
	_, err := TemplateName("2024-stats", "data file name")
	want := `data file name "2024-stats" becomes "2024Stats" in templates, which is not a valid name (names must start with a letter); please rename it`
	if err == nil || err.Error() != want {
		t.Errorf("error:\n got %v\nwant %s", err, want)
	}
}

func TestCheckReserved(t *testing.T) {
	tests := []struct {
		key  string
		name string
		want string // empty when the name is not reserved
	}{
		// The example of SPEC §4.
		{"site", "site", `front matter key "site" is reserved by Gondola (templates use it for the site settings); please rename it`},
		{"copy", "copy", `front matter key "copy" is reserved by Gondola (templates use it for the page's HTML); please rename it`},
		{"data", "data", `front matter key "data" is reserved by Gondola (templates use it for the data files); please rename it`},
		{"fleet", "fleet", `front matter key "fleet" is reserved by Gondola (templates use it for the collection of all pages); please rename it`},
		{"collections", "collections", `front matter key "collections" is reserved by Gondola (templates use it for the collections of tagged pages); please rename it`},
		{"page", "page", `front matter key "page" is reserved by Gondola (templates use it for the page's URL and metadata); please rename it`},
		// A key that only becomes a reserved name says so.
		{"Site", "site", `front matter key "Site" becomes "site" in templates, which is reserved by Gondola (templates use it for the site settings); please rename it`},
		// Names that only contain a reserved word are fine.
		{"site-name", "siteName", ""},
		{"pages", "pages", ""},
		{"content", "content", ""},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			err := CheckReserved(test.key, test.name, "front matter key")
			if test.want == "" {
				if err != nil {
					t.Fatalf("CheckReserved(%q, %q) returned error: %v", test.key, test.name, err)
				}
				return
			}
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestScope(t *testing.T) {
	tests := []struct {
		name    string
		noun    string
		keys    []string
		want    []string // names for the keys before the failing one
		wantErr string   // empty when every key is accepted
	}{
		{
			name: "distinct keys",
			noun: "key",
			keys: []string{"title", "og-image", "canonical-url"},
			want: []string{"title", "ogImage", "canonicalUrl"},
		},
		{
			name:    "kebab-case and camelCase collide",
			noun:    "front matter key",
			keys:    []string{"robots-agent", "robotsAgent"},
			want:    []string{"robotsAgent"},
			wantErr: `front matter keys "robots-agent" and "robotsAgent" both become "robotsAgent" in templates; please rename one`,
		},
		{
			name:    "kebab-case and snake_case collide",
			noun:    "key",
			keys:    []string{"title", "og_image", "og-image"},
			want:    []string{"title", "ogImage"},
			wantErr: `keys "og_image" and "og-image" both become "ogImage" in templates; please rename one`,
		},
		{
			name:    "case of the first letter collides",
			noun:    "key",
			keys:    []string{"Title", "title"},
			want:    []string{"title"},
			wantErr: `keys "Title" and "title" both become "title" in templates; please rename one`,
		},
		{
			name:    "capitals and lowercase collide",
			noun:    "key",
			keys:    []string{"url-path", "URL-path"},
			want:    []string{"urlPath"},
			wantErr: `keys "url-path" and "URL-path" both become "urlPath" in templates; please rename one`,
		},
		{
			name:    "invalid key",
			noun:    "data file name",
			keys:    []string{"site-info", "2024"},
			want:    []string{"siteInfo"},
			wantErr: `data file name "2024" becomes "2024" in templates, which is not a valid name (names must start with a letter); please rename it`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scope := NewScope(test.noun)
			var got []string
			var gotErr error
			for _, key := range test.keys {
				name, err := scope.Add(key)
				if err != nil {
					gotErr = err
					break
				}
				got = append(got, name)
			}
			if strings.Join(got, ",") != strings.Join(test.want, ",") {
				t.Errorf("names = %q, want %q", got, test.want)
			}
			switch {
			case test.wantErr == "" && gotErr != nil:
				t.Errorf("unexpected error: %v", gotErr)
			case test.wantErr != "" && gotErr == nil:
				t.Errorf("no error, want %s", test.wantErr)
			case test.wantErr != "" && gotErr.Error() != test.wantErr:
				t.Errorf("error:\n got %s\nwant %s", gotErr, test.wantErr)
			}
		})
	}
}

func FuzzTemplateName(f *testing.F) {
	for _, seed := range []string{
		"", "-", "_", "og-image", "og_image", "robotsAgent", "Title", "URL-path",
		"2-col", "og--image", "range", "Range", "città", "\xff", "a\xff", "og.image",
		"URLPath", "HTML5-page", "ID", "日本-X", "ℂ-x", "A\xff",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, key string) {
		name, err := TemplateName(key, "key")
		if err != nil {
			if !strings.HasPrefix(err.Error(), "key ") {
				t.Errorf("error does not start with the noun: %v", err)
			}
			return
		}
		for i, r := range name {
			if i == 0 && !unicode.IsLetter(r) {
				t.Errorf("TemplateName(%q) = %q, which does not start with a letter", key, name)
			}
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
				t.Errorf("TemplateName(%q) = %q, which contains %q", key, name, r)
			}
		}
		if jetKeywords[name] {
			t.Errorf("TemplateName(%q) = %q, which is a Jet keyword", key, name)
		}
		// A converted name has no separators left, so converting it again
		// must change nothing. The exception is a name that still starts
		// with an uppercase letter because the letter has no lowercase form
		// ("ℂ-x" becomes "ℂX", and then "ℂx"); real keys do not do that.
		if first, _ := utf8.DecodeRuneInString(name); unicode.IsUpper(first) {
			return
		}
		again, err := TemplateName(name, "key")
		if err != nil || again != name {
			t.Errorf("TemplateName(%q) = %q, %v; want it unchanged", name, again, err)
		}
	})
}
