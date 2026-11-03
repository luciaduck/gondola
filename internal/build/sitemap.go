// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package build

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/luciaduck/gondola/internal/scan"
	"github.com/luciaduck/gondola/internal/sitemap"
)

// claimSitemap records that the build writes sitemap.xml, when the sitemap
// is on, so that a page or a passthrough file that would write the same
// file is an error (M10, Q4). It runs after scan, which has claimed the
// passthrough files, and before the pages claim theirs.
func (b *builder) claimSitemap(outputPaths *scan.OutputPaths) error {
	if !b.cfg.Settings.Sitemap {
		return nil
	}
	return outputPaths.Claim(sitemap.FileName, scan.SitemapSource, "")
}

// writeSitemap is stage 7: it writes sitemap.xml into the temp output, when
// the sitemap is on, and prints its line. Every page in pages has been
// written, so robots.txt is already in the temp output if any source makes
// it.
func (b *builder) writeSitemap(pages []preparedPage, outputPaths *scan.OutputPaths) error {
	if !b.cfg.Settings.Sitemap {
		return nil
	}
	rules, robotsFound, err := b.readRobots(outputPaths)
	if err != nil {
		return err
	}
	sitemapPages := make([]sitemap.Page, 0, len(pages))
	for _, page := range pages {
		// The sitemap variable is always a boolean: frontmatter checks its
		// type and gives every page true when no level sets it.
		listed, _ := page.merged.Variables["sitemap"].(bool)
		sitemapPages = append(sitemapPages, sitemap.Page{
			OutputPath: page.location.OutputPath,
			URLPath:    page.location.URLPath,
			Listed:     listed,
			Date:       page.merged.Date,
			HasDate:    page.merged.HasDate,
		})
	}
	text, listedCount, err := sitemap.Build(sitemapPages, b.cfg.Settings.CanonicalURL, rules)
	if err != nil {
		return err
	}
	outputFilePath := path.Join(b.cfg.Output.TempDir(), sitemap.FileName)
	if err := b.writeFile(outputFilePath, text); err != nil {
		return fmt.Errorf("writing %s: %w", outputFilePath, err)
	}
	b.options.Printer.Sitemap(listedCount, robotsFound)
	return nil
}

// readRobots reads robots.txt from the root of the temp output, the file
// that will be published wherever it came from (SPEC §5.12), and returns
// the rules for config.robots-agent. It returns false, and rules that allow
// everything, when there is no robots.txt.
func (b *builder) readRobots(outputPaths *scan.OutputPaths) (sitemap.Rules, bool, error) {
	tempFilePath := path.Join(b.cfg.Output.TempDir(), sitemap.RobotsFileName)
	src, err := os.ReadFile(b.diskPath(tempFilePath))
	if errors.Is(err, fs.ErrNotExist) {
		return sitemap.Rules{}, false, nil
	}
	if err != nil {
		return sitemap.Rules{}, false, fmt.Errorf("reading %s: %w", tempFilePath, withoutPath(err))
	}
	// Errors name the source that wrote robots.txt, the file the user can
	// fix. Every file in the temp output was claimed, so the fallback to
	// the temp file is only a precaution.
	sourcePath := tempFilePath
	if _, claimedBy, found := outputPaths.Source(sitemap.RobotsFileName); found {
		sourcePath = claimedBy
	}
	rules, err := sitemap.ParseRobots(sourcePath, src, b.cfg.Settings.RobotsAgent)
	if err != nil {
		return sitemap.Rules{}, false, err
	}
	return rules, true, nil
}
