package yamlnode

import (
	"errors"
	"testing"

	"github.com/luciaduck/gondola/internal/diag"
	"gopkg.in/yaml.v3"
)

func TestToFilePosition(t *testing.T) {
	tests := []struct {
		name string
		src  string
		// The position as yaml.v3 counts it: lines split at any of its line
		// breaks, columns in characters.
		yamlLine, yamlCol int
		// The position as Gondola counts it: lines split at "\n", columns in
		// bytes.
		wantLine, wantCol int
	}{
		{"ascii", "ab\ncd\n", 2, 2, 2, 2},
		{"first line", "abc\n", 1, 3, 1, 3},
		{"after accents", "ab\nàè x\n", 2, 4, 2, 6},
		{"column 1", "ab\nàè x\n", 2, 1, 2, 1},
		{"column past the end of the line", "ab\nàè x\n", 2, 40, 2, 7},
		{"line past the end", "ab\n", 9, 3, 2, 1},
		{"no final line break", "ab\ncd", 2, 3, 2, 3},
		{"past the end without final break", "ab\ncd", 2, 9, 2, 3},
		{"CRLF", "ab\r\ncd\r\n", 2, 2, 2, 2},
		{"lone CR", "ab\rcd\rxy\n", 3, 2, 1, 8},
		{"NEL", "ab\u0085cd\n", 2, 2, 1, 6},
		{"LS", "ab\u2028cd\nxy\n", 3, 1, 2, 1},
		{"PS", "a\u2029é\u2029x\n", 3, 1, 1, 10},
		{"LS after accents", "è\u2028x\n", 2, 1, 1, 6},
		{"empty text", "", 1, 1, 1, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := Source{Text: []byte(test.src)}
			err := &diag.Error{File: "x.yaml", Line: test.yamlLine, Col: test.yamlCol, Msg: "x"}
			got, ok := source.ToFilePosition(err).(*diag.Error)
			want := diag.Error{File: "x.yaml", Line: test.wantLine, Col: test.wantCol, Msg: "x"}
			if !ok || *got != want {
				t.Errorf("ToFilePosition(%d:%d) = %+v, want %+v", test.yamlLine, test.yamlCol, got, want)
			}
			// The same text after an opening line, as in front matter: only
			// the line moves.
			shifted := Source{Text: []byte(test.src), LineOffset: 1}
			got, ok = shifted.ToFilePosition(err).(*diag.Error)
			want.Line++
			if !ok || *got != want {
				t.Errorf("with LineOffset 1: %+v, want %+v", got, want)
			}
			if line := shifted.FileLine(test.yamlLine); line != test.wantLine+1 {
				t.Errorf("FileLine(%d) = %d, want %d", test.yamlLine, line, test.wantLine+1)
			}
		})
	}
}

func TestToFilePositionLeavesOthersAlone(t *testing.T) {
	source := Source{Text: []byte("a\u2028b\n")}
	// No column: only the line is converted.
	got := source.ToFilePosition(&diag.Error{Line: 2, Msg: "x"}).(*diag.Error)
	if want := (diag.Error{Line: 1, Msg: "x"}); *got != want {
		t.Errorf("without a column: %+v, want %+v", got, want)
	}
	// No line: unchanged.
	noLine := &diag.Error{Msg: "x"}
	if got := source.ToFilePosition(noLine); got != noLine {
		t.Errorf("without a line: %v changed", got)
	}
	plain := errors.New("plain")
	if got := source.ToFilePosition(plain); got != plain {
		t.Errorf("a plain error changed: %v", got)
	}
	if got := source.ToFilePosition(nil); got != nil {
		t.Errorf("nil became %v", got)
	}
}

func TestSyntaxError(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"syntax", "a: [\n", "1:1: invalid YAML: did not find expected node content"},
		// yaml.v3 gives no line for this one.
		{"type error", "a: !!int x\n", "invalid YAML: cannot decode !!str `x` as a !!int"},
		{"tab indentation", "a:\n\tb: 1\n", "2:1: invalid YAML: found character that cannot start any token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var value any
			err := yaml.Unmarshal([]byte(test.src), &value)
			if err == nil {
				t.Fatal("yaml.v3 accepted the input")
			}
			if got := SyntaxError(err).Error(); got != test.want {
				t.Errorf("got  %s\nwant %s", got, test.want)
			}
		})
	}
	// A message without a position keeps no position.
	if got := SyntaxError(errors.New("yaml: something")).Error(); got != "invalid YAML: something" {
		t.Errorf("without a line: %s", got)
	}
}

func TestParseDocument(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantKind yaml.Kind // 0 means no root
		wantErr  string
	}{
		{"empty", "", 0, ""},
		{"only a comment", "# nothing\n", 0, ""},
		{"map", "a: 1\n", yaml.MappingNode, ""},
		{"list", "- a\n", yaml.SequenceNode, ""},
		{"second document", "a: 1\n---\nb: 2\n", 0, `2:1: a second YAML document starts here; the file must hold only one, so please remove it`},
		{"document end then a second document", "a: 1\n...\n---\nb: 2\n", 0, `3:1: a second YAML document starts here; the file must hold only one, so please remove it`},
		{"document end then text", "a: 1\n...\nb: 2\n", 0, `2:1: invalid YAML: did not find expected <document start>`},
		{"syntax error", "a: [\n", 0, "1:1: invalid YAML: did not find expected node content"},
		{"syntax error in the second document", "a: 1\n---\nb: [\n", 0, "3:1: invalid YAML: did not find expected node content"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := ParseDocument([]byte(test.src), "the file")
			if test.wantErr != "" {
				if err == nil || err.Error() != test.wantErr {
					t.Errorf("error:\n got %v\nwant %s", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.wantKind == 0 {
				if root != nil {
					t.Errorf("root %v, want none", root.Kind)
				}
				return
			}
			if root == nil || root.Kind != test.wantKind {
				t.Errorf("root %+v, want kind %v", root, test.wantKind)
			}
		})
	}
}
