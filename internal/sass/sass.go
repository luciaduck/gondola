// Package sass compiles the Sass entry points found by package scan into CSS
// files in the temp output, through the Dart Sass executable driven by
// godartsass (SPEC §5.10). Imports are resolved next to the importing file
// first, then in the partials directories (the Sass load paths).
package sass

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bep/godartsass/v2"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/scan"
	"github.com/luciaduck/gondola/internal/sourcetext"
)

// OutputStyle is how the CSS is written.
type OutputStyle int

const (
	// Compressed CSS, for gondola ride.
	Compressed OutputStyle = iota + 1
	// Expanded CSS, one declaration per line, for gondola serve.
	Expanded
)

// indentedSyntaxExtension marks a source in Sass's indented syntax; every
// other entry point is SCSS. scan accepts the extension in any letter case.
const indentedSyntaxExtension = ".sass"

// Options configures a Compiler.
type Options struct {
	// ProjectDir is the project root; the paths of Input are relative to it.
	ProjectDir string
	// ExecutablePath is the Dart Sass executable returned by FindExecutable.
	ExecutablePath string
	// Input holds the partials directories and where they are written.
	Input config.Input
	Style OutputStyle
	// PrintWarning receives each warning of Dart Sass (@warn, @debug and
	// deprecations), already formatted, once per build.
	PrintWarning func(text string)
	// IgnoresLetterCase is config.Config.IgnoresLetterCase: whether the
	// project's file system ignores letter case.
	IgnoresLetterCase bool
}

// Compiler compiles the entry points of one build. Create it with Start and
// close it with Close.
type Compiler struct {
	projectDir string
	// resolvedProjectDir is projectDir with symbolic links resolved and, on
	// Windows, short names such as RUNNER~1 expanded: Dart Sass reports the
	// partials it finds by their long names. It is "" when it cannot be
	// found or equals projectDir.
	resolvedProjectDir string
	// ignoresLetterCase lets displayedPath find the project in other
	// letters: on macOS and Windows Dart Sass reports the partials it finds
	// in their letters on disk, which may differ from how the project
	// directory was written (Site for site), and on macOS filepath.Rel
	// compares letters exactly.
	ignoresLetterCase bool
	partialDirs       []string // absolute paths
	style             godartsass.OutputStyle
	printWarning      func(text string)
	transpiler        *godartsass.Transpiler

	// printedWarnings holds the warnings printed in this build, so that a
	// partial used by several entry points does not repeat its warnings.
	printedWarnings map[string]bool

	// pendingEvents are the log events of the compilation that runs.
	// godartsass calls the handler on its own goroutine, so the events are
	// kept here and printed by Compile, on the build's goroutine, in order.
	eventsMutex   sync.Mutex
	pendingEvents []godartsass.LogEvent
}

// Start checks the partials directories and starts Dart Sass. A partials
// directory written in gondola.yaml must exist; the default one is skipped
// when it does not.
func Start(options Options) (*Compiler, error) {
	projectDir, err := filepath.Abs(options.ProjectDir)
	if err != nil {
		return nil, fmt.Errorf("finding the project directory: %w", err)
	}
	style, err := godartsassStyle(options.Style)
	if err != nil {
		return nil, err
	}
	partialDirs, err := existingPartialDirs(projectDir, options.Input)
	if err != nil {
		return nil, err
	}
	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil || resolvedProjectDir == projectDir {
		resolvedProjectDir = ""
	}
	compiler := &Compiler{
		projectDir:         projectDir,
		resolvedProjectDir: resolvedProjectDir,
		ignoresLetterCase:  options.IgnoresLetterCase,
		partialDirs:        partialDirs,
		style:              style,
		printWarning:       options.PrintWarning,
		printedWarnings:    map[string]bool{},
	}
	transpiler, err := godartsass.Start(godartsass.Options{
		DartSassEmbeddedFilename: options.ExecutablePath,
		LogEventHandler:          compiler.keepEvent,
	})
	if err != nil {
		return nil, fmt.Errorf("starting Dart Sass: %w", err)
	}
	compiler.transpiler = transpiler
	return compiler, nil
}

func godartsassStyle(style OutputStyle) (godartsass.OutputStyle, error) {
	switch style {
	case Compressed:
		return godartsass.OutputStyleCompressed, nil
	case Expanded:
		return godartsass.OutputStyleExpanded, nil
	}
	return "", fmt.Errorf("unknown Sass output style %d", style)
}

// existingPartialDirs returns the absolute paths of the partials directories
// that exist, in the order written.
func existingPartialDirs(projectDir string, input config.Input) ([]string, error) {
	var dirs []string
	for i, dir := range input.PartialDirs.Dirs {
		exists, err := checkPartialDir(projectDir, dir, input.PartialDirs.FromDefault, positionAt(input.PartialDirPositions, i))
		if err != nil {
			return nil, err
		}
		if exists {
			dirs = append(dirs, filepath.Join(projectDir, filepath.FromSlash(dir)))
		}
	}
	return dirs, nil
}

// positionAt returns positions[i], or the zero Position (no position) when
// the list is shorter, as in a Config built by hand.
func positionAt(positions []config.Position, i int) config.Position {
	if i < len(positions) {
		return positions[i]
	}
	return config.Position{}
}

// checkPartialDir reports whether the partials directory dir exists. A
// default directory that does not exist is skipped silently (a minimal site
// has no _components/sass); every other problem is an error. The same rules
// are applied to the data and template directories (packages data, engine).
func checkPartialDir(projectDir, dir string, fromDefault bool, position config.Position) (bool, error) {
	subject := fmt.Sprintf("input.partials %q", dir)
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

// keepEvent is godartsass's LogEventHandler. It runs on godartsass's
// goroutine, before the compilation that logged the event returns.
func (c *Compiler) keepEvent(event godartsass.LogEvent) {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()
	c.pendingEvents = append(c.pendingEvents, event)
}

// takeEvents returns the events kept since the last call and forgets them.
func (c *Compiler) takeEvents() []godartsass.LogEvent {
	c.eventsMutex.Lock()
	defer c.eventsMutex.Unlock()
	events := c.pendingEvents
	c.pendingEvents = nil
	return events
}

// CompileAll compiles every entry point into tempOutputDir, at its
// OutputPath. compileErrors are the mistakes in the user's stylesheets,
// which the build collects with the page errors; an error repeated by
// several entry points (a broken partial that they all use) is returned
// once. err is any other problem, which stops the build at once.
//
// A later release could take the build's error limit here, to stop
// compiling once it is reached.
func (c *Compiler) CompileAll(entryPoints []scan.SassEntryPoint, tempOutputDir string) (compileErrors []error, err error) {
	returned := map[string]bool{}
	for _, entryPoint := range entryPoints {
		compileErr, err := c.compile(entryPoint, tempOutputDir)
		if err != nil {
			return compileErrors, err
		}
		if compileErr == nil || returned[compileErr.Error()] {
			continue
		}
		returned[compileErr.Error()] = true
		compileErrors = append(compileErrors, compileErr)
	}
	return compileErrors, nil
}

// compile compiles one entry point. compileErr is a mistake in the user's
// stylesheets; err is any other problem.
func (c *Compiler) compile(entryPoint scan.SassEntryPoint, tempOutputDir string) (compileErr, err error) {
	sourceFilePath := filepath.Join(c.projectDir, filepath.FromSlash(entryPoint.SourcePath))
	source, err := os.ReadFile(sourceFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", entryPoint.SourcePath, err)
	}
	if err := sourcetext.CheckEncoding(source); err != nil {
		return diag.WithFile(err, entryPoint.SourcePath), nil
	}
	// Dart Sass skips the byte order mark of the files it reads itself; this
	// one is given as text, so it is removed here.
	sourceText := string(sourcetext.TrimByteOrderMark(source))

	syntax := godartsass.SourceSyntaxSCSS
	if strings.EqualFold(filepath.Ext(entryPoint.SourcePath), indentedSyntaxExtension) {
		syntax = godartsass.SourceSyntaxSASS
	}
	result, executeErr := c.transpiler.Execute(godartsass.Args{
		Source:       sourceText,
		URL:          fileURL(sourceFilePath),
		SourceSyntax: syntax,
		OutputStyle:  c.style,
		IncludePaths: c.partialDirs,
	})
	c.printWarnings(c.takeEvents(), sourceFilePath, sourceText)

	// errors.As finds a SassError, the type godartsass returns for a mistake
	// in the stylesheets; any other error comes from running Dart Sass.
	var sassErr godartsass.SassError
	if errors.As(executeErr, &sassErr) {
		return c.compileError(entryPoint, sourceFilePath, sourceText, sassErr), nil
	}
	if executeErr != nil {
		return nil, fmt.Errorf("compiling %s: %w", entryPoint.SourcePath, executeErr)
	}
	return nil, writeCSS(tempOutputDir, entryPoint.OutputPath, result.CSS)
}

// writeCSS writes css at outputPath (relative, with "/") in tempOutputDir.
// An entry point that produces no CSS still gets its file, empty, so that
// every entry point has one. Other CSS ends with a line break, as the sass
// command writes it.
func writeCSS(tempOutputDir, outputPath, css string) error {
	outputFilePath := filepath.Join(tempOutputDir, filepath.FromSlash(outputPath))
	if err := os.MkdirAll(filepath.Dir(outputFilePath), 0o755); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}
	if css != "" {
		css += "\n"
	}
	if err := os.WriteFile(outputFilePath, []byte(css), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", outputPath, err)
	}
	return nil
}

// compileError turns Dart Sass's error into a *diag.Error that points into
// the file where Dart Sass found the mistake, which may be a partial. Dart
// Sass's message is kept as it is.
func (c *Compiler) compileError(entryPoint scan.SassEntryPoint, entryFilePath, entryText string, sassErr godartsass.SassError) error {
	filePath, ok := pathFromFileURL(sassErr.Span.Url)
	if !ok {
		return &diag.Error{File: entryPoint.SourcePath, Msg: sassErr.Message}
	}
	diagErr := &diag.Error{File: c.displayedPath(filePath), Msg: sassErr.Message}
	text, ok := c.fileText(filePath, entryFilePath, entryText)
	if !ok {
		return diagErr
	}
	if line, col, ok := sourcePosition(text, sassErr.Span.Start.Offset); ok {
		diagErr.Line = line
		diagErr.Col = col
	}
	return diagErr
}

// fileText returns the text of filePath as Dart Sass read it, without a byte
// order mark: the entry point's text as given to Dart Sass, or the file read
// from disk for a partial.
func (c *Compiler) fileText(filePath, entryFilePath, entryText string) (string, bool) {
	if filePath == entryFilePath {
		return entryText, true
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", false
	}
	return string(sourcetext.TrimByteOrderMark(content)), true
}

// displayedPath returns filePath relative to the project root, with "/", or
// the absolute path with "/" when it lies outside the project. The root is
// tried as written and then resolved, since Dart Sass may name a file either
// way, and in any letters where the file system ignores letter case.
func (c *Compiler) displayedPath(filePath string) string {
	for _, rootDir := range []string{c.projectDir, c.resolvedProjectDir} {
		if rootDir == "" {
			continue
		}
		relativePath, err := filepath.Rel(rootDir, filePath)
		if err == nil && relativePath != ".." && !strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
			return filepath.ToSlash(relativePath)
		}
		if relativePath, ok := relativeIgnoringCase(rootDir, filePath); ok && c.ignoresLetterCase {
			return filepath.ToSlash(relativePath)
		}
	}
	return filepath.ToSlash(filePath)
}

// relativeIgnoringCase returns filePath relative to rootDir when filePath
// starts with rootDir, in any letters, and a separator. Both are absolute
// and clean. A root whose other spelling has another length in bytes, as
// with the Kelvin sign for "K", is not recognized; the path is then shown
// absolute.
func relativeIgnoringCase(rootDir, filePath string) (string, bool) {
	prefix := strings.TrimSuffix(rootDir, string(filepath.Separator)) + string(filepath.Separator)
	if len(filePath) <= len(prefix) || !strings.EqualFold(filePath[:len(prefix)], prefix) {
		return "", false
	}
	return filePath[len(prefix):], true
}

// Close stops Dart Sass.
func (c *Compiler) Close() error {
	err := c.transpiler.Close()
	// godartsass returns ErrShutdown when Dart Sass exits as expected.
	if err != nil && !errors.Is(err, godartsass.ErrShutdown) {
		return fmt.Errorf("stopping Dart Sass: %w", err)
	}
	return nil
}
