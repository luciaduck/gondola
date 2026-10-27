package sourcetext

import "testing"

func TestCheckEncoding(t *testing.T) {
	tests := []struct {
		name    string
		src     []byte
		wantErr bool
	}{
		{"plain", []byte("a: 1\n"), false},
		{"empty", nil, false},
		{"UTF-8 byte order mark", []byte("\xEF\xBB\xBFa: 1\n"), false},
		{"UTF-16 little-endian", []byte("\xFF\xFEa\x00"), true},
		{"UTF-16 big-endian", []byte("\xFE\xFF\x00a"), true},
		{"one byte of a mark", []byte("\xFF"), false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := CheckEncoding(test.src)
			if !test.wantErr {
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				return
			}
			if err == nil || err.Error() != UTF16Message {
				t.Errorf("error %v, want %q", err, UTF16Message)
			}
		})
	}
}

func TestTrimByteOrderMark(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"no mark", "a: 1\n", "a: 1\n"},
		{"mark", "\xEF\xBB\xBFa: 1\n", "a: 1\n"},
		{"only the mark", "\xEF\xBB\xBF", ""},
		{"empty", "", ""},
		{"two marks keep the second", "\xEF\xBB\xBF\xEF\xBB\xBFa", "\xEF\xBB\xBFa"},
		{"mark not at the start", "a\xEF\xBB\xBF", "a\xEF\xBB\xBF"},
		{"part of a mark", "\xEF\xBBa", "\xEF\xBBa"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := string(TrimByteOrderMark([]byte(test.src))); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
