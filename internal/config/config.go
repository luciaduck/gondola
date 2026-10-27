// Package config reads and validates gondola.yaml, the file that holds a
// site's settings, and gives the rest of Gondola a typed configuration with
// every default applied.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/luciaduck/gondola/internal/diag"
)

// FileName is the name of the configuration file, read from the project
// directory (the working directory).
const FileName = "gondola.yaml"

// TempDirSuffix is appended to the output directory to name the temp output,
// where a build is assembled before it is published: "_site" becomes
// "_site.temp".
const TempDirSuffix = ".temp"

// NoLayout is the layout value that means "no layout": the page is not run
// through the template engine at all.
const NoLayout = "nil"

// Routing decides the shape of output paths (SPEC §5.7).
type Routing string

// The values of output.routing.
const (
	// RoutingFile turns about.md into about.html.
	RoutingFile Routing = "file"
	// RoutingDirectory turns about.md into about/index.html.
	RoutingDirectory Routing = "directory"
)

// Permalink decides how a page's output file is named (SPEC §5.7). Custom
// permalinks such as "/about-us/" are only written in a page's frontmatter,
// so they are not a Permalink.
type Permalink string

// The values of default.permalink.
const (
	// PermalinkPath keeps the source path.
	PermalinkPath Permalink = "path"
	// PermalinkSlug names the file after the slug of the title.
	PermalinkSlug Permalink = "slug"
	// PermalinkDate builds /YYYY/MM/DD/<slug> from the date and the title.
	PermalinkDate Permalink = "date"
)

// Engine names a template engine.
type Engine string

// EngineJet is the Jet template engine, the only one supported today.
// Another engine would be added here.
const EngineJet Engine = "jet"

// The defaults of SPEC §5.3. The three required keys (site-name, version,
// canonical-url) have none.
const (
	defaultDescription  = ""
	defaultSass         = true
	defaultFencedBlocks = false
	defaultMarkdown     = true
	defaultEngine       = EngineJet
	defaultSitemap      = false
	defaultRobotsAgent  = "*"
	defaultDateFormat   = "2006-01-02"
	defaultPort         = 1094

	defaultContentDir  = "."
	defaultTemplateDir = "_components/includes"
	defaultDataDir     = "_data"
	defaultPartialDir  = "_components/sass"

	defaultRouting   = RoutingFile
	defaultOutputDir = "_site"

	defaultLayout    = NoLayout
	defaultTitle     = ""
	defaultAuthor    = ""
	defaultDate      = ""
	defaultPermalink = PermalinkPath
)

// The range of valid TCP ports for config.port.
const (
	minPort = 1
	maxPort = 65535
)

// Config is the whole of gondola.yaml with every default applied.
//
// Every path is relative to the project directory, cleaned, and written with
// "/". Use filepath.FromSlash before touching the disk.
type Config struct {
	Settings Settings
	Input    Input
	Output   Output
	Defaults Defaults
}

// Settings is the config: section.
type Settings struct {
	SiteName string
	// SiteVersion is the version of the site, not of Gondola. It is kept
	// exactly as written, so an unquoted 0.10 stays "0.10".
	SiteVersion string
	// CanonicalURL is an absolute http or https URL without a trailing
	// slash, such as "https://example.com" or "https://example.com/blog".
	CanonicalURL string
	Description  string
	Sass         bool
	// FencedBlocks turns on Squero. It is only valid with Markdown on.
	FencedBlocks bool
	// Markdown makes .md and .markdown files pages.
	Markdown bool
	// PluginPaths are the plugins to run on the finished build, in order.
	PluginPaths []string
	Engine      Engine
	Sitemap     bool
	// RobotsAgent is the user-agent whose robots.txt rules filter the
	// sitemap.
	RobotsAgent string
	// DateFormat is a Go time layout, such as "2 January 2006".
	DateFormat string
	// Port is the port that `gondola serve` listens on.
	Port int
}

// Input is the input: section.
type Input struct {
	ContentDir   string
	TemplateDirs DirList
	DataDirs     DirList
	// PartialDirs are the Sass load paths ("partials" in the Sass sense).
	// They are never scanned for entry points.
	PartialDirs DirList
}

// DirList is the value of a key that accepts a path or a list of paths.
type DirList struct {
	// Dirs are the directories in the order written. Empty when the user
	// wrote [].
	Dirs []string
	// FromDefault is true when gondola.yaml leaves the key out or empty, so
	// Dirs holds the default. A default directory that does not exist is
	// skipped; a directory the user wrote must exist.
	FromDefault bool
}

// Output is the output: section.
type Output struct {
	Routing Routing
	// Dir is the output directory. It is deleted at the start of every
	// build.
	Dir string
	// PassthroughPatterns are kept as written; package scan reads them.
	PassthroughPatterns []string
}

// TempDir returns the temp output, where a build is assembled before it is
// published.
func (o Output) TempDir() string {
	return o.Dir + TempDirSuffix
}

// Defaults is the default: section: the values a page takes when it leaves a
// key out or empty.
type Defaults struct {
	// Permalink is the validated value of default.permalink. It is also in
	// Variables, as a string.
	Permalink Permalink
	// Variables holds every entry of default:, the built-in ones (layout,
	// title, author, date, permalink) and the user's own, keyed by template
	// name ("og-image" becomes "ogImage"). Values keep their YAML types; a
	// user key left empty holds "".
	Variables map[string]any
}

// missingFileMessage explains what gondola.yaml is for, since a user who
// sees it may be running Gondola for the first time.
const missingFileMessage = FileName + " not found in the current directory; it holds the site settings and must set at least config.site-name, config.version and config.canonical-url"

// Load reads gondola.yaml from projectDir and parses it with Parse.
func Load(projectDir string) (Config, error) {
	src, err := os.ReadFile(filepath.Join(projectDir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, errors.New(missingFileMessage)
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", FileName, err)
	}
	return Parse(src)
}

// Parse parses and validates the contents of gondola.yaml. Errors that point
// at a place in the file are *diag.Error values naming FileName.
func Parse(src []byte) (Config, error) {
	config, err := parse(src)
	if err != nil {
		return Config{}, toFilePosition(err, src)
	}
	return config, nil
}

// utf16ByteOrderMarks start a file saved as UTF-16, little-endian and
// big-endian. Windows PowerShell 5.1 writes such files by default.
var utf16ByteOrderMarks = [][]byte{{0xFF, 0xFE}, {0xFE, 0xFF}}

// utf16Message is the error for a file saved as UTF-16. It has no position:
// the console could not print a line of such a file.
const utf16Message = "the file is saved as UTF-16; please save it as UTF-8"

func parse(src []byte) (Config, error) {
	// yaml.v3 would read UTF-16, but the positions in its errors would count
	// the decoded text, not the bytes of the file, and every other source
	// file Gondola reads is UTF-8. A UTF-8 byte order mark is fine.
	for _, byteOrderMark := range utf16ByteOrderMarks {
		if bytes.HasPrefix(src, byteOrderMark) {
			return Config{}, &diag.Error{File: FileName, Msg: utf16Message}
		}
	}
	root, err := parseDocument(src)
	if err != nil {
		return Config{}, err
	}
	values := newFileValues()
	if err := values.readRoot(root); err != nil {
		return Config{}, err
	}
	if err := values.validate(); err != nil {
		return Config{}, err
	}
	return values.config(), nil
}
