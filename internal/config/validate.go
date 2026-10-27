package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// validate runs the checks that involve more than one key. Checks on a
// single value happen while it is read.
func (v *fileValues) validate() error {
	if v.settings.FencedBlocks && !v.settings.Markdown {
		return errorAt(v.fencedBlocksNode, "config.fenced-blocks is true but config.markdown is false; fenced blocks only work in Markdown pages")
	}
	return v.checkDirectories()
}

// roleDir is a role directory with the words that name it in messages.
type roleDir struct {
	// subject names the directory in messages, such as
	// `input.templates "_components/includes"`.
	subject string
	// label is the key alone, such as "input.data".
	label string
	dir   locatedPath
}

func newRoleDir(label string, dir locatedPath) roleDir {
	subject := fmt.Sprintf("%s %q", label, dir.path)
	if dir.node == nil {
		subject += " (the default)"
	}
	return roleDir{subject: subject, label: label, dir: dir}
}

// inputRoleDirs returns the template, data and partials directories, in
// that order.
func (v *fileValues) inputRoleDirs() []roleDir {
	var dirs []roleDir
	for _, list := range []struct {
		label string
		dirs  locatedDirList
	}{
		{"input.templates", v.templateDirs},
		{"input.data", v.dataDirs},
		{"input.partials", v.partialDirs},
	} {
		for _, dir := range list.dirs.dirs {
			dirs = append(dirs, newRoleDir(list.label, dir))
		}
	}
	return dirs
}

// deletedDirs returns the output directory and the temp output, which every
// build deletes.
func (v *fileValues) deletedDirs() []roleDir {
	tempDir := locatedPath{path: v.outputDir.path + TempDirSuffix, node: v.outputDir.node}
	return []roleDir{
		newRoleDir("output.directory", v.outputDir),
		{subject: fmt.Sprintf("the temp directory %q", tempDir.path), label: "the temp directory", dir: tempDir},
	}
}

// checkDirectories checks that role directories leave the content directory
// alone, and that the directories a build deletes hold none of the user's
// sources. Several input roles may share a directory (SPEC §1).
func (v *fileValues) checkDirectories() error {
	inputDirs := v.inputRoleDirs()
	deletedDirs := v.deletedDirs()
	for _, role := range append(append([]roleDir{}, inputDirs...), deletedDirs...) {
		if err := checkApartFromContent(role, v.contentDir); err != nil {
			return err
		}
	}
	for _, deleted := range deletedDirs {
		for _, input := range inputDirs {
			if err := checkApartFromDeleted(deleted, input); err != nil {
				return err
			}
		}
		for _, plugin := range v.pluginPaths {
			if isWithin(plugin.path, deleted.dir.path) {
				return errorAt(firstNode(deleted.dir.node, plugin.node), "%s contains config.plugin %q; every build deletes it, so please move the plugin or choose another output directory", deleted.subject, plugin.path)
			}
		}
	}
	return nil
}

// checkApartFromContent checks that role may be inside the content
// directory but is neither the content directory nor contains it.
func checkApartFromContent(role roleDir, content locatedPath) error {
	const rule = "role directories may be inside it but must not be it or contain it"
	at := firstNode(role.dir.node, content.node)
	if role.dir.path == content.path {
		return errorAt(at, "%s is the content directory; %s", role.subject, rule)
	}
	if isWithin(content.path, role.dir.path) {
		return errorAt(at, "%s contains the content directory %q; %s", role.subject, content.path, rule)
	}
	return nil
}

// checkApartFromDeleted checks that a directory a build deletes neither
// equals, contains nor sits inside an input directory.
func checkApartFromDeleted(deleted, input roleDir) error {
	at := firstNode(deleted.dir.node, input.dir.node)
	switch {
	case deleted.dir.path == input.dir.path:
		return errorAt(at, "%s is also used by %s; every build deletes it, so please choose another output directory", deleted.subject, input.label)
	case isWithin(input.dir.path, deleted.dir.path):
		return errorAt(at, "%s contains %s; every build deletes it, so please choose another output directory", deleted.subject, input.subject)
	case isWithin(deleted.dir.path, input.dir.path):
		return errorAt(at, "%s is inside %s; please choose an output directory outside it", deleted.subject, input.subject)
	}
	return nil
}

// isWithin reports whether the cleaned relative path is parentDir or inside
// it. The comparison is by text: two spellings of one directory that differ
// in letter case are not caught here.
func isWithin(path, parentDir string) bool {
	return parentDir == "." || path == parentDir || strings.HasPrefix(path, parentDir+"/")
}

// firstNode returns the first node that is not nil, so that an error points
// at a value the user wrote rather than at a default.
func firstNode(nodes ...*yaml.Node) *yaml.Node {
	for _, node := range nodes {
		if node != nil {
			return node
		}
	}
	return nil
}
