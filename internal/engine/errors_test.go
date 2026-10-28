package engine

import "testing"

func TestRewriteRemovesAddresses(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"could not parse  (string) into 0xc000012345 (*int64)", "could not parse  (string) into *int64"},
		{"could not parse abc (string) into 0x33703f5e35d0 (*float64)", "could not parse abc (string) into *float64"},
		{"trying to parse x into 0xc0000aa000: unhandled value type *jet.thing", "trying to parse x into a value: unhandled value type *jet.thing"},
		// Text that only looks like an address is left alone.
		{"unexpected 0xff (number)", "unexpected 0xff (number)"},
		{`could not find "0x10 (*int)"`, `could not find "0x10 (*int)"`},
	}
	engine := newProject(t, nil)
	run := engine.newRun(testPage(), newVariables(testGlobals(), testPage()))
	for _, test := range tests {
		if got := run.rewrite(test.message); got != test.want {
			t.Errorf("%q:\n got %q\nwant %q", test.message, got, test.want)
		}
	}
}

func TestCutLine(t *testing.T) {
	tests := []struct {
		text     string
		wantLine int
		wantRest string
		wantOK   bool
	}{
		{":12): x", 12, "x", true},
		{":0): x", 0, "x", true},
		{":): x", 0, "", false},
		{":-1): x", 0, "", false},
		{":12)x", 0, "", false},
		{"12): x", 0, "", false},
		{":1a): x", 0, "", false},
	}
	for _, test := range tests {
		line, rest, ok := cutLine(test.text, ":", "): ")
		if line != test.wantLine || rest != test.wantRest || ok != test.wantOK {
			t.Errorf("%q: got %d, %q, %v; want %d, %q, %v", test.text, line, rest, ok, test.wantLine, test.wantRest, test.wantOK)
		}
	}
}
