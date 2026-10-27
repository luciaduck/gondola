package frontmatter

import "testing"

func TestCheckPermalink(t *testing.T) {
	tests := []struct {
		permalink string
		want      string // "" when the permalink is valid
	}{
		{"path", ""},
		{"slug", ""},
		{"date", ""},
		{"/", ""},
		{"/about-us/", ""},
		{"/feed.xml", ""},
		{"/about", ""},
		{"/Città di mare/", ""},
		{"/a..b/c...html", ""},
		{"/.well-known/x.json", ""},
		{"//a//b/", ""},
		{"/./a/.", ""},
		{"", `permalink "" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"Path", `permalink "Path" must be "path", "slug" or "date", or a path that begins with "/"`},
		{" /a/", `permalink " /a/" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"https://example.com/a/", `permalink "https://example.com/a/" must be "path", "slug" or "date", or a path that begins with "/"`},
		{"/..", `permalink "/.." is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"/../secret.html", `permalink "/../secret.html" is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"/a/..", `permalink "/a/.." is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{"/a/../", `permalink "/a/../" is not allowed: permalinks may not contain ".." (they must stay inside the output directory)`},
		{`/..\x`, `permalink "/..\\x" contains "\\"; please separate directories with "/"`},
		{"/C:/x.html", `permalink "/C:/x.html" contains ":"; Windows does not allow it in file names; please remove it`},
		{"/a#top", `permalink "/a#top" contains "#"; a permalink names a file in the output, so it cannot have a fragment (the part after "#")`},
		{"/a?b", `permalink "/a?b" contains "?"; a permalink names a file in the output, so it cannot have a query (the part after "?")`},
		{"/a\nb", `permalink "/a\nb" contains a control character (U+000A); please remove it`},
		{"/a\x00", `permalink "/a\x00" contains a control character (U+0000); please remove it`},
		{"/a\x7f", `permalink "/a\x7f" contains a control character (U+007F); please remove it`},
		{"/a\u0085", `permalink "/a\u0085" contains a control character (U+0085); please remove it`},
		{"/\x8d", `permalink "/\x8d" is not valid UTF-8; please write it in UTF-8`},
		{"caf\xe9", `permalink "caf\xe9" is not valid UTF-8; please write it in UTF-8`},
		{"/a\xff\n", `permalink "/a\xff\n" is not valid UTF-8; please write it in UTF-8`},
		{"/\uFFFD", ""},
	}
	for _, test := range tests {
		t.Run(test.permalink, func(t *testing.T) {
			err := CheckPermalink(test.permalink)
			if test.want == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestIsCustomPermalink(t *testing.T) {
	tests := []struct {
		permalink string
		want      bool
	}{
		{"path", false},
		{"slug", false},
		{"date", false},
		{"", false},
		{"/", true},
		{"/about-us/", true},
	}
	for _, test := range tests {
		if got := IsCustomPermalink(test.permalink); got != test.want {
			t.Errorf("IsCustomPermalink(%q) = %v, want %v", test.permalink, got, test.want)
		}
	}
}
