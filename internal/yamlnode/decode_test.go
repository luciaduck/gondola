package yamlnode

import (
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// decodeV parses src, a YAML map with the key "v", and decodes the value of
// v with DecodeValue. The text sits after one line, as front matter does.
func decodeV(t *testing.T, src string) (any, error) {
	t.Helper()
	root, err := ParseDocument([]byte(src), "test")
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "v" {
			source := Source{Text: []byte(src), LineOffset: 1}
			value, err := DecodeValue(root.Content[i+1], `front matter key "v"`, source)
			return value, source.ToFilePosition(err)
		}
	}
	t.Fatal("no key v")
	return nil, nil
}

func TestDecodeValue(t *testing.T) {
	utcNoon := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		src  string
		want any
	}{
		{"text", "v: hello\n", "hello"},
		{"number", "v: 3\n", 3},
		{"float", "v: 0.10\n", 0.1},
		{"boolean", "v: true\n", true},
		{"list", "v: [a, 1]\n", []any{"a", 1}},
		{"nested map", "v: {name: Lucia, links: {site: x}}\n", map[string]any{"name": "Lucia", "links": map[string]any{"site": "x"}}},
		{"number key as written", "v: {1: one, 0x10: hex, 1.50: f}\n", map[string]any{"1": "one", "0x10": "hex", "1.50": "f"}},
		{"boolean and null keys", "v: {true: a, ~: b, null: c}\n", map[string]any{"true": "a", "~": "b", "null": "c"}},
		{"keys inside a list", "v: [{2: x}]\n", []any{map[string]any{"2": "x"}}},
		{"alias to a map", "m: &m {1: x}\nv: *m\n", map[string]any{"1": "x"}},
		{"alias as a key", "k: &k 7\nv: {*k : seven}\n", map[string]any{"7": "seven"}},
		{"anchored key keeps its type elsewhere", "v: {&k 7 : seven, again: *k}\n", map[string]any{"7": "seven", "again": 7}},
		{"merge key", "base: &b {a: 1, b: 2}\nv: {<<: *b, b: 3}\n", map[string]any{"a": 1, "b": 3}},
		{"time in UTC", "v: 2024-05-01T12:00:00Z\n", utcNoon},
		{"time without zone", "v: 2024-05-01 12:00:00\n", utcNoon},
		{"nested time", "v: {at: [2024-05-01T12:00:00Z]}\n", map[string]any{"at": []any{utcNoon}}},
		{"null inside a list", "v: [a, ~]\n", []any{"a", nil}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := decodeV(t, test.src)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got  %#v\nwant %#v", got, test.want)
			}
		})
	}
}

func TestDecodeValueTimeWithOffset(t *testing.T) {
	// Even when the offset matches the computer's zone, the time gets a
	// zone without a name, so formatting it gives the same text everywhere.
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	time.Local = time.FixedZone("CEST", 2*60*60)

	got, err := decodeV(t, "v: 2024-05-01T10:00:00+02:00\n")
	if err != nil {
		t.Fatal(err)
	}
	at := got.(time.Time)
	if formatted := at.Format("2006-01-02 15:04 MST"); formatted != "2024-05-01 10:00 +0200" {
		t.Errorf("formatted as %q", formatted)
	}
}

func TestDecodeValueErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"key twice", "v:\n  name: a\n  name: b\n", `4:3: key "name" appears twice in front matter key "v" (first at line 3); please keep only one`},
		{"key twice deeper", "v: {a: {x: 1, x: 2}}\n", `2:15: key "x" appears twice in front matter key "v" (first at line 2); please keep only one`},
		{"number and text key", "v: {1: a, \"1\": b}\n", `2:11: key "1" appears twice in front matter key "v" (first at line 2); please keep only one`},
		{"key twice after a lone CR", "x: 1\rv:\n  a: 1\n  a: 2\n", `4:3: key "a" appears twice in front matter key "v" (first at line 3); please keep only one`},
		{"list as key", "v: {[a]: 1}\n", `2:5: keys in front matter key "v" must be text, not a list`},
		{"map as key", "v:\n  ? {a: 1}\n  : x\n", `3:5: keys in front matter key "v" must be text, not a map`},
		{"wrong explicit tag", "v: !!int abc\n", "2:4: invalid YAML: cannot decode !!str `abc` as a !!int"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeV(t, test.src)
			if err == nil || err.Error() != test.want {
				t.Errorf("error:\n got %v\nwant %s", err, test.want)
			}
		})
	}
}

func TestDecodeValueRepeatedAliasesStayFast(t *testing.T) {
	// Each anchor is walked once, however many aliases point to it; yaml.v3
	// itself refuses the expansion when decoding.
	src := `a: &a ["x","x","x","x","x","x","x","x","x"]
b: &b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
c: &c [*b,*b,*b,*b,*b,*b,*b,*b,*b]
d: &d [*c,*c,*c,*c,*c,*c,*c,*c,*c]
e: &e [*d,*d,*d,*d,*d,*d,*d,*d,*d]
f: &f [*e,*e,*e,*e,*e,*e,*e,*e,*e]
g: &g [*f,*f,*f,*f,*f,*f,*f,*f,*f]
v: [*g,*g,*g,*g,*g,*g,*g,*g,*g]
`
	_, err := decodeV(t, src)
	want := "9:4: invalid YAML: document contains excessive aliasing"
	if err == nil || err.Error() != want {
		t.Errorf("error %v, want %s", err, want)
	}
}

func TestDecodeValueChangesOnlyKeys(t *testing.T) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("v: {1: 2}\n"), &document); err != nil {
		t.Fatal(err)
	}
	mapping := document.Content[0].Content[1]
	if _, err := DecodeValue(mapping, "v", Source{}); err != nil {
		t.Fatal(err)
	}
	if key, value := mapping.Content[0], mapping.Content[1]; key.Tag != stringTag || value.ShortTag() != "!!int" {
		t.Errorf("key tag %q, value tag %q", key.Tag, value.ShortTag())
	}
}
