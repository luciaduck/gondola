// Package collections builds the template variables fleet, every page that
// is not excluded from it, and collections, the pages of each tag. It runs
// after every page is prepared and before any is rendered, so that a
// template may list other pages.
package collections

import (
	"slices"
	"strings"

	"github.com/luciaduck/gondola/internal/frontmatter"
)

// The keys of an entry, as templates read them (entry.url).
const (
	urlKey       = "url"
	canonicalKey = "canonical"
	sourceKey    = "source"
	titleKey     = "title"
	dateKey      = "date"
	tagsKey      = "tags"
	metaKey      = "meta"
)

// The page variables that Build reads. Package frontmatter guarantees that
// they exist on every page, with these types.
const (
	titleVariable            = "title"            // as written, or ""
	dateVariable             = "date"             // formatted, or ""
	tagsVariable             = "tags"             // []string
	excludeFromFleetVariable = "excludeFromFleet" // bool
)

// Page is what Build needs to know about one page.
type Page struct {
	// SourcePath is relative to the project root, with "/", such as
	// "blog/post.md".
	SourcePath string
	// URLPath is the page's URL path from package routing: it begins with
	// "/" and is already percent-encoded.
	URLPath string
	// Merged holds the page's variables after merging with its layouts and
	// the defaults, and its date.
	Merged frontmatter.Merged
}

// Collections holds the two template variables.
type Collections struct {
	// Fleet is every page except those with excludeFromFleet: true.
	Fleet []map[string]any
	// Tags maps each tag that at least one page has to its pages; templates
	// read it as collections["web-dev"]. A page with excludeFromFleet:
	// true is still in the collections of its tags.
	Tags map[string][]map[string]any
}

// Build returns fleet and the collections of pages. canonicalURL is
// config.canonical-url, without a trailing slash.
//
// Each entry is a map[string]any with the keys url, canonical, source,
// title, date (formatted with config.date-format, or ""), tags and meta (all
// the page's variables). A map rather than a struct, because templates read
// the lower-case names (entry.url) and Jet can read only exported, capital
// struct fields. One page has one entry, shared by fleet and its tags, and
// the entries share their values with the pages' variables: nothing may
// change them. There is no rendered body in an entry, so pages never depend
// on the order in which they are rendered.
//
// Every list is in the order of sortPages; pages is not changed.
func Build(pages []Page, canonicalURL string) Collections {
	sorted := slices.Clone(pages)
	sortPages(sorted)
	result := Collections{Fleet: []map[string]any{}, Tags: make(map[string][]map[string]any)}
	for _, page := range sorted {
		entry := newEntry(page, canonicalURL)
		if excluded, _ := page.Merged.Variables[excludeFromFleetVariable].(bool); !excluded {
			result.Fleet = append(result.Fleet, entry)
		}
		tags, _ := page.Merged.Variables[tagsVariable].([]string)
		for _, tag := range tags {
			result.Tags[tag] = append(result.Tags[tag], entry)
		}
	}
	return result
}

// Pagination, when it comes, would split these lists here.

func newEntry(page Page, canonicalURL string) map[string]any {
	variables := page.Merged.Variables
	return map[string]any{
		urlKey:       page.URLPath,
		canonicalKey: canonicalURL + page.URLPath,
		sourceKey:    page.SourcePath,
		titleKey:     variables[titleVariable],
		dateKey:      variables[dateVariable],
		tagsKey:      variables[tagsVariable],
		metaKey:      variables,
	}
}

// sortPages puts pages in the order of every collection: pages without a
// date first, then by date from the oldest, comparing the moments in time
// (so 10:00+02:00 and 08:00Z are a tie); ties are broken by source path,
// compared byte by byte. There is no fallback on the files' modification
// times, which change on every checkout. Source paths are unique, so the
// order never depends on the order of pages.
func sortPages(pages []Page) {
	slices.SortFunc(pages, func(a, b Page) int {
		if a.Merged.HasDate != b.Merged.HasDate {
			if !a.Merged.HasDate {
				return -1
			}
			return 1
		}
		if a.Merged.HasDate {
			if byDate := a.Merged.Date.Compare(b.Merged.Date); byDate != 0 {
				return byDate
			}
		}
		return strings.Compare(a.SourcePath, b.SourcePath)
	})
}
