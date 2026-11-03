// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package engine

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/scan"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// markdownFragmentExtension marks the included templates whose fenced
// blocks are rewritten before Jet reads them (SPEC §5.11).
const markdownFragmentExtension = ".md"

// loader gives Jet the templates of the template directories. Jet calls it
// for include, import and extends with an absolute path. Jet cleans a
// relative name against its own root ("../../x" arrives as "/x"), but it
// passes an absolute name as written ("/../x" arrives as "/../x"), so the
// loader cleans every path the same way before it looks for the file
// (templateName), and no name can leave the template directories.
//
// It implements jet.Loader (Exists and Open).
type loader struct {
	projectDir string
	// templateDirs are the template directories that exist, in the order
	// written, relative to the project root with "/".
	templateDirs []string
	// configuredDirs and fromDefault are input.templates as written, for
	// messages.
	configuredDirs []string
	fromDefault    bool
	fencedBlocks   FencedBlockRewriter
	// opened maps each template path that Jet has opened, such as
	// "/snippets/x.md", to its file relative to the project root, so that
	// Jet's errors can name the file.
	opened map[string]string
	// parsing lists the templates that are open now (by templateName),
	// outermost first. Jet
	// closes a template only after it has parsed it together with the
	// templates it extends or imports, so opening one that is still open is
	// a cycle. Jet itself does not notice and recurses until the Go stack
	// overflows, which ends the program without a chance to recover.
	parsing []string
	// failures are the errors that Open returned during this render, in
	// order. Jet keeps only the text of the errors it receives, so the
	// renderer returns the error itself, which keeps the file and position
	// of an error inside a Markdown fragment. Jet's includeIfExists also
	// ignores a template that exists but cannot be opened; the renderer
	// reports it anyway, because Gondola never fails silently.
	failures []error
}

// startRender forgets what the previous render left behind.
func (l *loader) startRender() {
	l.parsing = nil
	l.failures = nil
}

// Exists reports whether templatePath names a template. It also reports true
// for a name that Open will refuse, such as a symbolic link or a name with a
// "\", so that Open can say why instead of Jet saying "not found".
func (l *loader) Exists(templatePath string) bool {
	if l.nameProblem(templatePath) != nil {
		return true
	}
	_, found, err := l.find(templatePath)
	return found || err != nil
}

// Open returns the contents of the template at templatePath. A Markdown
// fragment comes back with its fenced blocks already rewritten.
func (l *loader) Open(templatePath string) (io.ReadCloser, error) {
	text, err := l.read(templatePath)
	if err != nil {
		l.failures = append(l.failures, err)
		return nil, err
	}
	// The stack holds cleaned names, so that "/../a.jet" and "/a.jet" are
	// seen as the same template.
	l.parsing = append(l.parsing, templateName(templatePath))
	return &openTemplate{Reader: bytes.NewReader(text), close: l.closeTemplate, path: templatePath}, nil
}

// read returns the text of the template at templatePath, ready for Jet.
func (l *loader) read(templatePath string) ([]byte, error) {
	if err := l.nameProblem(templatePath); err != nil {
		return nil, err
	}
	if slices.Contains(l.parsing, templateName(templatePath)) {
		return nil, l.cycleError(templatePath)
	}
	file, found, err := l.find(templatePath)
	if err != nil {
		return nil, err
	}
	if !found {
		// Jet always calls Exists first, so this happens only if the file
		// was removed in between.
		return nil, fmt.Errorf("template %q was not found", templateName(templatePath))
	}
	src, err := os.ReadFile(filepath.Join(l.projectDir, filepath.FromSlash(file)))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	if err := sourcetext.CheckEncoding(src); err != nil {
		return nil, diag.WithFile(err, file)
	}
	// The byte order mark is removed from every template, not only from
	// fragments: Jet would copy it into the middle of the page.
	text := sourcetext.TrimByteOrderMark(src)
	l.opened[templatePath] = file
	if l.fencedBlocks != nil && strings.HasSuffix(templatePath, markdownFragmentExtension) {
		// A fragment has no front matter, so its line 1 is line 1 of the
		// file.
		return l.fencedBlocks.Squero(file, text, 0)
	}
	return text, nil
}

// closeTemplate is called by Jet when it has parsed templatePath.
func (l *loader) closeTemplate(templatePath string) {
	if i := slices.Index(l.parsing, templateName(templatePath)); i >= 0 {
		l.parsing = slices.Delete(l.parsing, i, i+1)
	}
}

// cycleError names the templates that extend or import each other, in the
// order Jet opened them: "a.jet → b.jet → a.jet".
func (l *loader) cycleError(templatePath string) error {
	name := templateName(templatePath)
	start := slices.Index(l.parsing, name)
	names := slices.Clone(l.parsing[start:])
	names = append(names, name)
	return fmt.Errorf("the templates form a cycle: %s; please remove one of the extends or import statements", strings.Join(names, " → "))
}

// nameProblem returns an error when templatePath cannot name a file on
// every platform.
func (l *loader) nameProblem(templatePath string) error {
	name := templateName(templatePath)
	if !utf8.ValidString(name) {
		return fmt.Errorf("template name %q is not valid UTF-8; please write it in UTF-8", name)
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return fmt.Errorf("template name %q contains a control character (%U); please remove it", name, character)
		}
	}
	// On Windows Jet turns "\" into "/" before the loader sees it; elsewhere
	// the name is refused, so that a site does not build on one system only.
	if strings.Contains(name, `\`) {
		return fmt.Errorf(`template name %q contains "\\"; please separate directories with "/"`, name)
	}
	return nil
}

// find looks for templatePath in the template directories, in order, and
// returns the first file found, relative to the project root with "/". A
// directory of that name does not count. A symbolic link on the way is an
// error, because Gondola does not follow links.
func (l *loader) find(templatePath string) (string, bool, error) {
	name := templateName(templatePath)
	if name == "" {
		return "", false, nil
	}
	for _, dir := range l.templateDirs {
		file, found, err := findInDir(l.projectDir, dir, name)
		if err != nil || found {
			return file, found, err
		}
	}
	return "", false, nil
}

// findInDir looks for name, a path with "/", in the template directory dir.
// Each part of the path is checked with os.Lstat, which does not follow
// links, so a linked directory on the way is noticed too.
func findInDir(projectDir, dir, name string) (string, bool, error) {
	file := dir
	parts := strings.Split(name, "/")
	for i, part := range parts {
		file = path.Join(file, part)
		info, err := os.Lstat(filepath.Join(projectDir, filepath.FromSlash(file)))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf("looking for template %q: %w", name, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", false, scan.LinkError(file)
		}
		last := i == len(parts)-1
		if last && info.IsDir() {
			return "", false, nil
		}
		if !last && !info.IsDir() {
			// "header.jet/x" when header.jet is a file.
			return "", false, nil
		}
	}
	return file, true, nil
}

// displayedName is a template path as messages show it: Jet's
// "/snippets/x.md" becomes "snippets/x.md".
func displayedName(templatePath string) string {
	return strings.TrimPrefix(templatePath, "/")
}

// templateName returns the name of the file that templatePath stands for
// inside the template directories: cleaned against the root as Jet cleans
// relative names, so "/../x.md" is "x.md", and without the leading "/".
func templateName(templatePath string) string {
	return displayedName(path.Clean("/" + templatePath))
}

// openTemplate is the io.ReadCloser that Open returns. Embedding
// *bytes.Reader gives it Read; Close tells the loader that Jet has finished
// parsing the template.
type openTemplate struct {
	*bytes.Reader
	close func(templatePath string)
	path  string
}

func (t *openTemplate) Close() error {
	t.close(t.path)
	return nil
}
