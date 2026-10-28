// Package engine is everything about the template language: it finds the
// layouts of a page, runs a page body and its layouts through Jet, and turns
// Jet's errors into messages that point into the user's files (SPEC §5.11).
//
// Only Jet exists today. Package build will define the small interface it
// needs and use the concrete Jet type through it; another template engine
// would be a second type next to it, not a registry.
package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/CloudyKit/jet/v6"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/scan"
)

// templateExtensions are the extensions tried after a template name, in
// order, both by Jet for include, import and extends, and by the layout
// lookup (without the first, empty one), so that `include "header"` and
// `layout: header` find the same file. A .md fragment must be named in full.
var templateExtensions = []string{"", ".jet", ".html"}

// FencedBlockRewriter rewrites the fenced blocks of a Markdown text into
// HTML. *parser.Parser implements it; the engine uses it for the Markdown
// fragments that pages include.
type FencedBlockRewriter interface {
	Squero(file string, body []byte, bodyLineOffset int) ([]byte, error)
}

// Options are what New needs to know about the project.
type Options struct {
	// ProjectDir is the directory that holds gondola.yaml.
	ProjectDir string
	// TemplateDirs and TemplateDirPositions are input.templates, with where
	// each entry is written in gondola.yaml (config.Input).
	TemplateDirs         config.DirList
	TemplateDirPositions []config.Position
	// FencedBlocks rewrites the fenced blocks of included .md fragments. It
	// is nil when config.fenced-blocks is off, and the fragments are then
	// used as written.
	FencedBlocks FencedBlockRewriter
}

// Jet renders pages and layouts with the Jet template engine. Build one per
// build with New: it holds a Jet set and the layouts it has read, so a new
// build always sees the files as they are now. It is not safe for concurrent
// use, because builds render pages one after the other.
type Jet struct {
	set    *jet.Set
	loader *loader
	// layouts are the layouts read so far, by their file path, so that each
	// layout file is read and split once per build.
	layouts map[string]*Layout
}

// New checks the template directories and returns a Jet for one build. A
// template directory written in gondola.yaml must exist; the default one is
// skipped when it does not.
func New(options Options) (*Jet, error) {
	templateDirs, err := existingTemplateDirs(options)
	if err != nil {
		return nil, err
	}
	loader := &loader{
		projectDir:     options.ProjectDir,
		templateDirs:   templateDirs,
		configuredDirs: options.TemplateDirs.Dirs,
		fromDefault:    options.TemplateDirs.FromDefault,
		fencedBlocks:   options.FencedBlocks,
		opened:         map[string]string{},
	}
	// Jet's development mode is not used: a new set per build already makes
	// edits visible between two `serve` rebuilds (SPEC §5.11).
	set := jet.NewSet(loader, jet.WithTemplateNameExtensions(templateExtensions))
	return &Jet{set: set, loader: loader, layouts: map[string]*Layout{}}, nil
}

// existingTemplateDirs returns the template directories that exist, in the
// order written, with "/".
func existingTemplateDirs(options Options) ([]string, error) {
	dirs := []string{}
	for i, dir := range options.TemplateDirs.Dirs {
		var position config.Position
		if i < len(options.TemplateDirPositions) {
			position = options.TemplateDirPositions[i]
		}
		exists, err := checkTemplateDir(options.ProjectDir, dir, options.TemplateDirs.FromDefault, position)
		if err != nil {
			return nil, err
		}
		if exists {
			dirs = append(dirs, dir)
		}
	}
	return dirs, nil
}

// checkTemplateDir reports whether dir, a template directory relative to the
// project root, exists. A default directory that does not exist is skipped;
// a directory written in gondola.yaml must exist and must be a directory,
// and the error points at position.
func checkTemplateDir(projectDir, dir string, fromDefault bool, position config.Position) (bool, error) {
	subject := fmt.Sprintf("input.templates %q", dir)
	if fromDefault {
		subject += " (the default)"
	}
	info, err := os.Lstat(filepath.Join(projectDir, filepath.FromSlash(dir)))
	if errors.Is(err, fs.ErrNotExist) {
		if fromDefault {
			return false, nil
		}
		return false, position.Errorf("%s does not exist", subject)
	}
	if err != nil {
		return false, fmt.Errorf("checking %s: %w", subject, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return false, scan.LinkError(dir)
	}
	if !info.IsDir() {
		return false, position.Errorf("%s is a file, not a directory", subject)
	}
	return true, nil
}

// searchedDirs names the template directories for a message about a
// template that was not found: `_components/includes (the default)` or
// `layouts, shared/templates`.
func (l *loader) searchedDirs() string {
	text := strings.Join(l.templateDirs, ", ")
	if l.fromDefault {
		text += " (the default)"
	}
	return text
}

// noTemplateDirHint ends a message about a template that was not found when
// the project has no template directory at all: the default one does not
// exist, or gondola.yaml says templates: [].
func (l *loader) noTemplateDirHint() string {
	if l.fromDefault {
		return fmt.Sprintf("the project has no template directory; please create %s or set input.templates in %s", strings.Join(l.configuredDirs, ", "), config.FileName)
	}
	return fmt.Sprintf("the project has no template directory; please set input.templates in %s", config.FileName)
}

// errorInFile returns a diag error that names file without a position.
func errorInFile(file, format string, args ...any) error {
	return &diag.Error{File: file, Msg: fmt.Sprintf(format, args...)}
}
