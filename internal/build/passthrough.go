// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/luciaduck/gondola/internal/scan"
)

// copyPassthroughFiles is stage 6: it copies every passthrough file into the
// temp output. Any error stops the build: scan has already refused what the
// user could get wrong, so what is left is a problem with the disk.
func (b *builder) copyPassthroughFiles(files []scan.PassthroughFile) error {
	for _, file := range files {
		outputFilePath := path.Join(b.cfg.Output.TempDir(), file.OutputPath)
		if err := b.copyFile(file.SourcePath, outputFilePath); err != nil {
			return fmt.Errorf("copying %s to %s: %w", file.SourcePath, outputFilePath, err)
		}
	}
	return nil
}

// copyFile copies the contents of the file at sourcePath to outputFilePath,
// both relative to the project root and written with "/". Only the contents
// are copied: the copy gets filePermissions and the current time, so the
// output does not depend on the permissions of the source.
func (b *builder) copyFile(sourcePath, outputFilePath string) (err error) {
	source, err := os.Open(b.diskPath(sourcePath))
	if err != nil {
		return withoutPath(err)
	}
	defer source.Close()
	diskPath := b.diskPath(outputFilePath)
	if err := os.MkdirAll(filepath.Dir(diskPath), directoryPermissions); err != nil {
		return withoutPath(err)
	}
	destination, err := os.OpenFile(diskPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePermissions)
	if err != nil {
		return withoutPath(err)
	}
	// A named result lets the deferred Close report its error: for a file
	// being written, Close is where a full disk may show up.
	defer func() {
		if closeErr := destination.Close(); err == nil && closeErr != nil {
			err = withoutPath(closeErr)
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return withoutPath(err)
	}
	return nil
}
