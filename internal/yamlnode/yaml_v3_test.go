package yamlnode

import (
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// These tests record how yaml.v3 itself decodes free-form values, which is
// why DecodeValue exists (SPEC §5.4, spike for M2). If one fails after an
// upgrade of yaml.v3, check whether DecodeValue is still needed.

func TestYAMLv3NestedMapTypes(t *testing.T) {
	var value map[string]any
	src := "author: {name: Lucia}\ncounts: {1: one}\n"
	if err := yaml.Unmarshal([]byte(src), &value); err != nil {
		t.Fatal(err)
	}
	if _, ok := value["author"].(map[string]any); !ok {
		t.Errorf("a map with text keys decoded as %T, want map[string]any", value["author"])
	}
	// One key that is not text is enough for the general map type, which
	// templates cannot read with author.name.
	if _, ok := value["counts"].(map[any]any); !ok {
		t.Errorf("a map with a number key decoded as %T, want map[interface{}]interface{}", value["counts"])
	}
}

func TestYAMLv3TimesTakeTheLocalZone(t *testing.T) {
	// time.Parse gives a time the computer's own zone when the offset
	// matches it, so the zone's name would depend on where Gondola runs.
	saved := time.Local
	t.Cleanup(func() { time.Local = saved })
	time.Local = time.FixedZone("CEST", 2*60*60)

	var value map[string]any
	if err := yaml.Unmarshal([]byte("at: 2024-05-01T10:00:00+02:00\n"), &value); err != nil {
		t.Fatal(err)
	}
	at, ok := value["at"].(time.Time)
	if !ok {
		t.Fatalf("decoded as %T, want time.Time", value["at"])
	}
	if name, _ := at.Zone(); name != "CEST" {
		t.Errorf("zone name %q, want the local zone CEST", name)
	}
}

func TestYAMLv3TimeWithoutZoneIsUTC(t *testing.T) {
	var value map[string]any
	if err := yaml.Unmarshal([]byte("at: 2024-05-01 10:00:00\n"), &value); err != nil {
		t.Fatal(err)
	}
	at, ok := value["at"].(time.Time)
	if !ok || at.Location() != time.UTC {
		t.Errorf("decoded as %#v, want a time in UTC", value["at"])
	}
}
