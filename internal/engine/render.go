// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine

import (
	"bytes"
	"fmt"
	"path"

	"github.com/CloudyKit/jet/v6"
	"github.com/luciaduck/gondola/internal/diag"
)

// internalNameMarker starts the base name under which the engine gives Jet
// a page body or a layout. The loader refuses names with control
// characters, so no template file can have such a name, and an error that
// Jet reports at this name is known to come from the page body or the
// layout, whose lines are shifted by their front matter, and not from a
// template file of the same name opened through the loader.
const internalNameMarker = "\x00"

// pageTemplateName is the name under which the page body is given to Jet.
// It has no directory, so a relative include in a page starts at the root
// of the template directories, wherever the page is (SPEC §5.11).
const pageTemplateName = "/" + internalNameMarker + "page"

// RenderBody runs body, the body of page after its fenced blocks were
// rewritten, through Jet: the first round of SPEC §3.2. bodyLineOffset is
// the number of lines of the page file before body, so that errors count
// lines in the whole file. Do not call it for a page with layout: nil.
func (j *Jet) RenderBody(page Page, globals Globals, body []byte, bodyLineOffset int) ([]byte, error) {
	run := j.newRun(page, newVariables(globals, page))
	run.addSource(pageTemplateName, page.SourcePath, bodyLineOffset)
	template, err := run.parse(pageTemplateName, body)
	if err != nil {
		return nil, err
	}
	return run.execute(template, pageTemplateName)
}

// WrapInLayouts runs the layouts of page, nearest first as LayoutChain
// returns them, each with copy set to the result of the one before, starting
// with copyHTML: the second round of SPEC §3.2. It returns the final page.
func (j *Jet) WrapInLayouts(page Page, globals Globals, layouts []*Layout, copyHTML []byte) ([]byte, error) {
	pageVariables := newVariables(globals, page)
	result := copyHTML
	for _, layout := range layouts {
		run := j.newRun(page, pageVariables.withCopy(string(result)))
		name := layoutTemplateName(layout)
		run.addSource(name, layout.File, layout.Document.BodyLineOffset)
		if layout.template == nil {
			template, err := run.parse(name, layout.Document.Body)
			if err != nil {
				return nil, err
			}
			layout.template = template
		}
		output, err := run.execute(layout.template, name)
		if err != nil {
			return nil, err
		}
		result = output
	}
	return result, nil
}

// layoutTemplateName is the name under which layout is given to Jet: its
// own directory, so that relative includes start next to it, and a base
// name that no template file can have (internalNameMarker).
func layoutTemplateName(layout *Layout) string {
	return path.Join(path.Dir(layout.templatePath), internalNameMarker+path.Base(layout.templatePath))
}

// run is one Jet execution: the page it is for, its variables, and where
// the templates it may report errors in come from.
type run struct {
	jet       *Jet
	page      Page
	variables variables
	// sources maps the names given to Jet with Parse to their files.
	// Templates opened through the loader are found in loader.opened.
	sources map[string]source
}

// source is a file that Jet knows under some name, with the number of lines
// of the file before the text that Jet parsed (its front matter).
type source struct {
	file       string
	lineOffset int
}

func (j *Jet) newRun(page Page, values variables) *run {
	j.loader.startRender()
	return &run{jet: j, page: page, variables: values, sources: map[string]source{}}
}

func (r *run) addSource(name, file string, lineOffset int) {
	r.sources[name] = source{file: file, lineOffset: lineOffset}
}

// parse parses text under name.
func (r *run) parse(name string, text []byte) (template *jet.Template, err error) {
	defer r.recoverPanic(name, &err)
	template, err = r.jet.set.Parse(name, string(text))
	if err != nil {
		return nil, r.translate(err, name)
	}
	if err := r.ignoredFailure(name); err != nil {
		return nil, err
	}
	return template, nil
}

// execute runs template, which was parsed under name, with the variables.
func (r *run) execute(template *jet.Template, name string) (output []byte, err error) {
	defer r.recoverPanic(name, &err)
	var buffer bytes.Buffer
	if err := template.Execute(&buffer, r.variables.values, nil); err != nil {
		return nil, r.translate(err, name)
	}
	if err := r.ignoredFailure(name); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// recoverPanic turns a panic inside Jet into an error naming the template
// that was running. Jet returns its own errors, but it lets Go's runtime
// errors (such as a nil pointer) go on panicking, and a mistake in a
// template must never stop Gondola with a crash.
func (r *run) recoverPanic(name string, err *error) {
	recovered := recover()
	if recovered == nil {
		return
	}
	file := r.sourceFile(name)
	*err = &diag.Error{File: file, Msg: fmt.Sprintf("Jet stopped unexpectedly (%v); this is a bug in Jet or in Gondola, please report it%s", recovered, renderingNote(r.page.SourcePath, file))}
}

// sourceFile returns the file of the template that Jet knows as name.
func (r *run) sourceFile(name string) string {
	if source, ok := r.sources[name]; ok {
		return source.file
	}
	if file, ok := r.jet.loader.opened[name]; ok {
		return file
	}
	return r.page.SourcePath
}
