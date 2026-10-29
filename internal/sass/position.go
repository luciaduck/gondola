package sass

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf16"
)

// sourcePosition returns the line (from 1) and the byte column (from 1) of
// the character that starts at utf16Offset in text, the way package diag
// counts. Dart Sass counts offsets in UTF-16 code units, from 0, because Dart
// strings are UTF-16: "😀" is two units, four bytes in Go. ok is false when
// the offset lies past the end of text. An offset at the very end is valid,
// as for an error at the end of the file.
//
// Lines end at "\n" only, as the console counts them when it prints the
// line; "\r\n" works because the "\r" stays at the end of the line.
func sourcePosition(text string, utf16Offset int) (line, col int, ok bool) {
	if utf16Offset < 0 {
		return 0, 0, false
	}
	line = 1
	lineStart := 0
	units := 0
	// Ranging over a string decodes UTF-8; an invalid byte comes out as
	// utf8.RuneError, one unit long, as Dart Sass replaces it with U+FFFD.
	for byteIndex, r := range text {
		if units >= utf16Offset {
			return line, byteIndex - lineStart + 1, true
		}
		units += utf16.RuneLen(r)
		if r == '\n' {
			line++
			lineStart = byteIndex + 1
		}
	}
	if units >= utf16Offset {
		return line, len(text) - lineStart + 1, true
	}
	return 0, 0, false
}

// lineColumn returns the byte column (from 1) of the character at
// utf16Column (from 0) on line lineIndex (from 0) of text, for the positions
// in Dart Sass's log events, which give a line and a column instead of an
// offset. ok is false when the line or the column does not exist.
func lineColumn(text string, lineIndex, utf16Column int) (col int, ok bool) {
	if lineIndex < 0 {
		return 0, false
	}
	lines := strings.Split(text, "\n")
	if lineIndex >= len(lines) {
		return 0, false
	}
	line, col, ok := sourcePosition(lines[lineIndex], utf16Column)
	if !ok || line != 1 {
		return 0, false
	}
	return col, true
}

// fileURL returns the file: URL of absolutePath, which Dart Sass needs to
// resolve the imports of a stylesheet given as text relative to its file.
func fileURL(absolutePath string) string {
	slashed := filepath.ToSlash(absolutePath)
	// On Windows the path starts with a drive letter, and a file: URL wants
	// a "/" before it: file:///C:/site/main.scss.
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}

// pathFromFileURL returns the file system path of a file: URL written by Dart
// Sass, or false for another kind of URL.
func pathFromFileURL(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}
	return pathFromURLPath(parsed.Path), true
}

// pathFromURLPath turns the path of a file: URL, already unescaped, into a
// file system path.
func pathFromURLPath(urlPath string) string {
	if runtime.GOOS == "windows" {
		// "/C:/site/main.scss" becomes "C:/site/main.scss".
		urlPath = strings.TrimPrefix(urlPath, "/")
	}
	return filepath.FromSlash(urlPath)
}
