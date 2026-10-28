package parser

import (
	"testing"

	"github.com/yuin/goldmark/ast"
)

func TestHeadingIDs(t *testing.T) {
	tests := []struct {
		name     string
		put      []string
		headings []string
		want     []string
	}{
		{"words", nil, []string{"Hello World"}, []string{"hello-world"}},
		{"accents are transliterated", nil, []string{"Città"}, []string{"citta"}},
		{"Italian", nil, []string{"Perché sì, però no"}, []string{"perche-si-pero-no"}},
		{"other scripts are transliterated", nil, []string{"你好"}, []string{"ni-hao"}},
		{"ampersand", nil, []string{"Q&A"}, []string{"qanda"}},
		// Jet escapes the variables it writes into a page, so a heading may
		// hold entities; they are decoded before the slug is made.
		{"escaped ampersand", nil, []string{"Fish &amp; chips"}, []string{"fish-and-chips"}},
		{"numeric entities", nil, []string{"Caf&#233; &#x3C;b&#x3E;"}, []string{"cafe-b"}},
		{"named accent", nil, []string{"Citt&agrave;"}, []string{"citta"}},
		{"escaped quote", nil, []string{"&#34;Quoted&#34;"}, []string{"quoted"}},
		{"not an entity", nil, []string{"AT&T &amp"}, []string{"atandt-and"}},
		{"Markdown is written as is", nil, []string{"**Bold** `code` text"}, []string{"bold-code-text"}},
		{"underscores are kept", nil, []string{"snake_case"}, []string{"snake_case"}},
		{"surrounding spaces and dashes", nil, []string{"  - spaced -  "}, []string{"spaced"}},
		{"digits first", nil, []string{"2024 review"}, []string{"2024-review"}},
		{"nothing left", nil, []string{"!!!"}, []string{"heading"}},
		{"empty", nil, []string{""}, []string{"heading"}},
		{"nothing left twice", nil, []string{"!!!", "???"}, []string{"heading", "heading-1"}},
		{
			"repeated headings",
			nil,
			[]string{"Notes", "Notes", "Notes"},
			[]string{"notes", "notes-1", "notes-2"},
		},
		{
			"a heading that looks like a suffix",
			nil,
			[]string{"Notes", "Notes 1", "Notes"},
			[]string{"notes", "notes-1", "notes-2"},
		},
		{
			"different text, same id",
			nil,
			[]string{"Città", "citta", "CITTÀ"},
			[]string{"citta", "citta-1", "citta-2"},
		},
		{"ids written by hand are not reused", []string{"intro"}, []string{"Intro"}, []string{"intro-1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ids := newHeadingIDs()
			for _, id := range test.put {
				ids.Put([]byte(id))
			}
			for i, heading := range test.headings {
				if got := string(ids.Generate([]byte(heading), ast.KindHeading)); got != test.want[i] {
					t.Errorf("heading %q: got %q, want %q", heading, got, test.want[i])
				}
			}
		})
	}
}
