package scan

import "testing"

func TestOutputPathsClaim(t *testing.T) {
	type claim struct {
		outputPath string
		kind       SourceKind
		sourcePath string
		want       string // the error; empty when the claim is accepted
	}
	tests := []struct {
		name   string
		claims []claim
	}{
		{"different paths", []claim{
			{"a.css", SassSource, "a.scss", ""},
			{"b.css", PassthroughSource, "b.css", ""},
			{"blog/a.css", SassSource, "blog/a.scss", ""},
			{"index.html", PageSource, "index.md", ""},
		}},
		{"same path", []claim{
			{"a.css", PassthroughSource, "a.css", ""},
			{"a.css", SassSource, "src/a.scss", `passthrough file "a.css" and Sass entry point "src/a.scss" both write "a.css" in the output; please rename or remove one`},
		}},
		{"page", []claim{
			{"about.html", PassthroughSource, "about.html", ""},
			{"about.html", PageSource, "src/about.md", `passthrough file "about.html" and page "src/about.md" both write "about.html" in the output; please rename or remove one`},
		}},
		{"letter case only", []claim{
			{"img/Logo.png", PassthroughSource, "img/Logo.png", ""},
			{"img/logo.png", PassthroughSource, "img/logo.png", `passthrough file "img/Logo.png" and passthrough file "img/logo.png" both write "img/Logo.png" and "img/logo.png", which differ only in letter case, in the output; please rename or remove one`},
		}},
		{"letter case in a directory", []claim{
			{"Blog/a.css", SassSource, "Blog/a.scss", ""},
			{"blog/a.css", SassSource, "blog/a.sass", `Sass entry point "Blog/a.scss" and Sass entry point "blog/a.sass" both write "Blog/a.css" and "blog/a.css", which differ only in letter case, in the output; please rename or remove one`},
		}},
		{"the first claim is named", []claim{
			{"a.css", SassSource, "a.sass", ""},
			{"a.css", SassSource, "a.scss", `Sass entry point "a.sass" and Sass entry point "a.scss" both write "a.css" in the output; please rename or remove one`},
			{"A.CSS", PassthroughSource, "A.CSS", `Sass entry point "a.sass" and passthrough file "A.CSS" both write "a.css" and "A.CSS", which differ only in letter case, in the output; please rename or remove one`},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The zero value is ready to use.
			var outputPaths OutputPaths
			for _, c := range test.claims {
				err := outputPaths.Claim(c.outputPath, c.kind, c.sourcePath)
				switch {
				case c.want == "" && err != nil:
					t.Errorf("Claim(%q): %v", c.outputPath, err)
				case c.want != "" && (err == nil || err.Error() != c.want):
					t.Errorf("Claim(%q) error:\n got %v\nwant %s", c.outputPath, err, c.want)
				}
			}
		})
	}
}
