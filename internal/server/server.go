// Copyright (c) 2026 Lucia Capurri
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

// Package server is the development server of `gondola serve`: it builds the
// site, serves the output directory over HTTP on localhost, and builds again
// when the user types g and Enter.
package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"

	"github.com/luciaduck/gondola/internal/build"
	"github.com/luciaduck/gondola/internal/config"
)

// localHost is the only host that gondola serve listens on: it is a tool for
// local development and has no setting for the host.
const localHost = "localhost"

// outputDirPermissions are those of the empty output directory recreated
// after a failed build, as the build creates its directories.
const outputDirPermissions = 0o755

// Printer is the part of the console that the server writes to.
// *console.Printer implements it.
type Printer interface {
	Error(err error)
	Failure(errs []error, limitReached bool)
	AskingGondolier(outputDir string, port int)
	ServerReady(siteURL string)
	ServerHelp()
}

// BuildFunc runs one build of the site. It calls configLoaded with the
// settings as soon as gondola.yaml has been read, before anything is
// deleted, and stops with the error that configLoaded returns, if any.
type BuildFunc func(configLoaded func(config.Config) error) *build.Failure

// Options are what the server needs.
type Options struct {
	// ProjectDir is the directory that holds gondola.yaml.
	ProjectDir     string
	GondolaVersion string
	Build          BuildFunc
	// Input is where the commands g and q are read, standard input for
	// gondola serve.
	Input   io.Reader
	Printer Printer
	// Listen opens the listener on the configured port. ListenOnLocalhost
	// is the one gondola serve uses.
	Listen func(port int) (net.Listener, error)
}

// ListenOnLocalhost listens on port of localhost.
func ListenOnLocalhost(port int) (net.Listener, error) {
	return net.Listen("tcp", net.JoinHostPort(localHost, strconv.Itoa(port)))
}

// siteState is what the server answers requests with.
type siteState int32

const (
	// siteReady serves the output directory.
	siteReady siteState = iota
	// siteBuilding answers with the page shown while a build runs.
	siteBuilding
	// siteFailed answers with the page shown after a failed build.
	siteFailed
)

// devServer holds the state of one gondola serve.
type devServer struct {
	options Options
	// port and outputDir are those of the first build. They cannot change
	// while serving, because the listener and the served directory stay.
	port int
	// outputDir is relative to the project root, written with "/".
	outputDir   string
	configKnown bool
	pages       waitingPages
	// state is a siteState. It is atomic because the HTTP handlers read it
	// while the loop of Run changes it.
	state atomic.Int32
	files http.Handler
}

// Run builds the site, serves it, and builds again on every g, until q or
// until ctx is cancelled (Ctrl+C). A build that is running is always allowed
// to finish. Run reports whether it served: it does not when the first build
// could not read gondola.yaml or the port could not be opened, and it has
// then printed why.
func Run(ctx context.Context, options Options) (served bool) {
	s := &devServer{options: options}
	pages, err := renderWaitingPages(options.GondolaVersion)
	if err != nil {
		options.Printer.Error(err)
		return false
	}
	s.pages = pages
	s.runBuild(s.recordConfig)
	if !s.configKnown {
		return false
	}
	options.Printer.AskingGondolier(s.outputDir, s.port)
	listener, err := options.Listen(s.port)
	if err != nil {
		options.Printer.Error(fmt.Errorf("serving on %s:%d: %w; set config.port in %s to use another port", localHost, s.port, err, config.FileName))
		return false
	}
	options.Printer.ServerReady("http://" + listener.Addr().String())
	options.Printer.ServerHelp()

	httpServer := &http.Server{Handler: s}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- httpServer.Serve(listener) }()
	received := newCommands()
	go readCommands(options.Input, received)

	served = s.loop(ctx, received, serveErrors)
	// Shutdown waits for the requests in progress, and makes Serve return.
	httpServer.Shutdown(context.Background())
	return served
}

// loop answers the commands until the server closes, and reports whether it
// closed normally. Every rebuild ends with the help line, so the terminal
// always ends with what can be typed next. Closing is checked before anything else, so a g that
// waited during the last build does not start another one after q or
// Ctrl+C.
func (s *devServer) loop(ctx context.Context, received commands, serveErrors <-chan error) (closedNormally bool) {
	for {
		if closing(ctx, received) {
			return true
		}
		select {
		case <-ctx.Done():
			return true
		case <-received.quit:
			return true
		case err := <-serveErrors:
			s.options.Printer.Error(fmt.Errorf("serving on %s:%d: %w", localHost, s.port, err))
			return false
		case <-received.rebuild:
			s.runBuild(s.checkConfig)
			// After a q or Ctrl+C that came during the build the server
			// closes, and the help line would offer what is gone.
			if !closing(ctx, received) {
				s.options.Printer.ServerHelp()
			}
		case <-received.unknown:
			s.options.Printer.ServerHelp()
		}
	}
}

// closing reports whether the server has been asked to close.
func closing(ctx context.Context, received commands) bool {
	select {
	case <-ctx.Done():
		return true
	case <-received.quit:
		return true
	default:
		return false
	}
}

// runBuild runs one build and sets what the server answers with afterwards.
// After a failure the output directory is left empty, also when the build
// stopped before deleting it, so an old output is never served.
func (s *devServer) runBuild(configLoaded func(config.Config) error) {
	s.state.Store(int32(siteBuilding))
	failure := s.options.Build(configLoaded)
	if failure == nil {
		s.state.Store(int32(siteReady))
		return
	}
	s.state.Store(int32(siteFailed))
	s.options.Printer.Failure(failure.Errors, failure.LimitReached)
	if s.configKnown {
		if err := s.emptyOutputDir(); err != nil {
			s.options.Printer.Error(err)
		}
	}
}

// recordConfig is the configLoaded of the first build: it keeps the port
// and the output directory for as long as the server runs.
func (s *devServer) recordConfig(cfg config.Config) error {
	s.port = cfg.Settings.Port
	s.outputDir = cfg.Output.Dir
	s.configKnown = true
	s.files = http.FileServer(http.Dir(s.outputDiskDir()))
	return nil
}

// checkConfig is the configLoaded of every later build: it refuses a port or
// an output directory different from the first build's (SPEC §5.16, point
// 8). When both changed, the port is reported; restarting applies both.
func (s *devServer) checkConfig(cfg config.Config) error {
	if cfg.Settings.Port != s.port {
		return fmt.Errorf("%s: config.port changed from %d to %d while serving; restart gondola serve to use it", config.FileName, s.port, cfg.Settings.Port)
	}
	if cfg.Output.Dir != s.outputDir {
		return fmt.Errorf("%s: output.directory changed from %q to %q while serving; restart gondola serve to use it", config.FileName, s.outputDir, cfg.Output.Dir)
	}
	return nil
}

// emptyOutputDir replaces the output directory with an empty one.
func (s *devServer) emptyOutputDir() error {
	diskDir := s.outputDiskDir()
	if err := os.RemoveAll(diskDir); err != nil {
		return fmt.Errorf("deleting %s: %w%s", s.outputDir, withoutPath(err), build.InUseHint(err, s.outputDir))
	}
	if err := os.MkdirAll(diskDir, outputDirPermissions); err != nil {
		return fmt.Errorf("creating %s: %w", s.outputDir, withoutPath(err))
	}
	return nil
}

// outputDiskDir is the output directory on disk.
func (s *devServer) outputDiskDir() string {
	return filepath.Join(s.options.ProjectDir, filepath.FromSlash(s.outputDir))
}

// ServeHTTP answers a request with the site, or with a waiting page while a
// build runs or after it failed.
func (s *devServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch siteState(s.state.Load()) {
	case siteBuilding:
		writeWaitingPage(w, s.pages.building)
	case siteFailed:
		writeWaitingPage(w, s.pages.failed)
	default:
		s.files.ServeHTTP(w, r)
	}
}

// writeWaitingPage answers with a page that stands in for the site. 503 says
// that the site is not available for now; no-store keeps the browser from
// showing the page again once the site is back.
func writeWaitingPage(w http.ResponseWriter, page []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write(page)
}

// withoutPath removes the path that package os puts into its errors: it is
// joined to the project directory and written with the platform's
// separator, while messages name paths relative to the project root.
func withoutPath(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return pathErr.Err
	}
	return err
}
