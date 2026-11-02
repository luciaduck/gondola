package server

import (
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"
)

// maxFuzzInputLength keeps fuzz inputs far below bufio.Scanner's limit of
// 64 KiB per line, past which readCommands stops reading; that case is not
// what these targets check.
const maxFuzzInputLength = 4096

// commandSeeds are typical inputs: commands, unknown lines, line ends of
// every kind, and text after q.
var commandSeeds = []string{
	"", "g\n", "q\n", "x\n", "\n", "G\n", " q \n", "g\r\n", "g\ng\ng\n",
	"g\nq\n", "q\ng\n", "x\ng\n", "gg\n", "g", "\r\n", "g\rq\n", "\tg\t\n",
}

// expectedCommands is what readCommands should leave after reading input,
// worked out without a scanner: the input is split at "\n", a "\r" before
// it is dropped, and a final "\n" does not start another line.
func expectedCommands(input string) (rebuild, unknown, quit bool) {
	lines := strings.Split(input, "\n")
	if strings.HasSuffix(input, "\n") || input == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		word := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(line, "\r")))
		switch word {
		case "g":
			rebuild = true
		case "q":
			return rebuild, unknown, true
		default:
			unknown = true
		}
	}
	return rebuild, unknown, false
}

// readAll runs readCommands on input and reports what it left.
func readAll(input io.Reader) (rebuild, unknown, quit bool) {
	received := newCommands()
	readCommands(input, received)
	return len(received.rebuild) == 1, len(received.unknown) == 1, isClosed(received.quit)
}

// FuzzReadCommands checks readCommands against expectedCommands, with the
// input read at once, one byte at a time, and in halves, as a terminal may
// deliver it.
//
//	go test ./internal/server -run '^$' -fuzz FuzzReadCommands -fuzztime 10m
func FuzzReadCommands(f *testing.F) {
	for _, seed := range commandSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxFuzzInputLength {
			t.Skip()
		}
		wantRebuild, wantUnknown, wantQuit := expectedCommands(input)
		readers := map[string]io.Reader{
			"at once":    strings.NewReader(input),
			"byte":       iotest.OneByteReader(strings.NewReader(input)),
			"in halves":  iotest.HalfReader(strings.NewReader(input)),
			"data error": iotest.DataErrReader(strings.NewReader(input)),
		}
		for name, reader := range readers {
			rebuild, unknown, quit := readAll(reader)
			if rebuild != wantRebuild || unknown != wantUnknown || quit != wantQuit {
				t.Errorf("%s: %q left rebuild %v, unknown %v, quit %v; want %v, %v, %v",
					name, input, rebuild, unknown, quit, wantRebuild, wantUnknown, wantQuit)
			}
		}
	})
}

// FuzzCanaryReadCommands is written to fail: it claims that no input both
// asks for a build and closes the server. "g" followed by "q" on the next
// line proves it wrong, which shows that the fuzzer's inputs reach both
// commands. It is skipped unless GONDOLA_FUZZ_CANARY is set, so that go test
// stays green with the failing input saved under testdata/fuzz:
//
//	GONDOLA_FUZZ_CANARY=1 go test ./internal/server -run '^$' -fuzz FuzzCanaryReadCommands -fuzztime 1m
func FuzzCanaryReadCommands(f *testing.F) {
	if os.Getenv("GONDOLA_FUZZ_CANARY") == "" {
		f.Skip("the canary fails on purpose; set GONDOLA_FUZZ_CANARY to run it")
	}
	// Only seeds that keep the claim true: the fuzzer has to find the
	// counterexample itself.
	for _, seed := range []string{"g\n", "q\n", "x\n", "", "G\r\n"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > maxFuzzInputLength {
			t.Skip()
		}
		rebuild, _, quit := readAll(strings.NewReader(input))
		if rebuild && quit {
			t.Errorf("%q asked for a build and closed the server", input)
		}
	})
}
