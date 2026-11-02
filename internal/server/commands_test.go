package server

import (
	"strings"
	"testing"
)

func TestReadCommands(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantRebuild bool
		wantUnknown bool
		wantQuit    bool
	}{
		{name: "g asks for a build", input: "g\n", wantRebuild: true},
		{name: "upper case and spaces are accepted", input: "  G \t\n", wantRebuild: true},
		{name: "several g are one build", input: "g\ng\ng\n", wantRebuild: true},
		{name: "q closes", input: "q\n", wantQuit: true},
		{name: "upper case Q closes", input: " Q\n", wantQuit: true},
		{name: "lines after q are not read", input: "q\ng\nx\n", wantQuit: true},
		{name: "g before q is still pending", input: "g\nq\n", wantRebuild: true, wantQuit: true},
		{name: "an empty line is unknown", input: "\n", wantUnknown: true},
		{name: "another word is unknown", input: "go\n", wantUnknown: true},
		{name: "several unknown lines print the help once", input: "x\n\ny\n", wantUnknown: true},
		{name: "the last line needs no line break", input: "g", wantRebuild: true},
		{name: "Windows line ends", input: "g\r\nq\r\n", wantRebuild: true, wantQuit: true},
		{name: "the end of input does not close", input: "", wantQuit: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			received := newCommands()
			readCommands(strings.NewReader(test.input), received)
			if got := len(received.rebuild) == 1; got != test.wantRebuild {
				t.Errorf("build pending = %v, want %v", got, test.wantRebuild)
			}
			if got := len(received.unknown) == 1; got != test.wantUnknown {
				t.Errorf("help pending = %v, want %v", got, test.wantUnknown)
			}
			if got := isClosed(received.quit); got != test.wantQuit {
				t.Errorf("quit closed = %v, want %v", got, test.wantQuit)
			}
		})
	}
}

// isClosed reports whether the channel is closed, without waiting.
func isClosed(channel chan struct{}) bool {
	select {
	case <-channel:
		return true
	default:
		return false
	}
}
