// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package data

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/sourcetext"
	"github.com/luciaduck/gondola/internal/yamlnode"
)

// emptyFileValue is what an empty data file holds: no error, so a file can
// sit in a data directory as a placeholder, and "" as for an empty front
// matter value, so templates can still name the key.
const emptyFileValue = ""

// The words that name a data file in messages from package yamlnode.
const (
	// yamlDocumentName completes "… must hold only one" for a second document.
	yamlDocumentName = "a data file"
	// yamlWhere completes "key "x" appears twice in …" and "keys in … must
	// be text".
	yamlWhere = "the data file"
)

// jsonWhitespace are the characters JSON allows between values (RFC 8259).
// A file made only of them is empty.
const jsonWhitespace = " \t\r\n"

// notUTF8Message is the error for a JSON file with bytes that are not UTF-8.
// encoding/json would quietly replace them with U+FFFD; yaml.v3 refuses
// them itself.
const notUTF8Message = "the file is not valid UTF-8; please save it as UTF-8"

// parseFunc parses the text of a data file, without its byte order mark.
// Errors are *diag.Error values without a file name.
type parseFunc func(text []byte) (any, error)

// parserFor returns the parser for a data file name, by its extension in
// any letter case, and false for a file that is not a data file.
func parserFor(name string) (parseFunc, bool) {
	switch strings.ToLower(path.Ext(name)) {
	case ".json":
		return parseJSON, true
	case ".yaml", ".yml":
		return parseYAML, true
	}
	// Another format, such as TOML, would add its extension here.
	return nil, false
}

// parseFile applies the Encoding rule of SPEC §4 and parses src with parse.
// Errors name sourcePath.
func parseFile(sourcePath string, src []byte, parse parseFunc) (any, error) {
	if err := sourcetext.CheckEncoding(src); err != nil {
		return nil, diag.WithFile(err, sourcePath)
	}
	// Positions never count the byte order mark, and encoding/json would
	// refuse it as an invalid character.
	value, err := parse(sourcetext.TrimByteOrderMark(src))
	if err != nil {
		return nil, diag.WithFile(err, sourcePath)
	}
	return value, nil
}

// parseYAML parses a YAML data file with the rules of front matter values
// (yamlnode.DecodeValue): every map is a map[string]any with its keys as
// written, a key written twice is an error, and times never take the
// computer's zone.
func parseYAML(text []byte) (any, error) {
	source := yamlnode.Source{Text: text}
	root, err := yamlnode.ParseDocument(text, yamlDocumentName)
	if err != nil {
		return nil, source.ToFilePosition(err)
	}
	// A file that is empty, holds only comments, or holds only null.
	if root == nil || yamlnode.IsEmpty(root) {
		return emptyFileValue, nil
	}
	value, err := yamlnode.DecodeValue(root, yamlWhere, source)
	if err != nil {
		return nil, source.ToFilePosition(err)
	}
	return value, nil
}

// parseJSON parses a JSON data file with encoding/json. As encoding/json
// does, a key written twice keeps its last value. Whole numbers become int,
// as they do in YAML; other numbers are float64.
func parseJSON(text []byte) (any, error) {
	if !utf8.Valid(text) {
		return nil, &diag.Error{Err: errors.New(notUTF8Message)}
	}
	if len(bytes.Trim(text, jsonWhitespace)) == 0 {
		return emptyFileValue, nil
	}
	// The file is read twice. json.Unmarshal checks it and gives the byte
	// offset of any problem; a json.Decoder with UseNumber then reads the
	// numbers as written, so whole numbers can become int instead of
	// float64 (which would print 1000000 as 1e+06 in templates). Unmarshal
	// has no UseNumber, and the Decoder's errors are less precise.
	var checked any
	if err := json.Unmarshal(text, &checked); err != nil {
		return nil, jsonError(text, err)
	}
	if checked == nil {
		// The file holds only null.
		return emptyFileValue, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decoding JSON already checked: %w", err)
	}
	return withGoNumbers(value), nil
}

// jsonError turns an error of json.Unmarshal into a *diag.Error at the line
// and column of the problem.
func jsonError(text []byte, err error) error {
	// errors.As finds the error of the type that its second argument points
	// to, and sets it, so its fields can be read.
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		// Offset counts the bytes read when the problem was found, so the
		// offending character is the one that ends there. A file cut short
		// ends there too, so the error points at its last character.
		return errorAtByte(text, startOfCharacterBefore(text, syntaxErr.Offset), "invalid JSON: %s", syntaxErr.Error())
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		// Decoding into an any fails this way only for a number too large
		// for a float64, such as 1e999. Value is "number 1e999" and Offset
		// counts the bytes up to the end of the number.
		start := startOfCharacterBefore(text, typeErr.Offset)
		if literal, isNumber := strings.CutPrefix(typeErr.Value, "number "); isNumber && int64(len(literal)) <= typeErr.Offset {
			start = int(typeErr.Offset) - len(literal)
		}
		return errorAtByte(text, start, "invalid JSON: %s is out of range", typeErr.Value)
	}
	return fmt.Errorf("parsing JSON: %w", err)
}

// startOfCharacterBefore returns the index of the first byte of the
// character that ends at offset, or 0 at the start of text.
func startOfCharacterBefore(text []byte, offset int64) int {
	end := int(min(max(offset, 0), int64(len(text))))
	if end == 0 {
		return 0
	}
	_, size := utf8.DecodeLastRune(text[:end])
	return end - size
}

// errorAtByte returns a *diag.Error at the line and column of the byte at
// index in text, counted as package diag counts: lines end at "\n" and
// columns count bytes.
func errorAtByte(text []byte, index int, format string, args ...any) error {
	before := text[:index]
	line := 1 + bytes.Count(before, []byte("\n"))
	col := index - (bytes.LastIndexByte(before, '\n') + 1) + 1
	return &diag.Error{Line: line, Col: col, Err: fmt.Errorf(format, args...)}
}

// withGoNumbers returns value with every json.Number in it replaced by an
// int when it is written as a whole number that fits in one, or by a
// float64 otherwise. Maps and lists are changed in place; they were just
// decoded and are not shared yet.
func withGoNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		return goNumber(typed)
	case map[string]any:
		for key, item := range typed {
			typed[key] = withGoNumbers(item)
		}
	case []any:
		for i, item := range typed {
			typed[i] = withGoNumbers(item)
		}
	}
	return value
}

func goNumber(number json.Number) any {
	written := number.String()
	// 1.0 and 1e6 are written as fractional numbers, so they stay float64,
	// as they do in YAML.
	if !strings.ContainsAny(written, ".eE") {
		if whole, err := strconv.Atoi(written); err == nil {
			return whole
		}
	}
	// json.Unmarshal has already refused numbers out of the float64 range,
	// so the error is always nil here.
	fractional, _ := number.Float64()
	return fractional
}
