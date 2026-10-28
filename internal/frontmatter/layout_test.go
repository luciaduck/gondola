package frontmatter

import "testing"

func TestCheckLayout(t *testing.T) {
	tests := []struct {
		layout string
		want   string // "" when the layout is valid
	}{
		{"", ""},
		{"nil", ""},
		{"Nil", ""},
		{"page", ""},
		{"page.jet", ""},
		{"page.html", ""},
		{"blog/post", ""},
		{"Città/base", ""},
		{"a..b", ""},
		{".hidden", ""},
		{"..", `layout ".." is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"../x", `layout "../x" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"a/../b", `layout "a/../b" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"a/..", `layout "a/.." is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"/page", `layout "/page" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{"/", `layout "/" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{`a\b`, `layout "a\\b" contains "\\"; please separate directories with "/"`},
		{`..\x`, `layout "..\\x" contains "\\"; please separate directories with "/"`},
		{"./page", `layout "./page" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"a/./b", `layout "a/./b" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{".", `layout "." has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"a//b", `layout "a//b" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"blog/", `layout "blog/" has an empty or "." part; please write it as a path such as "page" or "blog/post"`},
		{"a\tb", `layout "a\tb" contains a control character (U+0009); please remove it`},
		{"a\x00", `layout "a\x00" contains a control character (U+0000); please remove it`},
		{"a\u0085", `layout "a\u0085" contains a control character (U+0085); please remove it`},
		{"caf\xe9", `layout "caf\xe9" is not valid UTF-8; please write it in UTF-8`},
	}
	for _, test := range tests {
		t.Run(test.layout, func(t *testing.T) {
			err := CheckLayout(Key{Written: "layout"}, test.layout)
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

func TestCheckLayoutLabels(t *testing.T) {
	tests := []struct {
		key  Key
		want string
	}{
		{Key{Written: "layout"}, `layout "../x" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{Key{Written: "Layout"}, `Layout "../x" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
		{Key{Written: "layout", InDefault: true}, `default.layout "../x" is not allowed: layout names are relative to the template directories and may not contain ".." or be absolute`},
	}
	for _, test := range tests {
		err := CheckLayout(test.key, "../x")
		if err == nil || err.Error() != test.want {
			t.Errorf("%+v:\n got %v\nwant %s", test.key, err, test.want)
		}
	}
}
