package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

// These tests prove V5 of SPEC §8: how yaml.v3 treats null values and
// pre-filled maps when it decodes into Go values. They test yaml.v3, not
// Gondola. They are the reason Parse walks yaml.Node values instead of
// decoding into structs: a null would wipe a default list or map.

// prefilled is a struct whose fields hold defaults before decoding.
type prefilled struct {
	Text   string         `yaml:"text"`
	Switch bool           `yaml:"switch"`
	Number int            `yaml:"number"`
	List   []string       `yaml:"list"`
	Map    map[string]any `yaml:"map"`
}

func newPrefilled() prefilled {
	return prefilled{
		Text:   "default",
		Switch: true,
		Number: 7,
		List:   []string{"default"},
		Map:    map[string]any{"built-in": "default"},
	}
}

func TestYAMLv3NullKeepsScalarsButClearsListsAndMaps(t *testing.T) {
	for _, src := range []string{
		"text: null\nswitch: null\nnumber: null\nlist: null\nmap: null\n",
		"text: ~\nswitch: ~\nnumber: ~\nlist: ~\nmap: ~\n",
		"text:\nswitch:\nnumber:\nlist:\nmap:\n",
	} {
		got := newPrefilled()
		if err := yaml.Unmarshal([]byte(src), &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", src, err)
		}
		// Scalars keep their defaults.
		if got.Text != "default" || !got.Switch || got.Number != 7 {
			t.Errorf("Unmarshal(%q): scalars = %q, %v, %d; want the defaults", src, got.Text, got.Switch, got.Number)
		}
		// Lists and maps are set to nil: the defaults are lost.
		if got.List != nil || got.Map != nil {
			t.Errorf("Unmarshal(%q): list = %#v, map = %#v; want both nil", src, got.List, got.Map)
		}
	}
}

func TestYAMLv3AbsentKeysKeepDefaults(t *testing.T) {
	for _, src := range []string{"", "~", "number: 8\n"} {
		got := newPrefilled()
		if err := yaml.Unmarshal([]byte(src), &got); err != nil {
			t.Fatalf("Unmarshal(%q): %v", src, err)
		}
		want := newPrefilled()
		if src == "number: 8\n" {
			want.Number = 8
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Unmarshal(%q) = %#v, want %#v", src, got, want)
		}
	}
}

func TestYAMLv3MergesIntoPrefilledMap(t *testing.T) {
	got := newPrefilled()
	src := "map: {user: 1}\nlist: [user]\n"
	if err := yaml.Unmarshal([]byte(src), &got); err != nil {
		t.Fatal(err)
	}
	// A map is merged: the built-in entry stays and the user's is added.
	wantMap := map[string]any{"built-in": "default", "user": 1}
	if !reflect.DeepEqual(got.Map, wantMap) {
		t.Errorf("map = %#v, want %#v", got.Map, wantMap)
	}
	// A list is replaced, not appended to.
	if !reflect.DeepEqual(got.List, []string{"user"}) {
		t.Errorf("list = %#v, want [user]", got.List)
	}
}

func TestYAMLv3NullInsideMapReplacesEntry(t *testing.T) {
	got := newPrefilled()
	if err := yaml.Unmarshal([]byte("map: {built-in: null}\n"), &got); err != nil {
		t.Fatal(err)
	}
	// The entry is kept but its value becomes nil: the default is lost.
	value, present := got.Map["built-in"]
	if !present || value != nil {
		t.Errorf(`map["built-in"] = %#v (present: %v), want nil and present`, value, present)
	}
}
