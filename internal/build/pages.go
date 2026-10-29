package build

import (
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/luciaduck/gondola/internal/engine"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/gondola/internal/routing"
	"github.com/luciaduck/gondola/internal/scan"
)

// preparedPage is a page after stage 3: split, merged with its layouts and
// the defaults, and routed. Nothing in it changes during rendering.
type preparedPage struct {
	source   scan.Page
	document frontmatter.Document
	// layouts are the page's layout chain, nearest first; empty for a page
	// with layout: nil.
	layouts []*engine.Layout
	// usesTemplateEngine is false for a page with layout: nil, which is
	// written without running the template engine at all (SPEC §3.2).
	usesTemplateEngine bool
	merged             frontmatter.Merged
	location           routing.Location
}

// preparePages is stage 3 for every page, in the order scan found them.
// Errors are collected; a page with an error is left out of the result.
func (b *builder) preparePages(jet templateEngine, scanned scan.Result) []preparedPage {
	defaults := frontmatter.NewDefaults(b.cfg.Defaults.Variables, b.cfg.Settings.DateFormat)
	pages := make([]preparedPage, 0, len(scanned.Pages))
	for _, source := range scanned.Pages {
		page, err := b.preparePage(jet, defaults, scanned.OutputPaths, source)
		if err != nil {
			if !b.collect(err) {
				break
			}
			continue
		}
		pages = append(pages, page)
	}
	return pages
}

// preparePage reads one page, splits its front matter, finds its layouts,
// merges its variables, works out where it is written and claims that
// output path.
func (b *builder) preparePage(jet templateEngine, defaults frontmatter.Defaults, outputPaths *scan.OutputPaths, source scan.Page) (preparedPage, error) {
	src, err := os.ReadFile(b.diskPath(source.SourcePath))
	if err != nil {
		return preparedPage{}, fmt.Errorf("reading %s: %w", source.SourcePath, withoutPath(err))
	}
	document, err := frontmatter.Split(source.SourcePath, src)
	if err != nil {
		return preparedPage{}, err
	}
	layout := b.pageLayout(document)
	layouts, err := jet.LayoutChain(source.SourcePath, layout)
	if err != nil {
		return preparedPage{}, err
	}
	merged := frontmatter.Merge(defaults, engine.Documents(layouts), document)
	location, err := routing.Route(routingPage(source, merged), b.cfg.Output.Routing)
	if err != nil {
		return preparedPage{}, err
	}
	if err := outputPaths.Claim(location.OutputPath, scan.PageSource, source.SourcePath); err != nil {
		return preparedPage{}, err
	}
	return preparedPage{
		source:             source,
		document:           document,
		layouts:            layouts,
		usesTemplateEngine: layout != frontmatter.NoLayout,
		merged:             merged,
		location:           location,
	}, nil
}

// pageLayout returns the layout of a page: its own, or default.layout when
// the page leaves it out or empty. The layouts' own layout keys never take
// part, because they only name their parents (SPEC §3.2).
func (b *builder) pageLayout(document frontmatter.Document) string {
	if own, _ := document.Variables["layout"].(string); own != "" {
		return own
	}
	// default.layout is always text and never empty: config gives it the
	// value nil when gondola.yaml leaves it out or empty.
	defaultLayout, _ := b.cfg.Defaults.Variables["layout"].(string)
	return defaultLayout
}

// renderPages is stage 5: it renders every page and writes it into the temp
// output, printing a line for each. Errors are collected. It returns the
// number of pages written.
func (b *builder) renderPages(jet templateEngine, globals engine.Globals, pages []preparedPage) int {
	written := 0
	for _, page := range pages {
		err := b.renderPage(jet, globals, page)
		if err != nil {
			if !b.collect(err) {
				break
			}
			continue
		}
		written++
	}
	return written
}

// renderPage runs the steps of SPEC §3.2 on one page and writes the result.
func (b *builder) renderPage(jet templateEngine, globals engine.Globals, page preparedPage) error {
	rendered, err := b.renderedPage(jet, globals, page)
	if err != nil {
		return err
	}
	outputFilePath := path.Join(b.cfg.Output.TempDir(), page.location.OutputPath)
	if err := b.writeFile(outputFilePath, rendered); err != nil {
		return fmt.Errorf("%s: writing %s: %w", page.source.SourcePath, outputFilePath, err)
	}
	engineName := ""
	if page.usesTemplateEngine {
		engineName = string(b.cfg.Settings.Engine)
	}
	b.options.Printer.Shaping(outputFilePath, page.source.SourcePath, engineName)
	return nil
}

// renderedPage returns the final text of a page: Squero, the first template
// round, Goldmark, then the layouts. A page with layout: nil skips both
// template rounds; an HTML page skips Squero and Goldmark.
func (b *builder) renderedPage(jet templateEngine, globals engine.Globals, page preparedPage) ([]byte, error) {
	sourcePath := page.source.SourcePath
	body := page.document.Body
	bodyLineOffset := page.document.BodyLineOffset
	enginePage := engine.Page{
		SourcePath: sourcePath,
		URLPath:    page.location.URLPath,
		Variables:  page.merged.Variables,
	}
	var err error
	if page.source.IsMarkdown && b.cfg.Settings.FencedBlocks {
		if body, err = b.markdown.Squero(sourcePath, body, bodyLineOffset); err != nil {
			return nil, err
		}
	}
	if page.usesTemplateEngine {
		if body, err = jet.RenderBody(enginePage, globals, body, bodyLineOffset); err != nil {
			return nil, err
		}
	}
	if page.source.IsMarkdown {
		if body, err = b.markdown.Markdown(body); err != nil {
			return nil, fmt.Errorf("%s: %w", sourcePath, err)
		}
	}
	if !page.usesTemplateEngine {
		return body, nil
	}
	return jet.WrapInLayouts(enginePage, globals, page.layouts, body)
}

// writeFile writes content to slashPath, relative to the project root and
// written with "/", creating the directories above it.
func (b *builder) writeFile(slashPath string, content []byte) error {
	diskPath := b.diskPath(slashPath)
	if err := os.MkdirAll(filepath.Dir(diskPath), directoryPermissions); err != nil {
		return withoutPath(err)
	}
	if err := os.WriteFile(diskPath, content, filePermissions); err != nil {
		return withoutPath(err)
	}
	return nil
}
