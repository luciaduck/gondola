package config

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/frontmatter"
	"github.com/luciaduck/gondola/internal/naming"
	"github.com/luciaduck/gondola/internal/yamlnode"
	"gopkg.in/yaml.v3"
)

// gondola.yaml is read as a tree of yaml.Node values instead of being decoded
// into structs. Decoding into structs would lose the column of each value,
// would report unknown keys with Go type names, and turns an explicit null
// into an empty list or map (see yaml_v3_test.go). Walking the nodes lets
// every error point at line and column, and lets null and "" mean "use the
// default" for every optional key.

// atTopLevel places a key in the outermost mapping of the file, in messages.
const atTopLevel = "at the top level of " + FileName

// defaultKeyNoun names the keys of default: in messages from package naming.
const defaultKeyNoun = "default key"

// What to write for each required key, used in the messages for an absent
// or empty one.
const (
	siteNamePurpose     = "the name of the site"
	siteVersionPurpose  = "the version of the site"
	canonicalURLPurpose = "the absolute URL where the site is published, such as https://example.com"
)

// locatedPath is a cleaned path and the node it was read from, so that the
// checks between directories can point at it. node is nil for a default.
type locatedPath struct {
	path string
	node *yaml.Node
}

// locatedPattern is a passthrough pattern as written and the node it was
// read from.
type locatedPattern struct {
	pattern string
	node    *yaml.Node
}

// locatedDirList is a DirList whose entries remember their nodes.
type locatedDirList struct {
	dirs        []locatedPath
	fromDefault bool
}

// fileValues collects what Parse reads from the file, starting from the
// defaults.
type fileValues struct {
	settings Settings
	// The nodes of the values that later checks point at. A nil node means
	// the key was not written.
	siteNameNode     *yaml.Node
	siteVersionNode  *yaml.Node
	canonicalURLNode *yaml.Node
	fencedBlocksNode *yaml.Node

	pluginPaths         []locatedPath
	contentDir          locatedPath
	templateDirs        locatedDirList
	dataDirs            locatedDirList
	partialDirs         locatedDirList
	outputDir           locatedPath
	routing             Routing
	passthroughPatterns []locatedPattern
	permalink           Permalink
	variables           map[string]any
	// source is the whole file, for the line numbers written in messages
	// about the values of default:, and for the positions kept in Config.
	source yamlnode.Source
	// directories compares the role directories, ignoring letter case when
	// the project's file system does.
	directories pathComparer
}

func newFileValues(src []byte, ignoresLetterCase bool) *fileValues {
	return &fileValues{
		source:      yamlnode.Source{Text: src},
		directories: pathComparer{ignoresLetterCase: ignoresLetterCase},
		settings: Settings{
			Description:  defaultDescription,
			Sass:         defaultSass,
			FencedBlocks: defaultFencedBlocks,
			Markdown:     defaultMarkdown,
			Engine:       defaultEngine,
			Sitemap:      defaultSitemap,
			RobotsAgent:  defaultRobotsAgent,
			DateFormat:   defaultDateFormat,
			Port:         defaultPort,
		},
		contentDir:   locatedPath{path: defaultContentDir},
		templateDirs: defaultDirList(defaultTemplateDir),
		dataDirs:     defaultDirList(defaultDataDir),
		partialDirs:  defaultDirList(defaultPartialDir),
		outputDir:    locatedPath{path: defaultOutputDir},
		routing:      defaultRouting,
		permalink:    defaultPermalink,
		variables: map[string]any{
			"layout":    defaultLayout,
			"title":     defaultTitle,
			"author":    defaultAuthor,
			"date":      defaultDate,
			"permalink": string(defaultPermalink),
		},
	}
}

func defaultDirList(dir string) locatedDirList {
	return locatedDirList{dirs: []locatedPath{{path: dir}}, fromDefault: true}
}

// config returns the values as a Config. Lists are never nil, so callers
// and tests need not tell nil from empty.
func (v *fileValues) config() Config {
	settings := v.settings
	settings.PluginPaths = paths(v.pluginPaths)
	return Config{
		Settings: settings,
		Input: Input{
			ContentDir:         v.contentDir.path,
			ContentDirPosition: v.position(v.contentDir.node),
			TemplateDirs:       v.templateDirs.dirList(),
			DataDirs:           v.dataDirs.dirList(),
			DataDirPositions:   v.positions(v.dataDirs.dirs),
			PartialDirs:        v.partialDirs.dirList(),
		},
		Output: Output{
			Routing:             v.routing,
			Dir:                 v.outputDir.path,
			PassthroughPatterns: v.passthroughPatternList(),
		},
		Defaults: Defaults{
			Permalink: v.permalink,
			Variables: v.variables,
		},
		IgnoresLetterCase: v.directories.ignoresLetterCase,
	}
}

// passthroughPatternList returns the patterns with their positions in the
// file. The list is never nil.
func (v *fileValues) passthroughPatternList() []PassthroughPattern {
	patterns := make([]PassthroughPattern, 0, len(v.passthroughPatterns))
	for _, located := range v.passthroughPatterns {
		patterns = append(patterns, PassthroughPattern{Pattern: located.pattern, Position: v.position(located.node)})
	}
	return patterns
}

// position returns where node is written, counted as package diag counts,
// or the zero Position for a default (a nil node).
func (v *fileValues) position(node *yaml.Node) Position {
	if node == nil {
		return Position{}
	}
	line, col := v.source.FilePosition(node.Line, node.Column)
	return Position{Line: line, Col: col}
}

// positions returns where each of located is written, at the same indexes.
// The list is never nil.
func (v *fileValues) positions(located []locatedPath) []Position {
	result := make([]Position, 0, len(located))
	for _, entry := range located {
		result = append(result, v.position(entry.node))
	}
	return result
}

func (l locatedDirList) dirList() DirList {
	return DirList{Dirs: paths(l.dirs), FromDefault: l.fromDefault}
}

func paths(located []locatedPath) []string {
	result := make([]string, 0, len(located))
	for _, entry := range located {
		result = append(result, entry.path)
	}
	return result
}

// parseDocument parses src into a node tree and returns its root, or nil
// when the file holds no document.
func parseDocument(src []byte) (*yaml.Node, error) {
	root, err := yamlnode.ParseDocument(src, FileName)
	return root, diag.WithFile(err, FileName)
}

// readRoot reads the four sections, then checks that the required keys were
// written.
func (v *fileValues) readRoot(root *yaml.Node) error {
	if root != nil && !yamlnode.IsEmpty(root) {
		if err := v.readSections(root); err != nil {
			return err
		}
	}
	return v.checkRequired()
}

func (v *fileValues) readSections(root *yaml.Node) error {
	mapping := yamlnode.Resolved(root)
	if mapping.Kind != yaml.MappingNode {
		return errorAt(root, "%s must be a map with the sections config, input, output and default, not %s", FileName, yamlnode.Describe(root))
	}
	items, err := entries(mapping, atTopLevel)
	if err != nil {
		return err
	}
	for _, item := range items {
		var readSection func(section *yaml.Node) error
		switch item.key {
		case "config":
			readSection = v.readSettings
		case "input":
			readSection = v.readInput
		case "output":
			readSection = v.readOutput
		case "default":
			readSection = v.readDefaults
		default:
			return errorAt(item.keyNode, "unknown key %q %s; please remove it or check its spelling", item.key, atTopLevel)
		}
		if yamlnode.IsEmpty(item.valueNode) {
			continue
		}
		section := yamlnode.Resolved(item.valueNode)
		if section.Kind != yaml.MappingNode {
			return errorAt(item.valueNode, "%s must be a map of keys, not %s", item.key, yamlnode.Describe(item.valueNode))
		}
		if err := readSection(section); err != nil {
			return err
		}
	}
	return nil
}

// keyHandler reads the value of one known key.
type keyHandler func(item keyValue) error

// readKeys calls the handler of each key of section, in the order written.
// sectionName names the section in messages.
func readKeys(sectionName string, section *yaml.Node, handlers map[string]keyHandler) error {
	items, err := entries(section, "in "+sectionName)
	if err != nil {
		return err
	}
	for _, item := range items {
		handle, known := handlers[item.key]
		if !known {
			return errorAt(item.keyNode, "unknown key %q in %s; please remove it or check its spelling", item.key, sectionName)
		}
		if err := handle(item); err != nil {
			return err
		}
	}
	return nil
}

func (v *fileValues) readSettings(section *yaml.Node) error {
	settings := &v.settings
	return readKeys("config", section, map[string]keyHandler{
		"site-name": func(item keyValue) error {
			v.siteNameNode = item.valueNode
			return readRequiredText(&settings.SiteName, "config.site-name", siteNamePurpose, item)
		},
		"version": func(item keyValue) error {
			v.siteVersionNode = item.valueNode
			return readRequiredText(&settings.SiteVersion, "config.version", siteVersionPurpose, item)
		},
		"canonical-url": func(item keyValue) error {
			v.canonicalURLNode = item.valueNode
			return readCanonicalURL(&settings.CanonicalURL, "config.canonical-url", item)
		},
		"description": func(item keyValue) error {
			return readText(&settings.Description, "config.description", item)
		},
		"sass": func(item keyValue) error {
			return readBool(&settings.Sass, "config.sass", item)
		},
		"fenced-blocks": func(item keyValue) error {
			v.fencedBlocksNode = item.valueNode
			return readBool(&settings.FencedBlocks, "config.fenced-blocks", item)
		},
		"markdown": func(item keyValue) error {
			return readBool(&settings.Markdown, "config.markdown", item)
		},
		"plugin": func(item keyValue) error {
			return readPathList(&v.pluginPaths, "config.plugin", item)
		},
		"engine": func(item keyValue) error {
			return readChoice(&settings.Engine, "config.engine", item, []Engine{EngineJet})
		},
		"sitemap": func(item keyValue) error {
			return readBool(&settings.Sitemap, "config.sitemap", item)
		},
		"robots-agent": func(item keyValue) error {
			return readText(&settings.RobotsAgent, "config.robots-agent", item)
		},
		"date-format": func(item keyValue) error {
			// Any text is a valid Go time layout, so there is nothing to check.
			return readText(&settings.DateFormat, "config.date-format", item)
		},
		"port": func(item keyValue) error {
			return readPort(&settings.Port, "config.port", item)
		},
	})
}

func (v *fileValues) readInput(section *yaml.Node) error {
	return readKeys("input", section, map[string]keyHandler{
		"content": func(item keyValue) error {
			return readPath(&v.contentDir, "input.content", item)
		},
		"templates": func(item keyValue) error {
			return readDirList(&v.templateDirs, "input.templates", item)
		},
		"data": func(item keyValue) error {
			return readDirList(&v.dataDirs, "input.data", item)
		},
		"partials": func(item keyValue) error {
			return readDirList(&v.partialDirs, "input.partials", item)
		},
	})
}

func (v *fileValues) readOutput(section *yaml.Node) error {
	return readKeys("output", section, map[string]keyHandler{
		"routing": func(item keyValue) error {
			return readChoice(&v.routing, "output.routing", item, []Routing{RoutingFile, RoutingDirectory})
		},
		"directory": func(item keyValue) error {
			return readPath(&v.outputDir, "output.directory", item)
		},
		"passthrough": func(item keyValue) error {
			return readPatternList(&v.passthroughPatterns, "output.passthrough", item)
		},
	})
}

// readDefaults merges the user's default: entries into the built-in ones.
// The keys are free: any key that is a valid template name is accepted.
func (v *fileValues) readDefaults(section *yaml.Node) error {
	items, err := entries(section, "in default")
	if err != nil {
		return err
	}
	scope := naming.NewScope(defaultKeyNoun)
	for _, item := range items {
		name, err := scope.Add(item.key)
		if err != nil {
			return wrapAt(item.keyNode, err)
		}
		if err := naming.CheckReserved(item.key, name, defaultKeyNoun); err != nil {
			return wrapAt(item.keyNode, err)
		}
		if err := v.readDefault(name, "default."+item.key, item); err != nil {
			return err
		}
	}
	return nil
}

// readDefault reads one entry of default:, whose template name is name.
func (v *fileValues) readDefault(name, label string, item keyValue) error {
	_, builtIn := v.variables[name]
	switch {
	case name == "permalink":
		if err := readPermalink(&v.permalink, label, item); err != nil {
			return err
		}
		v.variables[name] = string(v.permalink)
		return nil
	case name == "layout":
		// The scope above lets "layout" appear only once, so the value to
		// keep for an empty entry is always the built-in default.
		layout := defaultLayout
		if err := readText(&layout, label, item); err != nil {
			return err
		}
		v.variables[name] = layout
		return nil
	case yamlnode.IsEmpty(item.valueNode) && builtIn:
		return nil
	case yamlnode.IsEmpty(item.valueNode):
		// Templates fail on undefined variables, so a key the user declared
		// always exists, even when it was left empty.
		v.variables[name] = ""
		return nil
	}
	// The special keys (tags, date, sitemap, …) follow the same rules as in
	// front matter, so they are read by the same function.
	key := frontmatter.Key{Written: item.key, InDefault: true}
	value, err := frontmatter.ReadValue(name, key, item.valueNode, v.source)
	if err != nil {
		return diag.WithFile(err, FileName)
	}
	v.variables[name] = value
	return nil
}

func (v *fileValues) checkRequired() error {
	required := []struct {
		node    *yaml.Node
		label   string
		purpose string
	}{
		{v.siteNameNode, "config.site-name", siteNamePurpose},
		{v.siteVersionNode, "config.version", siteVersionPurpose},
		{v.canonicalURLNode, "config.canonical-url", canonicalURLPurpose},
	}
	for _, key := range required {
		if key.node == nil {
			return errorAt(nil, "%s is required; please set %s", key.label, key.purpose)
		}
	}
	return nil
}

// Value readers. Each one leaves its target untouched when the value is
// empty (null or ""), so the default stays.

func readText(target *string, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind != yaml.ScalarNode {
		return errorAt(item.valueNode, "%s must be text, not %s", label, yamlnode.Describe(item.valueNode))
	}
	// Value is the text as written, so 0.10 stays "0.10".
	*target = value.Value
	return nil
}

func readRequiredText(target *string, label, purpose string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return errorAt(item.keyNode, "%s is empty; please set %s", label, purpose)
	}
	return readText(target, label, item)
}

func readBool(target *bool, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	if yamlnode.Resolved(item.valueNode).ShortTag() != "!!bool" {
		return errorAt(item.valueNode, "%s must be true or false, not %s", label, yamlnode.Describe(item.valueNode))
	}
	return item.valueNode.Decode(target)
}

func readPort(target *int, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.ShortTag() != "!!int" {
		return errorAt(item.valueNode, "%s must be a whole number, not %s", label, yamlnode.Describe(item.valueNode))
	}
	var port int64
	// A number too large for int64 fails to decode; it is out of range too.
	if err := value.Decode(&port); err != nil || port < minPort || port > maxPort {
		return errorAt(item.valueNode, "%s is %s; it must be between %d and %d", label, value.Value, minPort, maxPort)
	}
	*target = int(port)
	return nil
}

// readChoice reads a value that must be one of choices. The type parameter
// lets one function fill a Routing, a Permalink or an Engine: "~string"
// accepts any type whose underlying type is string.
func readChoice[T ~string](target *T, label string, item keyValue, choices []T) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind == yaml.ScalarNode {
		for _, choice := range choices {
			if value.Value == string(choice) {
				*target = choice
				return nil
			}
		}
	}
	return errorAt(item.valueNode, "%s is %s; it must be %s", label, yamlnode.Describe(item.valueNode), quotedChoices(choices))
}

func readPermalink(target *Permalink, label string, item keyValue) error {
	choices := []Permalink{PermalinkPath, PermalinkSlug, PermalinkDate}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind == yaml.ScalarNode && strings.HasPrefix(value.Value, "/") {
		return errorAt(item.valueNode, "%s is %s; it must be %s (custom permalinks go in a page's front matter)", label, yamlnode.Describe(item.valueNode), quotedChoices(choices))
	}
	return readChoice(target, label, item, choices)
}

// quotedChoices lists choices for a message: `"jet"`, `"file" or
// "directory"`, `"path", "slug" or "date"`.
func quotedChoices[T ~string](choices []T) string {
	quoted := make([]string, len(choices))
	for i, choice := range choices {
		quoted[i] = strconv.Quote(string(choice))
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " or " + quoted[len(quoted)-1]
}

func readCanonicalURL(target *string, label string, item keyValue) error {
	var written string
	if err := readRequiredText(&written, label, canonicalURLPurpose, item); err != nil {
		return err
	}
	at := item.valueNode
	parsed, err := url.Parse(written)
	// url.Parse lowercases the scheme, so "HTTPS://" is accepted too.
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return errorAt(at, "%s %q is not an absolute http or https URL", label, written)
	}
	if parsed.User != nil {
		return errorAt(at, "%s %q must not contain a user name or password", label, written)
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return errorAt(at, "%s %q must not have a query (the part after \"?\")", label, written)
	}
	if strings.Contains(written, "#") {
		return errorAt(at, "%s %q must not have a fragment (the part after \"#\")", label, written)
	}
	// Templates join the URL with paths that begin with "/".
	*target = strings.TrimSuffix(written, "/")
	return nil
}

func readPath(target *locatedPath, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind != yaml.ScalarNode {
		return errorAt(item.valueNode, "%s must be a path, not %s", label, yamlnode.Describe(item.valueNode))
	}
	cleaned, err := cleanPath(label, item.valueNode, value.Value)
	if err != nil {
		return err
	}
	*target = locatedPath{path: cleaned, node: item.valueNode}
	return nil
}

// readDirList reads a key that accepts a path or a list of paths.
func readDirList(target *locatedDirList, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	switch value.Kind {
	case yaml.ScalarNode:
		cleaned, err := cleanPath(label, item.valueNode, value.Value)
		if err != nil {
			return err
		}
		*target = locatedDirList{dirs: []locatedPath{{path: cleaned, node: item.valueNode}}}
		return nil
	case yaml.SequenceNode:
		var dirs []locatedPath
		if err := readPathItems(&dirs, label, value); err != nil {
			return err
		}
		seenDirs := make(map[string]bool, len(dirs))
		for _, dir := range dirs {
			if seenDirs[dir.path] {
				return errorAt(dir.node, "%s lists %q twice; please keep only one", label, dir.path)
			}
			seenDirs[dir.path] = true
		}
		*target = locatedDirList{dirs: dirs}
		return nil
	}
	return errorAt(item.valueNode, "%s must be a path or a list of paths, not %s", label, yamlnode.Describe(item.valueNode))
}

// readPathList reads a key that accepts only a list of paths.
func readPathList(target *[]locatedPath, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind != yaml.SequenceNode {
		return errorAt(item.valueNode, "%s must be a list of paths, not %s", label, yamlnode.Describe(item.valueNode))
	}
	var located []locatedPath
	if err := readPathItems(&located, label, value); err != nil {
		return err
	}
	*target = located
	return nil
}

func readPathItems(target *[]locatedPath, label string, sequence *yaml.Node) error {
	located := make([]locatedPath, 0, len(sequence.Content))
	for _, entry := range sequence.Content {
		written, err := listItem(label, "a path", entry)
		if err != nil {
			return err
		}
		cleaned, err := cleanPath(label, entry, written)
		if err != nil {
			return err
		}
		located = append(located, locatedPath{path: cleaned, node: entry})
	}
	*target = located
	return nil
}

// readPatternList reads output.passthrough. The patterns are kept as
// written: package scan checks and matches them.
func readPatternList(target *[]locatedPattern, label string, item keyValue) error {
	if yamlnode.IsEmpty(item.valueNode) {
		return nil
	}
	value := yamlnode.Resolved(item.valueNode)
	if value.Kind != yaml.SequenceNode {
		return errorAt(item.valueNode, "%s must be a list of patterns, not %s", label, yamlnode.Describe(item.valueNode))
	}
	patterns := make([]locatedPattern, 0, len(value.Content))
	for _, entry := range value.Content {
		pattern, err := listItem(label, "a pattern", entry)
		if err != nil {
			return err
		}
		patterns = append(patterns, locatedPattern{pattern: pattern, node: entry})
	}
	*target = patterns
	return nil
}

// listItem returns the text of one entry of a list. what is the kind of
// entry for messages, such as "a path".
func listItem(label, what string, entry *yaml.Node) (string, error) {
	if yamlnode.IsEmpty(entry) {
		return "", errorAt(entry, "%s has an empty entry; please remove it", label)
	}
	value := yamlnode.Resolved(entry)
	if value.Kind != yaml.ScalarNode {
		return "", errorAt(entry, "each entry of %s must be %s, not %s", label, what, yamlnode.Describe(entry))
	}
	return value.Value, nil
}

// cleanPath checks that a path written by the user stays inside the project
// directory and returns it cleaned and written with "/". Any ".." is
// refused, even one that would stay inside, as SPEC §4 asks.
func cleanPath(label string, node *yaml.Node, written string) (string, error) {
	slashed := filepath.ToSlash(written)
	// On Windows, "/x" and "C:x" are not absolute for filepath.IsAbs, but
	// they do not stay inside the project either.
	isAbsolute := filepath.IsAbs(written) || filepath.VolumeName(written) != "" || strings.HasPrefix(slashed, "/")
	if isAbsolute || hasParentElement(slashed) {
		return "", errorAt(node, "%s %q is not allowed: paths are relative to the project directory and may not contain \"..\" or be absolute", label, written)
	}
	return path.Clean(slashed), nil
}

func hasParentElement(slashedPath string) bool {
	return slices.Contains(strings.Split(slashedPath, "/"), "..")
}

// keyValue is one entry of a YAML mapping.
type keyValue struct {
	key       string
	keyNode   *yaml.Node
	valueNode *yaml.Node
}

// entries returns the entries of mapping in the order written. where places
// a key in the mapping, in messages, such as "in config".
func entries(mapping *yaml.Node, where string) ([]keyValue, error) {
	// yaml.v3 only reports a repeated key when it decodes into a Go map or
	// struct, not when it builds nodes, so it is checked here.
	firstLineByKey := make(map[string]int)
	items := make([]keyValue, 0, len(mapping.Content)/2)
	// Content holds the keys and values one after the other.
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode, valueNode := mapping.Content[i], mapping.Content[i+1]
		if yamlnode.Resolved(keyNode).Kind != yaml.ScalarNode {
			return nil, errorAt(keyNode, "keys %s must be names, not %s", where, yamlnode.Describe(keyNode))
		}
		key := yamlnode.Resolved(keyNode).Value
		if firstLine, seen := firstLineByKey[key]; seen {
			return nil, errorAt(keyNode, "key %q appears twice %s (first at line %d); please keep only one", key, where, firstLine)
		}
		firstLineByKey[key] = keyNode.Line
		items = append(items, keyValue{key: key, keyNode: keyNode, valueNode: valueNode})
	}
	return items, nil
}

// errorAt returns an error at the position of node in gondola.yaml, or an
// error naming only the file when node is nil (the value is a default).
func errorAt(node *yaml.Node, format string, args ...any) error {
	return wrapAt(node, fmt.Errorf(format, args...))
}

// wrapAt is errorAt for an error that already has its message, such as one
// from package naming.
func wrapAt(node *yaml.Node, err error) error {
	diagErr := &diag.Error{File: FileName, Err: err}
	if node != nil {
		diagErr.Line, diagErr.Col = node.Line, node.Column
	}
	return diagErr
}
