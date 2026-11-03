// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/luciaduck/gondola/internal/diag"
)

// Jet's errors are plain text. These are the parts of that text that the
// engine reads to find where an error is and what it is about (checked
// against jet/v6 v6.3.3 by jet_v6_test.go).
const (
	// jetRuntimePrefix starts an error raised while a template runs:
	// `Jet Runtime Error ("/page.jet":12): message`. The name is quoted as
	// Go quotes strings.
	jetRuntimePrefix = "Jet Runtime Error ("
	// jetParsePrefix starts an error raised while a template is parsed:
	// `template: /page.jet:12: message`. The name is not quoted.
	jetParsePrefix = "template: "
	// jetUndefinedPrefix and jetUndefinedSuffix surround the name of an
	// undefined variable: `identifier "titel" not available in current
	// (map[…]) or parent scope, global, or default variables`. The part in
	// parentheses dumps every variable, data files included, so the message
	// is replaced.
	jetUndefinedPrefix = "identifier "
	jetUndefinedSuffix = " not available in current ("
	// jetNotFoundPrefix and jetNotFoundSuffix surround the path of a
	// template that no template directory holds: `template /blog/x could
	// not be found`.
	jetNotFoundPrefix = "template "
	jetNotFoundSuffix = " could not be found"
)

// jetPointerMessages match the messages in which Jet prints the address of
// one of its own variables (Arguments.ParseInto: `could not parse "" (string)
// into 0xc000012345 (*int64)`). The address changes on every run, and the
// same input must give the same message, so rewrite removes it. Only these
// two shapes are changed, so text that the user wrote is never touched.
// The compiled expressions are never changed, so they are shared safely.
var jetPointerMessages = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`^(could not parse .* into )0x[0-9a-f]+ \((\*[^)]*)\)$`), "${1}${2}"},
	{regexp.MustCompile(`^(trying to parse .* into )0x[0-9a-f]+(: unhandled value type .*)$`), "${1}a value${2}"},
}

// location is where a Jet error is: a file and a line in it, 0 when
// unknown.
type location struct {
	file string
	line int
}

// translate turns an error from Jet into a *diag.Error that names the file
// and line where it is, with a message for the user. topName is the name of
// the template that the run parsed or executed: an error that does not say
// where it is is reported in its file, without a line.
func (r *run) translate(err error, topName string) error {
	failures := r.jet.loader.failures
	for i := len(failures) - 1; i >= 0; i-- {
		var diagErr *diag.Error
		if errors.As(failures[i], &diagErr) && strings.Contains(err.Error(), failures[i].Error()) {
			return r.withRenderingNote(diagErr)
		}
	}
	where, message := r.locate(err.Error(), location{file: r.sourceFile(topName)})
	return &diag.Error{File: where.file, Line: where.line, Msg: r.rewrite(message) + renderingNote(r.page.SourcePath, where.file)}
}

// locate removes from text the prefixes that Jet puts in front of a message
// to say where it is, and returns the innermost place they name. Jet nests
// them: an error in a template that another one includes reads
// `Jet Runtime Error ("/page.jet":3): template: /x.jet:2: message`.
func (r *run) locate(text string, where location) (location, string) {
	for {
		name, line, rest, ok := r.cutJetPrefix(text)
		if !ok {
			return where, text
		}
		if source, known := r.source(name); known {
			where = location{file: source.file}
			// Jet counts lines from 1; it reports 0 for a few errors that
			// have no line, such as a division by zero.
			if line > 0 {
				where.line = line + source.lineOffset
			}
		}
		text = rest
	}
}

// cutJetPrefix removes one of Jet's position prefixes from the start of
// text and returns the template name and line it holds.
func (r *run) cutJetPrefix(text string) (name string, line int, rest string, ok bool) {
	if after, found := strings.CutPrefix(text, jetRuntimePrefix); found {
		quoted, err := strconv.QuotedPrefix(after)
		if err != nil {
			return "", 0, "", false
		}
		name, err = strconv.Unquote(quoted)
		if err != nil {
			return "", 0, "", false
		}
		line, rest, ok = cutLine(strings.TrimPrefix(after, quoted), ":", "): ")
		return name, line, rest, ok
	}
	if after, found := strings.CutPrefix(text, jetParsePrefix); found {
		// The name is not quoted and may hold a ":", so it is matched
		// against the names that this run knows, longest first.
		for _, known := range r.knownNames() {
			if afterName, found := strings.CutPrefix(after, known); found {
				if line, rest, ok := cutLine(afterName, ":", ": "); ok {
					return known, line, rest, true
				}
			}
		}
	}
	return "", 0, "", false
}

// cutLine reads a line number between before and after at the start of
// text: cutLine(":12): x", ":", "): ") is 12 and "x".
func cutLine(text, before, after string) (int, string, bool) {
	text, found := strings.CutPrefix(text, before)
	if !found {
		return 0, "", false
	}
	digits, rest, found := strings.Cut(text, after)
	if !found {
		return 0, "", false
	}
	line, err := strconv.Atoi(digits)
	if err != nil || line < 0 {
		return 0, "", false
	}
	return line, rest, true
}

// source returns the file that Jet knows as name.
func (r *run) source(name string) (source, bool) {
	if found, ok := r.sources[name]; ok {
		return found, true
	}
	if file, ok := r.jet.loader.opened[name]; ok {
		return source{file: file}, true
	}
	return source{}, false
}

// knownNames returns the template names of this run, longest first, so
// that a name is never mistaken for a shorter one that starts it.
func (r *run) knownNames() []string {
	names := []string{}
	for name := range r.sources {
		names = append(names, name)
	}
	for name := range r.jet.loader.opened {
		names = append(names, name)
	}
	sort.Slice(names, func(i, k int) bool {
		if len(names[i]) != len(names[k]) {
			return len(names[i]) > len(names[k])
		}
		return names[i] < names[k]
	})
	return names
}

// rewrite replaces the Jet messages that would not help the user; the
// others are kept as Jet writes them.
func (r *run) rewrite(message string) string {
	if name, ok := undefinedVariable(message); ok {
		return fmt.Sprintf("variable %q is not defined; the available variables are %s", name, strings.Join(r.variables.names, ", "))
	}
	if templatePath, ok := missingTemplate(message); ok {
		return r.templateNotFound(templatePath)
	}
	for _, pointerMessage := range jetPointerMessages {
		message = pointerMessage.pattern.ReplaceAllString(message, pointerMessage.replacement)
	}
	return message
}

// undefinedVariable returns the name in Jet's message about an undefined
// variable.
func undefinedVariable(message string) (string, bool) {
	after, found := strings.CutPrefix(message, jetUndefinedPrefix)
	if !found {
		return "", false
	}
	quoted, err := strconv.QuotedPrefix(after)
	if err != nil || !strings.HasPrefix(after[len(quoted):], jetUndefinedSuffix) {
		return "", false
	}
	name, err := strconv.Unquote(quoted)
	if err != nil {
		return "", false
	}
	return name, true
}

// missingTemplate returns the path in Jet's message about a template that
// was not found.
func missingTemplate(message string) (string, bool) {
	after, found := strings.CutPrefix(message, jetNotFoundPrefix)
	if !found {
		return "", false
	}
	templatePath, found := strings.CutSuffix(after, jetNotFoundSuffix)
	return templatePath, found
}

// templateNotFound is the message for an include, import or extends whose
// template no template directory holds.
func (r *run) templateNotFound(templatePath string) string {
	name := templateName(templatePath)
	loader := r.jet.loader
	if len(loader.templateDirs) == 0 {
		return fmt.Sprintf("template %q was not found, because %s", name, loader.noTemplateDirHint())
	}
	if path.Ext(name) != "" {
		return fmt.Sprintf("template %q was not found in %s", name, loader.searchedDirs())
	}
	tried := []string{}
	for _, extension := range templateExtensions {
		tried = append(tried, name+extension)
	}
	return fmt.Sprintf("template %q was not found; looked for %s and %s in %s", name, strings.Join(tried[:len(tried)-1], ", "), tried[len(tried)-1], loader.searchedDirs())
}

// ignoredFailure returns the first error of the loader in a render that Jet
// finished without an error, because includeIfExists (or a try block)
// ignored it, or nil when there was none.
func (r *run) ignoredFailure(topName string) error {
	failures := r.jet.loader.failures
	if len(failures) == 0 {
		return nil
	}
	var diagErr *diag.Error
	if errors.As(failures[0], &diagErr) {
		return r.withRenderingNote(diagErr)
	}
	file := r.sourceFile(topName)
	return errorInFile(file, "%s%s", failures[0], renderingNote(r.page.SourcePath, file))
}

// withRenderingNote returns diagErr, an error that already names its file
// (such as an error in a Markdown fragment), with the page being rendered
// added to its message.
func (r *run) withRenderingNote(diagErr *diag.Error) error {
	note := renderingNote(r.page.SourcePath, diagErr.File)
	if note == "" {
		return diagErr
	}
	message := diagErr.Msg
	if message == "" && diagErr.Err != nil {
		message = diagErr.Err.Error()
	}
	noted := *diagErr
	noted.Msg = message + note
	return &noted
}
