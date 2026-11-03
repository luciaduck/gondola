// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package data loads the data files of input.data, which templates read
// under the variable data: _data/site-info.json becomes data.siteInfo, and
// _data/blog/authors.yaml becomes data.blog.authors.
package data

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/naming"
	"github.com/luciaduck/gondola/internal/scan"
)

// The nouns that name a key in the messages of package naming.
const (
	fileNameNoun      = "data file name"
	directoryNameNoun = "data directory name"
)

// dataVariable is the template variable that holds the data files, the
// first word of the keys written in messages (data.blog.authors).
const dataVariable = "data"

// Load reads every data file in the data directories of input and returns
// them as the value of the template variable data: a map from template name
// to the file's contents, with a nested map for each subdirectory. The map
// is never nil.
//
// The directories are read in the order written in gondola.yaml, each one
// sorted by name. A subdirectory with the same name in two data directories
// is one map holding the files of both; two files, or a file and a
// directory, that become the same key are an error naming both. Names that
// begin with "." or "_" are skipped, as package scan does, and so are files
// with other extensions, since a data directory may also hold templates.
//
// A default data directory that does not exist is skipped; one written in
// gondola.yaml must exist. Errors stop the loading at once: data files are
// not pages, whose errors are collected (SPEC §5.15).
func Load(projectDir string, input config.Input) (map[string]any, error) {
	root := newDirectoryEntry("")
	for i, dir := range input.DataDirs.Dirs {
		exists, err := checkDataDir(projectDir, dir, input.DataDirs.FromDefault, positionAt(input.DataDirPositions, i))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if err := loadDirectory(projectDir, root, dir, dataVariable); err != nil {
			return nil, err
		}
	}
	return root.directoryValue(), nil
}

// positionAt returns positions[i], or the zero Position (no position) when
// the list is shorter, as in a Config built by hand.
func positionAt(positions []config.Position, i int) config.Position {
	if i < len(positions) {
		return positions[i]
	}
	return config.Position{}
}

// checkDataDir reports whether the data directory dir exists. A default
// directory that does not exist is skipped silently (a minimal site has no
// _data); every other problem is an error.
func checkDataDir(projectDir, dir string, fromDefault bool, position config.Position) (bool, error) {
	subject := fmt.Sprintf("input.data %q", dir)
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

// loadDirectory adds the files and subdirectories of dirPath (relative to
// the project root, with "/") to directory. fullKey is how templates reach
// directory, such as "data.blog", for messages.
func loadDirectory(projectDir string, directory *entry, dirPath, fullKey string) error {
	// os.ReadDir returns the entries sorted by name, so the order of the
	// keys, and which of two colliding files is named first, never depends
	// on the file system.
	items, err := os.ReadDir(filepath.Join(projectDir, filepath.FromSlash(dirPath)))
	if err != nil {
		return fmt.Errorf("reading directory %q: %w", dirPath, err)
	}
	for _, item := range items {
		if err := loadItem(projectDir, directory, dirPath, item, fullKey); err != nil {
			return err
		}
	}
	return nil
}

// loadItem adds one entry of a data directory: a subdirectory, a data file,
// or nothing for anything else.
func loadItem(projectDir string, directory *entry, dirPath string, item fs.DirEntry, fullKey string) error {
	name := item.Name()
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return nil
	}
	sourcePath := dirPath + "/" + name
	parse, isDataFile := parserFor(name)
	switch {
	case item.Type()&fs.ModeSymlink != 0:
		return checkLink(projectDir, sourcePath, isDataFile)
	case item.IsDir():
		return loadSubdirectory(projectDir, directory, sourcePath, fullKey)
	case item.Type().IsRegular() && isDataFile:
		return loadFile(projectDir, directory, sourcePath, parse, fullKey)
	}
	// Files with other extensions (templates, a README) and special files
	// such as pipes are ignored.
	return nil
}

// checkLink refuses a symbolic link that would be loaded: a link to a
// directory, or one with a data file extension. Gondola does not follow
// links (SPEC §5.6, rule 4); any other link is skipped, as it would be
// ignored anyway.
func checkLink(projectDir, sourcePath string, isDataFile bool) error {
	// os.Stat follows the link only to learn whether it points at a
	// directory; nothing behind it is read. A broken link counts as a file.
	target, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(sourcePath)))
	pointsAtDir := err == nil && target.IsDir()
	if pointsAtDir || isDataFile {
		return scan.LinkError(sourcePath)
	}
	return nil
}

func loadSubdirectory(projectDir string, directory *entry, sourcePath, fullKey string) error {
	key, err := keyOf(sourcePath, path.Base(sourcePath), directoryNameNoun, true)
	if err != nil {
		return err
	}
	subdirectory, err := directory.addDirectory(key, sourcePath, fullKey+"."+key)
	if err != nil {
		return err
	}
	return loadDirectory(projectDir, subdirectory, sourcePath, fullKey+"."+key)
}

func loadFile(projectDir string, directory *entry, sourcePath string, parse parseFunc, fullKey string) error {
	name := path.Base(sourcePath)
	key, err := keyOf(sourcePath, strings.TrimSuffix(name, path.Ext(name)), fileNameNoun, false)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(sourcePath)))
	if err != nil {
		return fmt.Errorf("reading %s: %w", sourcePath, err)
	}
	value, err := parseFile(sourcePath, src, parse)
	if err != nil {
		return err
	}
	return directory.addFile(key, sourcePath, value, fullKey+"."+key)
}

// keyOf returns the template name of a file or directory name, after the
// UTF-8 check that package scan makes for the files it uses (SPEC §4).
func keyOf(sourcePath, name, noun string, isDirectory bool) (string, error) {
	if err := scan.CheckNameEncoding(strings.Split(sourcePath, "/"), isDirectory); err != nil {
		return "", err
	}
	key, err := naming.TemplateName(name, noun)
	if err != nil {
		return "", &diag.Error{File: sourcePath, Err: err}
	}
	return key, nil
}

// entry is what one key under data holds while the files are loaded: the
// value of a file, or the keys of a directory.
type entry struct {
	// sourcePath is the file, or the first directory, that made the key,
	// for the message about a collision.
	sourcePath  string
	isDirectory bool
	value       any
	children    map[string]*entry
}

func newDirectoryEntry(sourcePath string) *entry {
	return &entry{sourcePath: sourcePath, isDirectory: true, children: make(map[string]*entry)}
}

// addDirectory returns the entry of the subdirectory key, creating it the
// first time. A directory with the same key in another data directory is
// the same entry, so their files end up in one map. fullKey is how
// templates reach the subdirectory, for messages.
func (e *entry) addDirectory(key, sourcePath, fullKey string) (*entry, error) {
	existing, found := e.children[key]
	if !found {
		subdirectory := newDirectoryEntry(sourcePath)
		e.children[key] = subdirectory
		return subdirectory, nil
	}
	if existing.isDirectory {
		return existing, nil
	}
	return nil, collisionError(existing, &entry{sourcePath: sourcePath, isDirectory: true}, fullKey)
}

// addFile adds the value of a data file under key.
func (e *entry) addFile(key, sourcePath string, value any, fullKey string) error {
	added := &entry{sourcePath: sourcePath, value: value}
	if existing, found := e.children[key]; found {
		return collisionError(existing, added, fullKey)
	}
	e.children[key] = added
	return nil
}

// collisionError names the two sources of one key, the earlier one first.
func collisionError(earlier, later *entry, fullKey string) error {
	if !earlier.isDirectory && !later.isDirectory {
		return fmt.Errorf("data files %q and %q both become %s; please rename one", earlier.sourcePath, later.sourcePath, fullKey)
	}
	return fmt.Errorf("%s %q and %s %q both become %s; please rename one", earlier.kind(), earlier.sourcePath, later.kind(), later.sourcePath, fullKey)
}

// kind names the entry in messages.
func (e *entry) kind() string {
	if e.isDirectory {
		return "data directory"
	}
	return "data file"
}

// directoryValue returns the keys of a directory entry as the map that
// templates read.
func (e *entry) directoryValue() map[string]any {
	result := make(map[string]any, len(e.children))
	for key, child := range e.children {
		if child.isDirectory {
			result[key] = child.directoryValue()
		} else {
			result[key] = child.value
		}
	}
	return result
}
