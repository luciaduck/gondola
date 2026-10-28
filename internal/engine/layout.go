package engine

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CloudyKit/jet/v6"
	"github.com/luciaduck/gondola/internal/frontmatter"
)

// layoutKey is the front matter key that names a layout, as messages write
// it.
const layoutKey = "layout"

// Layout is a layout template found by LayoutChain.
type Layout struct {
	// Name is the layout as the layout key wrote it, such as "page".
	Name string
	// File is the layout's file relative to the project root, with "/",
	// such as "_components/includes/page.jet".
	File string
	// Document is the layout's front matter and body. Its variables take
	// part in the page's merge (frontmatter.Merge); its layout key names
	// the parent layout.
	Document frontmatter.Document
	// templatePath is the layout's path inside its template directory, as
	// Jet names templates: "/page.jet". Relative includes in the layout
	// start from its directory.
	templatePath string
	// template is the parsed layout, once a page has used it.
	template *jet.Template
}

// LayoutChain returns the layouts that wrap the page pageFile, whose layout
// key is layout: the named layout first, then its parent, and so on until a
// layout has no parent. A page with layout "nil" (or none) has no layouts.
//
// A layout is looked for with ".jet", then ".html", added to its name, in
// the template directories in order: page.jet in any directory comes before
// page.html, as for Jet's include. A name that already ends in ".jet" or
// ".html" is used as it is.
func (j *Jet) LayoutChain(pageFile, layout string) ([]*Layout, error) {
	chain := []*Layout{}
	requester := pageFile
	name := layout
	for name != "" && name != frontmatter.NoLayout {
		found, err := j.layout(pageFile, requester, name)
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(chain, func(l *Layout) bool { return l.File == found.File }); i >= 0 {
			return nil, layoutCycleError(pageFile, chain[i:], found)
		}
		chain = append(chain, found)
		parent, hasParent := frontmatter.ParentLayout(found.Document)
		if !hasParent {
			break
		}
		requester = found.File
		name = parent
	}
	return chain, nil
}

// Documents returns the documents of layouts in the same order, as
// frontmatter.Merge takes them.
func Documents(layouts []*Layout) []frontmatter.Document {
	documents := make([]frontmatter.Document, len(layouts))
	for i, layout := range layouts {
		documents[i] = layout.Document
	}
	return documents
}

// layout finds and reads the layout name, asked for by the file requester
// (the page, or the layout whose parent it is) while preparing pageFile.
func (j *Jet) layout(pageFile, requester, name string) (*Layout, error) {
	if err := frontmatter.CheckLayout(frontmatter.Key{Written: layoutKey}, name); err != nil {
		return nil, errorInFile(requester, "%s%s", err, renderingNote(pageFile, requester))
	}
	candidates := layoutCandidates(name)
	var file, templatePath string
	for _, candidate := range candidates {
		found, ok, err := j.loader.find(candidate)
		if err != nil {
			return nil, err
		}
		if ok {
			file, templatePath = found, candidate
			break
		}
	}
	if file == "" {
		return nil, errorInFile(requester, "%s%s", j.layoutNotFound(name, candidates), renderingNote(pageFile, requester))
	}
	if cached, ok := j.layouts[file]; ok {
		return cached, nil
	}
	src, err := os.ReadFile(filepath.Join(j.loader.projectDir, filepath.FromSlash(file)))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", file, err)
	}
	document, err := frontmatter.Split(file, src)
	if err != nil {
		return nil, err
	}
	found := &Layout{Name: name, File: file, Document: document, templatePath: templatePath}
	j.layouts[file] = found
	return found, nil
}

// layoutCandidates returns the template paths that the layout name may
// have, in the order they are tried.
func layoutCandidates(name string) []string {
	switch path.Ext(name) {
	case ".jet", ".html":
		return []string{"/" + name}
	}
	candidates := []string{}
	for _, extension := range templateExtensions {
		if extension != "" {
			candidates = append(candidates, "/"+name+extension)
		}
	}
	return candidates
}

// layoutNotFound is the message for a layout that no template directory
// holds: `layout "page" was not found; looked for page.jet and page.html in
// _components/includes (the default)`.
func (j *Jet) layoutNotFound(name string, candidates []string) string {
	if len(j.loader.templateDirs) == 0 {
		return fmt.Sprintf("layout %q was not found, because %s", name, j.loader.noTemplateDirHint())
	}
	names := make([]string, len(candidates))
	for i, candidate := range candidates {
		names[i] = displayedName(candidate)
	}
	return fmt.Sprintf("layout %q was not found; looked for %s in %s", name, strings.Join(names, " and "), j.loader.searchedDirs())
}

// layoutCycleError names the layouts that name each other as parents:
// "page.jet → base.jet → page.jet".
func layoutCycleError(pageFile string, cycle []*Layout, again *Layout) error {
	names := []string{}
	for _, layout := range cycle {
		names = append(names, displayedName(layout.templatePath))
	}
	names = append(names, displayedName(again.templatePath))
	return errorInFile(pageFile, "the layouts form a cycle: %s; please remove one of the layout keys", strings.Join(names, " → "))
}

// renderingNote ends a message about a file other than the page itself,
// so that the user knows which page needed it: " (while rendering
// blog/post.md)".
func renderingNote(pageFile, file string) string {
	if file == pageFile {
		return ""
	}
	return fmt.Sprintf(" (while rendering %s)", pageFile)
}
