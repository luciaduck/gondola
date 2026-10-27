package yamlnode

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// valueNode parses "v: <value>" and returns the node of the value.
func valueNode(t *testing.T, value string) *yaml.Node {
	t.Helper()
	var document yaml.Node
	if err := yaml.Unmarshal([]byte("a: &a 1\nv: "+value+"\n"), &document); err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}
	return document.Content[0].Content[3]
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", true},
		{"~", true},
		{"null", true},
		{"Null", true},
		{`""`, true},
		{"''", true},
		{"!x", true},
		{`!!str ""`, true},
		{"!!null x", true},
		{`"null"`, false},
		{"0", false},
		{"false", false},
		{"[]", false},
		{"{}", false},
		{"!x text", false},
		{"*a", false},
	}
	for _, test := range tests {
		if got := IsEmpty(valueNode(t, test.value)); got != test.want {
			t.Errorf("IsEmpty(%s) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestDescribe(t *testing.T) {
	tests := []struct{ value, want string }{
		{"[a]", "a list"},
		{"{a: 1}", "a map"},
		{"yes", `"yes"`},
		{`"a \"b\""`, `"a \"b\""`},
		{"*a", `"1"`},
	}
	for _, test := range tests {
		if got := Describe(valueNode(t, test.value)); got != test.want {
			t.Errorf("Describe(%s) = %s, want %s", test.value, got, test.want)
		}
	}
}
