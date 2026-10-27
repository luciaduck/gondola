// Package scan walks the project directory and classifies every file: a
// page, a Sass entry point, a passthrough file, or ignored (SPEC §5.6).
package scan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/luciaduck/gondola/internal/config"
)

// The extensions that make a file a page or a Sass entry point. They are
// compared in small letters, so INDEX.HTML is a page too.
var (
	htmlExtensions     = []string{".html", ".htm"}
	markdownExtensions = []string{".md", ".markdown"}
	sassExtensions     = []string{".scss", ".sass"}
)

// cssExtension replaces the extension of a Sass entry point in its output
// path.
const cssExtension = ".css"

// Result is what Scan finds. Each list is in the order of the walk, which
// lists every directory by name, so the same tree always gives the same
// lists.
type Result struct {
	Pages            []Page
	SassEntryPoints  []SassEntryPoint
	PassthroughFiles []PassthroughFile
}

// Page is a source file that becomes a page.
type Page struct {
	// SourcePath is relative to the project root, written with "/".
	SourcePath string
	// ContentPath is relative to the content directory, written with "/".
	// Routing works out the page's output path from it.
	ContentPath string
	// IsMarkdown is true for .md and .markdown files, false for .html and
	// .htm files.
	IsMarkdown bool
}

// SassEntryPoint is a .scss or .sass file that is compiled to CSS.
type SassEntryPoint struct {
	// SourcePath is relative to the project root, written with "/".
	SourcePath string
	// OutputPath is relative to the output directory, written with "/": the
	// path relative to the content directory, ending in ".css".
	OutputPath string
}

// PassthroughFile is a file copied to the output untouched.
type PassthroughFile struct {
	// SourcePath is relative to the project root, written with "/".
	SourcePath string
	// OutputPath is relative to the output directory, written with "/". It
	// is the same as SourcePath: passthrough files keep their place relative
	// to the project root (SPEC §5.6).
	OutputPath string
}

// Scan walks projectDir with the settings of cfg and classifies its files.
//
// Role directories (templates, data, partials, the output and the temp
// output) are never entered. Below the content directory, names that start
// with "." or "_" are skipped; a file that a passthrough pattern matches is
// passed through wherever it is, even there and outside the content
// directory. Symbolic links are not followed: a link that would be used is
// an error. A pattern that matches no file is an error, and so are two
// sources that write the same output file.
func Scan(projectDir string, cfg config.Config) (Result, error) {
	patterns, err := compilePatterns(cfg)
	if err != nil {
		return Result{}, err
	}
	if err := checkContentDir(projectDir, cfg.Input); err != nil {
		return Result{}, err
	}
	w := newWalker(projectDir, cfg, patterns)
	if err := w.walk(nil, w.contentLevels == nil); err != nil {
		return Result{}, err
	}
	if err := w.checkEveryPatternMatched(); err != nil {
		return Result{}, err
	}
	return w.result, nil
}

func compilePatterns(cfg config.Config) ([]*patternUse, error) {
	var patterns []*patternUse
	for _, written := range cfg.Output.PassthroughPatterns {
		compiled, err := compilePattern(written, cfg.IgnoresLetterCase)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, &patternUse{pattern: compiled})
	}
	return patterns, nil
}

// checkContentDir checks that the content directory exists. A symbolic link
// is left to the walk, which refuses it.
func checkContentDir(projectDir string, input config.Input) error {
	info, err := os.Lstat(filepath.Join(projectDir, filepath.FromSlash(input.ContentDir)))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return input.ContentDirPosition.Errorf("input.content %q does not exist", input.ContentDir)
	case err != nil:
		return fmt.Errorf("checking input.content %q: %w", input.ContentDir, err)
	case !info.IsDir() && info.Mode()&fs.ModeSymlink == 0:
		return input.ContentDirPosition.Errorf("input.content %q is a file, not a directory", input.ContentDir)
	}
	return nil
}

// patternUse is a pattern and what the walk found for it.
type patternUse struct {
	pattern     pattern
	matchedFile bool
	// matchedDirectory is the first directory that a pattern without a
	// trailing "/" names, for the message when it matches no file.
	matchedDirectory string
}

// walker holds the settings of one walk and what it has found so far.
type walker struct {
	projectDir string
	// contentLevels are the levels of the content directory's path; nil when
	// it is the project root.
	contentLevels []string
	// roleDirs are never entered: the template, data and partials
	// directories, the output and the temp output.
	roleDirs          []string
	markdown          bool
	sass              bool
	ignoresLetterCase bool
	patterns          []*patternUse
	outputPaths       OutputPaths
	result            Result
}

func newWalker(projectDir string, cfg config.Config, patterns []*patternUse) *walker {
	w := &walker{
		projectDir:        projectDir,
		markdown:          cfg.Settings.Markdown,
		sass:              cfg.Settings.Sass,
		ignoresLetterCase: cfg.IgnoresLetterCase,
		patterns:          patterns,
	}
	if cfg.Input.ContentDir != "." {
		w.contentLevels = strings.Split(cfg.Input.ContentDir, "/")
	}
	input := cfg.Input
	for _, dirs := range [][]string{input.TemplateDirs.Dirs, input.DataDirs.Dirs, input.PartialDirs.Dirs} {
		w.roleDirs = append(w.roleDirs, dirs...)
	}
	w.roleDirs = append(w.roleDirs, cfg.Output.Dir, cfg.Output.TempDir())
	return w
}

// walk lists the directory at dirLevels (relative to the project root; nil
// for the root) and handles each entry. inPageArea is true when the
// directory may hold pages: it is the content directory or inside it, and no
// name below the content directory starts with "." or "_".
func (w *walker) walk(dirLevels []string, inPageArea bool) error {
	dirPath := slashPath(dirLevels)
	entries, err := os.ReadDir(filepath.Join(w.projectDir, filepath.FromSlash(dirPath)))
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", dirPath, err)
	}
	// os.ReadDir sorts the entries by name, which keeps the walk
	// deterministic.
	for _, entry := range entries {
		levels := append(slices.Clone(dirLevels), entry.Name())
		if w.isRoleDir(levels) {
			continue
		}
		entryInPageArea := w.isContentDir(levels) || inPageArea && !isSkippedName(entry.Name())
		if err := w.visit(entry, levels, entryInPageArea); err != nil {
			return err
		}
	}
	return nil
}

func (w *walker) visit(entry fs.DirEntry, levels []string, inPageArea bool) error {
	switch {
	case entry.Type()&fs.ModeSymlink != 0:
		return w.checkLink(levels, inPageArea)
	case entry.IsDir():
		return w.visitDir(levels, inPageArea)
	case entry.Type().IsRegular():
		return w.visitFile(levels, inPageArea)
	}
	// Anything else (a named pipe, a socket, a device) is not a source file,
	// and copying one could block the build.
	return nil
}

func (w *walker) visitDir(levels []string, inPageArea bool) error {
	for _, use := range w.patterns {
		if use.matchedDirectory == "" && use.pattern.matchesDirectoryItself(levels) {
			use.matchedDirectory = slashPath(levels)
		}
	}
	if inPageArea || w.leadsToContentDir(levels) || w.mayPassThroughBelow(levels) {
		return w.walk(levels, inPageArea)
	}
	return nil
}

func (w *walker) visitFile(levels []string, inPageArea bool) error {
	sourcePath := slashPath(levels)
	if w.passesThrough(levels) {
		if err := w.outputPaths.Claim(sourcePath, PassthroughSource, sourcePath); err != nil {
			return err
		}
		w.result.PassthroughFiles = append(w.result.PassthroughFiles, PassthroughFile{SourcePath: sourcePath, OutputPath: sourcePath})
		return nil
	}
	if !inPageArea {
		return nil
	}
	contentPath := slashPath(levels[len(w.contentLevels):])
	extension := strings.ToLower(path.Ext(sourcePath))
	switch {
	case slices.Contains(htmlExtensions, extension):
		w.result.Pages = append(w.result.Pages, Page{SourcePath: sourcePath, ContentPath: contentPath})
	case w.markdown && slices.Contains(markdownExtensions, extension):
		w.result.Pages = append(w.result.Pages, Page{SourcePath: sourcePath, ContentPath: contentPath, IsMarkdown: true})
	case w.sass && slices.Contains(sassExtensions, extension):
		// The extension is cut by its length, not by its letters, so
		// "Main.SCSS" becomes "Main.css".
		outputPath := contentPath[:len(contentPath)-len(extension)] + cssExtension
		if err := w.outputPaths.Claim(outputPath, SassSource, sourcePath); err != nil {
			return err
		}
		w.result.SassEntryPoints = append(w.result.SassEntryPoints, SassEntryPoint{SourcePath: sourcePath, OutputPath: outputPath})
	}
	// Any other file is ignored, .jet files included: templates live in the
	// template directories.
	return nil
}

// checkLink refuses a symbolic link that a rule would use, since Gondola
// does not follow links (SPEC §5.6). A link that would be ignored anyway is
// skipped.
func (w *walker) checkLink(levels []string, inPageArea bool) error {
	sourcePath := slashPath(levels)
	// os.Stat follows the link only to learn whether it points at a
	// directory; nothing behind it is read. A broken link counts as a file.
	target, err := os.Stat(filepath.Join(w.projectDir, filepath.FromSlash(sourcePath)))
	pointsAtDir := err == nil && target.IsDir()
	var used bool
	if pointsAtDir {
		used = inPageArea || w.leadsToContentDir(levels) || w.mayPassThroughBelow(levels)
	} else {
		used = w.passesThrough(levels) || inPageArea && w.isSourceExtension(sourcePath)
	}
	if used {
		return fmt.Errorf("%s is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself", sourcePath)
	}
	return nil
}

// isSourceExtension reports whether a file with this name would be a page
// or a Sass entry point inside the content directory.
func (w *walker) isSourceExtension(name string) bool {
	extension := strings.ToLower(path.Ext(name))
	return slices.Contains(htmlExtensions, extension) ||
		w.markdown && slices.Contains(markdownExtensions, extension) ||
		w.sass && slices.Contains(sassExtensions, extension)
}

// passesThrough reports whether a pattern matches the file at levels, and
// marks every pattern that does.
func (w *walker) passesThrough(levels []string) bool {
	var matched bool
	for _, use := range w.patterns {
		if use.pattern.matchesFile(levels) {
			use.matchedFile = true
			matched = true
		}
	}
	return matched
}

func (w *walker) mayPassThroughBelow(dirLevels []string) bool {
	for _, use := range w.patterns {
		if use.pattern.mayMatchBelow(dirLevels) {
			return true
		}
	}
	return false
}

func (w *walker) checkEveryPatternMatched() error {
	for _, use := range w.patterns {
		switch {
		case use.matchedFile:
			continue
		case use.matchedDirectory != "":
			return use.pattern.position.Errorf(`%s %q matches no file but matches the directory %q; to copy a directory and everything inside it, end the pattern with "/"`,
				passthroughLabel, use.pattern.written, use.matchedDirectory)
		default:
			return use.pattern.position.Errorf("%s %q matches no file (role directories and symbolic links are never copied)", passthroughLabel, use.pattern.written)
		}
	}
	return nil
}

// isRoleDir reports whether the entry at levels is a role directory.
func (w *walker) isRoleDir(levels []string) bool {
	entryPath := w.comparable(slashPath(levels))
	for _, roleDir := range w.roleDirs {
		if entryPath == w.comparable(roleDir) {
			return true
		}
	}
	return false
}

func (w *walker) isContentDir(levels []string) bool {
	return len(levels) == len(w.contentLevels) && w.startsContentDir(levels)
}

// leadsToContentDir reports whether the directory at levels is above the
// content directory, so the walk must go through it to reach the pages.
func (w *walker) leadsToContentDir(levels []string) bool {
	return len(levels) < len(w.contentLevels) && w.startsContentDir(levels)
}

// startsContentDir reports whether levels are the first levels of the
// content directory's path.
func (w *walker) startsContentDir(levels []string) bool {
	for i, name := range levels {
		if w.comparable(name) != w.comparable(w.contentLevels[i]) {
			return false
		}
	}
	return true
}

// comparable returns a name or path in the form that is compared with the
// directories of gondola.yaml: in small letters when the file system ignores
// letter case, as package config compares them.
func (w *walker) comparable(name string) string {
	if w.ignoresLetterCase {
		return strings.ToLower(name)
	}
	return name
}

// isSkippedName reports whether a name below the content directory is
// skipped: names that start with "." or "_", such as .git, _site and Sass
// partials like _variables.scss.
func isSkippedName(name string) bool {
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// slashPath joins path levels with "/"; no levels give ".", the project
// root.
func slashPath(levels []string) string {
	if len(levels) == 0 {
		return "."
	}
	return strings.Join(levels, "/")
}
