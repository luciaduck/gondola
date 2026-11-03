// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package build runs the stages of a build in order (SPEC §3.1): it reads
// gondola.yaml, finds the files, prepares and renders the pages, compiles
// Sass, copies the passthrough files and publishes the result. It owns the
// temp output, where the build is assembled. Every other package is called
// from here; none calls build.
package build

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/luciaduck/gondola/internal/collections"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/data"
	"github.com/luciaduck/gondola/internal/engine"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/gondola/internal/parser"
	"github.com/luciaduck/gondola/internal/plugin"
	"github.com/luciaduck/gondola/internal/routing"
	"github.com/luciaduck/gondola/internal/sass"
	"github.com/luciaduck/gondola/internal/scan"
)

// maxCollectedErrors is how many errors in pages and stylesheets a build
// collects before it stops (SPEC §5.15). Twenty is enough to show the shape
// of a mistake repeated across a site without filling the terminal. It can
// be changed to any positive number, such as 3 or 50; nothing else depends
// on its value.
const maxCollectedErrors = 20

// directoryPermissions and filePermissions are given to everything the
// build writes. The process's umask is applied on top, as for any program.
const (
	directoryPermissions = 0o755
	filePermissions      = 0o644
)

// Printer is the part of the console that a build writes to. *console.Printer
// implements it.
type Printer interface {
	TakingRide(directoryName string)
	CraftingFencedBlocks()
	Warning(text string)
	Shaping(outputFilePath, sourcePath, engineName string)
	Shipping(tempDir, outputDir string)
	Sitemap(listedCount int, robotsFound bool)
	Executing(pluginPath, tempDir string)
	Built(pageCount, copiedCount int, elapsed time.Duration, gondolaVersion string)
}

// templateEngine is what the build needs from a template engine. *engine.Jet
// implements it; another engine would be a second type behind the same
// methods (SPEC §5.11).
type templateEngine interface {
	LayoutChain(pageFile, layout string) ([]*engine.Layout, error)
	RenderBody(page engine.Page, globals engine.Globals, body []byte, bodyLineOffset int) ([]byte, error)
	WrapInLayouts(page engine.Page, globals engine.Globals, layouts []*engine.Layout, copyHTML []byte) ([]byte, error)
}

// Options are what one build needs to know.
type Options struct {
	// ProjectDir is the directory that holds gondola.yaml: the working
	// directory for gondola ride.
	ProjectDir string
	// GondolaVersion is Gondola's own version, printed on the last line.
	GondolaVersion string
	// SassStyle is Compressed for gondola ride and Expanded for gondola
	// serve.
	SassStyle sass.OutputStyle
	// Mode tells the plugins which command runs the build.
	Mode    plugin.Mode
	Printer Printer
	// Environment is what the plugins inherit, in the form of os.Environ.
	Environment []string
	// PluginStdout and PluginStderr receive the plugins' output as it is,
	// without colors.
	PluginStdout io.Writer
	PluginStderr io.Writer
	// ConfigLoaded, when not nil, is called with the settings as soon as
	// gondola.yaml has been read, before anything is deleted; an error it
	// returns stops the build. gondola serve uses it to learn the port and
	// the output directory, and to refuse a change to them while serving.
	ConfigLoaded func(config.Config) error
	// Now returns the current time. The build reads it at the start and at
	// the end to measure itself; tests give a clock of their own so that
	// the last line is always the same.
	Now func() time.Time
}

// Failure holds the errors that stopped a build, in the order they were
// found. The output directory was not published.
type Failure struct {
	Errors []error
	// LimitReached is true when the build stopped because it had collected
	// maxCollectedErrors errors, so there may be more.
	LimitReached bool
}

// Run builds the site in options.ProjectDir and publishes it. It returns nil
// on success, or the errors that stopped it; the caller prints them. After a
// failure the temp output stays on disk for inspection and the output
// directory does not exist, because every build deletes it first.
func Run(options Options) *Failure {
	b := &builder{options: options, startedAt: options.Now()}
	err := b.run()
	if err == nil && len(b.collected) == 0 {
		return nil
	}
	failure := &Failure{Errors: b.collected, LimitReached: b.limitReached}
	if err != nil {
		failure.Errors = append(failure.Errors, err)
	}
	return failure
}

// builder holds the state of one build as it goes through the stages.
type builder struct {
	options   Options
	startedAt time.Time
	cfg       config.Config
	// markdown converts Markdown and, when fenced blocks are on, runs
	// Squero.
	markdown *parser.Parser
	// collected are the errors in pages and stylesheets, which do not stop
	// the build at once.
	collected    []error
	limitReached bool
}

// collect records an error in a page or a stylesheet and reports whether the
// build may go on looking for more.
func (b *builder) collect(err error) (keepGoing bool) {
	b.collected = append(b.collected, err)
	if len(b.collected) >= maxCollectedErrors {
		b.limitReached = true
		return false
	}
	return true
}

// failed reports whether any error was collected.
func (b *builder) failed() bool {
	return len(b.collected) > 0
}

// run goes through the stages of SPEC §3.1. It returns an error that stops
// the build at once; errors in pages and stylesheets go to b.collected, and
// a stage that collected any is the last one to run, except that Sass errors
// do not stop the pages from rendering, since pages never depend on the CSS.
func (b *builder) run() error {
	if err := b.start(); err != nil {
		return err
	}
	scanned, sassExecutablePath, err := b.scan()
	if err != nil {
		return err
	}
	if err := b.claimSitemap(scanned.OutputPaths); err != nil {
		return err
	}
	jet, err := b.newTemplateEngine()
	if err != nil {
		return err
	}
	pages := b.preparePages(jet, scanned)
	if b.failed() {
		return nil
	}
	globals, err := b.globals(pages)
	if err != nil {
		return err
	}
	if err := b.compileSass(scanned.SassEntryPoints, sassExecutablePath); err != nil || b.limitReached {
		return err
	}
	pageCount := b.renderPages(jet, globals, pages)
	if b.failed() {
		return nil
	}
	if err := b.copyPassthroughFiles(scanned.PassthroughFiles); err != nil {
		return err
	}
	if err := b.writeSitemap(pages, scanned.OutputPaths); err != nil {
		return err
	}
	if err := b.runPlugins(); err != nil {
		return err
	}
	if err := b.publish(); err != nil {
		return err
	}
	elapsed := b.options.Now().Sub(b.startedAt)
	b.options.Printer.Built(pageCount, len(scanned.PassthroughFiles), elapsed, b.options.GondolaVersion)
	return nil
}

// start is stage 1: it reads gondola.yaml, checks the prerequisites and
// replaces the output and the temp output with an empty temp output.
func (b *builder) start() error {
	absoluteProjectDir, err := filepath.Abs(b.options.ProjectDir)
	if err != nil {
		return fmt.Errorf("finding the project directory: %w", err)
	}
	b.options.Printer.TakingRide(filepath.Base(absoluteProjectDir))
	b.cfg, err = config.Load(b.options.ProjectDir)
	if err != nil {
		return err
	}
	if b.options.ConfigLoaded != nil {
		if err := b.options.ConfigLoaded(b.cfg); err != nil {
			return err
		}
	}
	aliasFile := parser.AliasFile{}
	if b.cfg.Settings.FencedBlocks {
		b.options.Printer.CraftingFencedBlocks()
		// The alias file is read only when fenced blocks are on, so that a
		// broken file that nothing uses does not stop the build.
		if aliasFile, err = parser.LoadAliasFile(b.options.ProjectDir); err != nil {
			return err
		}
	}
	b.markdown = parser.New(aliasFile)
	// Checked before anything is deleted, so a missing plugin keeps the old
	// output (M11, Q2).
	if err := plugin.Check(b.options.ProjectDir, b.cfg.Settings.PluginPaths, b.cfg.Settings.PluginPositions); err != nil {
		return err
	}
	for _, dir := range []string{b.cfg.Output.Dir, b.cfg.Output.TempDir()} {
		if err := os.RemoveAll(b.diskPath(dir)); err != nil {
			return fmt.Errorf("deleting %s: %w%s", dir, withoutPath(err), InUseHint(err, dir))
		}
	}
	if err := os.MkdirAll(b.diskPath(b.cfg.Output.TempDir()), directoryPermissions); err != nil {
		return fmt.Errorf("creating %s: %w", b.cfg.Output.TempDir(), withoutPath(err))
	}
	return nil
}

// scan is stage 2. It returns the path of the Dart Sass executable when
// there are Sass files to compile, and "" otherwise.
func (b *builder) scan() (scan.Result, string, error) {
	scanned, err := scan.Scan(b.options.ProjectDir, b.cfg)
	if err != nil {
		return scan.Result{}, "", err
	}
	// A site without Sass files does not need Dart Sass installed.
	if !b.cfg.Settings.Sass || len(scanned.SassEntryPoints) == 0 {
		return scanned, "", nil
	}
	sassExecutablePath, err := sass.FindExecutable(scanned.SassEntryPoints[0].SourcePath)
	if err != nil {
		return scan.Result{}, "", err
	}
	return scanned, sassExecutablePath, nil
}

// newTemplateEngine returns the template engine of this build. Only Jet
// exists today; config.Settings.Engine would choose another one here.
func (b *builder) newTemplateEngine() (*engine.Jet, error) {
	// fencedBlocks stays a nil interface when fenced blocks are off: a nil
	// *parser.Parser stored in it would not compare equal to nil, and the
	// engine would call Squero on the included fragments.
	var fencedBlocks engine.FencedBlockRewriter
	if b.cfg.Settings.FencedBlocks {
		fencedBlocks = b.markdown
	}
	return engine.New(engine.Options{
		ProjectDir:           b.options.ProjectDir,
		TemplateDirs:         b.cfg.Input.TemplateDirs,
		TemplateDirPositions: b.cfg.Input.TemplateDirPositions,
		FencedBlocks:         fencedBlocks,
	})
}

// globals is the end of stage 3: once every page is prepared, it builds the
// collections, loads the data files and returns the variables that every
// page shares.
func (b *builder) globals(pages []preparedPage) (engine.Globals, error) {
	collectionPages := make([]collections.Page, 0, len(pages))
	for _, page := range pages {
		collectionPages = append(collectionPages, collections.Page{
			SourcePath: page.source.SourcePath,
			URLPath:    page.location.URLPath,
			Merged:     page.merged,
		})
	}
	builtCollections := collections.Build(collectionPages, b.cfg.Settings.CanonicalURL)
	dataFiles, err := data.Load(b.options.ProjectDir, b.cfg.Input)
	if err != nil {
		return engine.Globals{}, err
	}
	return engine.NewGlobals(b.cfg.Settings, dataFiles, builtCollections), nil
}

// compileSass is stage 4. Errors in the stylesheets are collected; any
// other problem stops the build.
func (b *builder) compileSass(entryPoints []scan.SassEntryPoint, executablePath string) error {
	if executablePath == "" {
		return nil
	}
	compiler, err := sass.Start(sass.Options{
		ProjectDir:     b.options.ProjectDir,
		ExecutablePath: executablePath,
		Input:          b.cfg.Input,
		Style:          b.options.SassStyle,
		PrintWarning:   b.options.Printer.Warning,
		// On macOS and Windows Dart Sass names files in their letters on
		// disk, which messages must still show relative to the project.
		IgnoresLetterCase: b.cfg.IgnoresLetterCase,
	})
	if err != nil {
		return err
	}
	compileErrors, err := compiler.CompileAll(entryPoints, b.diskPath(b.cfg.Output.TempDir()))
	if closeErr := compiler.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("stopping Dart Sass: %w", closeErr)
	}
	if err != nil {
		return err
	}
	for _, compileErr := range compileErrors {
		if !b.collect(compileErr) {
			break
		}
	}
	return nil
}

// runPlugins is stage 8. The first plugin that fails stops the build.
func (b *builder) runPlugins() error {
	return plugin.Run(b.cfg.Settings.PluginPaths, plugin.Options{
		ProjectDir:  b.options.ProjectDir,
		TempDir:     b.cfg.Output.TempDir(),
		Mode:        b.options.Mode,
		Environment: b.options.Environment,
		Stdout:      b.options.PluginStdout,
		Stderr:      b.options.PluginStderr,
		Printer:     b.options.Printer,
	})
}

// publish is stage 9: it replaces the output directory with the temp
// output. The output is deleted first because renaming over a directory
// fails on Windows, and everywhere when the directory is not empty. Stage 1
// has deleted it already, but publishing does not rely on that.
func (b *builder) publish() error {
	outputDir, tempDir := b.cfg.Output.Dir, b.cfg.Output.TempDir()
	if err := os.RemoveAll(b.diskPath(outputDir)); err != nil {
		return fmt.Errorf("deleting %s: %w%s", outputDir, withoutPath(err), InUseHint(err, outputDir))
	}
	if err := os.Rename(b.diskPath(tempDir), b.diskPath(outputDir)); err != nil {
		return fmt.Errorf("renaming %s to %s: %w%s", tempDir, outputDir, withoutPath(err), InUseHint(err, tempDir+" or "+outputDir))
	}
	b.options.Printer.Shipping(tempDir, outputDir)
	return nil
}

// diskPath turns slashPath, relative to the project root and written with
// "/", into a path on disk.
func (b *builder) diskPath(slashPath string) string {
	return filepath.Join(b.options.ProjectDir, filepath.FromSlash(slashPath))
}

// withoutPath returns the reason of a file system error without the path
// that the os package puts into it. That path is written with the
// platform's separator and joined to the project directory; messages name
// the file themselves, relative to the project root and with "/".
func withoutPath(err error) error {
	// errors.As looks through any wrapping for the two error types of
	// package os that carry paths.
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return linkErr.Err
	}
	return err
}

// routingPage returns what routing needs to know about a page whose
// variables are merged.
func routingPage(source scan.Page, merged frontmatter.Merged) routing.Page {
	// Merged always holds permalink and title, as text: the defaults
	// define both.
	permalink, _ := merged.Variables["permalink"].(string)
	title, _ := merged.Variables["title"].(string)
	return routing.Page{
		SourcePath:  source.SourcePath,
		ContentPath: source.ContentPath,
		Permalink:   permalink,
		Title:       title,
		Date:        merged.Date,
		HasDate:     merged.HasDate,
	}
}
