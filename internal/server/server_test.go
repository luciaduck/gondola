package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/luciaduck/gondola/internal/build"
	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/console"
	"github.com/luciaduck/gondola/internal/plugin"
	"github.com/luciaduck/gondola/internal/sass"
)

// testVersion is the Gondola version that test builds print.
const testVersion = "v0.0.1"

// minimalConfig is the smallest valid gondola.yaml, with Sass off so that
// the tests run without Dart Sass.
const minimalConfig = "config:\n  site-name: Test\n  version: 1\n  canonical-url: https://example.com\n  sass: false\n"

// builtLine is the last line of a test build of one page, whose fake clock
// always measures 40 ms.
const builtLine = "\\gondola/ Built 1 file and Copied 0 in 0.04 seconds (40.00ms each, v0.0.1)\n"

// testTimeout bounds every wait of these tests, so that a bug that blocks
// the server fails the test instead of hanging until go test gives up. No
// test waits for it when the code is right.
const testTimeout = 10 * time.Second

// signallingPrinter is the console printer, and also tells the test each
// time the help line is printed: the server prints it from its loop, so it
// marks the moment when the loop has finished what came before.
type signallingPrinter struct {
	*console.Printer
	helpPrinted chan struct{}
}

func (p signallingPrinter) ServerHelp() {
	p.Printer.ServerHelp()
	p.helpPrinted <- struct{}{}
}

// listened is a call to Options.Listen.
type listened struct {
	port     int
	listener net.Listener
}

// harness runs Run on a project in the background.
type harness struct {
	t          *testing.T
	projectDir string
	// output receives standard output and standard error together, so the
	// order of the lines can be checked. Read it only after Run returned.
	output   *strings.Builder
	printer  signallingPrinter
	input    *io.PipeWriter
	listened chan listened
	// listenErr, when not nil, is returned by Listen instead of listening.
	listenErr error
	cancel    context.CancelFunc
	done      chan struct{}
	served    bool
}

// newHarness prepares a server on the project in projectDir.
func newHarness(t *testing.T, projectDir string) *harness {
	output := &strings.Builder{}
	stream := console.Stream{Writer: output}
	return &harness{
		t:          t,
		projectDir: projectDir,
		output:     output,
		printer: signallingPrinter{
			Printer:     console.New(stream, stream, os.DirFS(projectDir)),
			helpPrinted: make(chan struct{}, 16),
		},
		listened: make(chan listened, 1),
		done:     make(chan struct{}),
	}
}

// start runs the server with buildSite as its build and input as its
// standard input; a nil input is a pipe that the test writes with typeLine.
func (h *harness) start(buildSite BuildFunc, input io.Reader) {
	if input == nil {
		var reader *io.PipeReader
		reader, h.input = io.Pipe()
		input = reader
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	h.t.Cleanup(func() {
		cancel()
		if h.input != nil {
			h.input.Close()
		}
		h.waitDone()
	})
	go func() {
		defer close(h.done)
		h.served = Run(ctx, Options{
			ProjectDir:     h.projectDir,
			GondolaVersion: testVersion,
			Build:          buildSite,
			Input:          input,
			Printer:        h.printer,
			Listen:         h.listen,
		})
	}()
}

func (h *harness) listen(port int) (net.Listener, error) {
	if h.listenErr != nil {
		h.listened <- listened{port: port}
		return nil, h.listenErr
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	h.listened <- listened{port: port, listener: listener}
	return listener, nil
}

// realBuild builds the project as gondola serve does, with a fake clock.
func (h *harness) realBuild(started chan<- struct{}) BuildFunc {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	return func(configLoaded func(config.Config) error) *build.Failure {
		started <- struct{}{}
		return build.Run(build.Options{
			ProjectDir:     h.projectDir,
			GondolaVersion: testVersion,
			SassStyle:      sass.Expanded,
			Mode:           plugin.Serve,
			Printer:        h.printer.Printer,
			PluginStdout:   h.output,
			PluginStderr:   h.output,
			ConfigLoaded:   configLoaded,
			Now: func() time.Time {
				current := now
				now = now.Add(40 * time.Millisecond)
				return current
			},
		})
	}
}

// waitListening waits until the server listens and returns the site's URL.
func (h *harness) waitListening() string {
	h.t.Helper()
	select {
	case call := <-h.listened:
		if call.port != 1094 {
			h.t.Errorf("Listen was asked for port %d, want the default 1094", call.port)
		}
		return "http://" + call.listener.Addr().String()
	case <-h.done:
		h.t.Fatalf("Run returned before listening; output:\n%s", h.output.String())
	case <-time.After(testTimeout):
		h.t.Fatal("the server did not listen")
	}
	return ""
}

// typeLine writes a line on the server's standard input. It returns once
// the server has read it; the line before it has then been handled.
func (h *harness) typeLine(line string) {
	h.t.Helper()
	if _, err := io.WriteString(h.input, line+"\n"); err != nil {
		h.t.Fatal(err)
	}
}

// waitHelp waits until the help line has been printed once more.
func (h *harness) waitHelp() {
	h.t.Helper()
	receive(h.t, h.printer.helpPrinted, "the help line")
}

// waitDone waits until Run returned, and returns what it reported.
func (h *harness) waitDone() bool {
	h.t.Helper()
	select {
	case <-h.done:
		return h.served
	case <-time.After(testTimeout):
		h.t.Fatal("Run did not return")
		return false
	}
}

// isDone reports whether Run has returned, without waiting.
func (h *harness) isDone() bool {
	select {
	case <-h.done:
		return true
	default:
		return false
	}
}

func receive[T any](t *testing.T, channel <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-channel:
		return value
	case <-time.After(testTimeout):
		t.Fatalf("waiting for %s", what)
		var zero T
		return zero
	}
}

// response is what a GET returned.
type response struct {
	status int
	header http.Header
	body   string
}

func get(t *testing.T, url string) response {
	t.Helper()
	// Without keep-alive, no idle connection is left for Shutdown to close.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: testTimeout}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: string(body)}
}

// checkWaitingPage checks that a response is the waiting page that holds
// heading.
func checkWaitingPage(t *testing.T, got response, heading string) {
	t.Helper()
	if got.status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", got.status)
	}
	if got.header.Get("Cache-Control") != "no-store" || got.header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("headers = %v", got.header)
	}
	if !strings.Contains(got.body, "<h1>"+heading+"</h1>") {
		t.Errorf("the page has not the heading %q", heading)
	}
}

// writeProject creates a project in a new directory called "site" and
// returns its path. files maps paths written with "/" to their contents.
func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()
	projectDir := filepath.Join(t.TempDir(), "site")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		writeFile(t, filepath.Join(projectDir, filepath.FromSlash(name)), content)
	}
	return projectDir
}

func writeFile(t *testing.T, diskPath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(diskPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// checkEmptyDir checks that dir exists and holds nothing.
func checkEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Errorf("%s: %d entries, error %v; want an empty directory", dir, len(entries), err)
	}
}

func TestRunServesAndRebuilds(t *testing.T) {
	projectDir := writeProject(t, map[string]string{"gondola.yaml": minimalConfig, "index.html": "<p>one</p>\n"})
	h := newHarness(t, projectDir)
	started := make(chan struct{}, 16)
	h.start(h.realBuild(started), nil)
	siteURL := h.waitListening()
	h.waitHelp()

	if got := get(t, siteURL+"/"); got.status != http.StatusOK || got.body != "<p>one</p>\n" {
		t.Errorf("GET / = %d %q, want the built page", got.status, got.body)
	}
	writeFile(t, filepath.Join(projectDir, "index.html"), "<p>two</p>\n")
	h.typeLine("g")
	receive(t, started, "the first build")
	receive(t, started, "the second build")
	// Waiting for the help line makes the q below arrive after the build;
	// during it, q would close the server without the help line.
	h.waitHelp()
	h.typeLine("q")
	if !h.waitDone() {
		t.Error("Run reported that it did not serve")
	}

	content, err := os.ReadFile(filepath.Join(projectDir, "_site", "index.html"))
	if err != nil || string(content) != "<p>two</p>\n" {
		t.Errorf("_site/index.html = %q, %v; want the second build", content, err)
	}
	buildLog := "> site is taking a gondola ride\n" +
		"\\gondola/ Shaping ./_site.temp/index.html from ./index.html\n" +
		"\\gondola/ Shipping _site.temp to _site\n" +
		builtLine
	want := buildLog +
		"> asking a gondolier to take _site to port 1094\n" +
		"Server ready at " + siteURL + "\n" +
		"Type g and Enter to rebuild, or q and Enter to close.\n" +
		buildLog +
		"Type g and Enter to rebuild, or q and Enter to close.\n"
	if h.output.String() != want {
		t.Errorf("output:\n got:\n%s\nwant:\n%s", h.output.String(), want)
	}
}

func TestRunFirstBuildFails(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		// wantErrors is what the failed build prints.
		wantErrors string
	}{
		{
			name: "an error in a page",
			files: map[string]string{
				"gondola.yaml": minimalConfig,
				"index.md":     "---\nsite: y\n---\n",
			},
			wantErrors: "error: index.md:2:1: front matter key \"site\" is reserved by Gondola (templates use it for the site settings); please rename it\n" +
				"    2 | site: y\n" +
				"      | ^\n" +
				"build failed: 1 error\n",
		},
		{
			name: "a missing plugin, which stops the build before the old output is deleted",
			files: map[string]string{
				"gondola.yaml":   minimalConfig + "  plugin: [missing.sh]\n",
				"index.md":       "hello\n",
				"_site/old.html": "<p>old</p>\n",
			},
			wantErrors: "error: gondola.yaml:6:12: config.plugin \"missing.sh\" does not exist; please check the path\n" +
				"    6 |   plugin: [missing.sh]\n" +
				"      |            ^\n" +
				"build failed: 1 error\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, test.files)
			h := newHarness(t, projectDir)
			started := make(chan struct{}, 16)
			h.start(h.realBuild(started), nil)
			siteURL := h.waitListening()

			checkEmptyDir(t, filepath.Join(projectDir, "_site"))
			checkWaitingPage(t, get(t, siteURL+"/"), "Build failed")
			checkWaitingPage(t, get(t, siteURL+"/old.html"), "Build failed")

			writeFile(t, filepath.Join(projectDir, "gondola.yaml"), minimalConfig)
			writeFile(t, filepath.Join(projectDir, "index.md"), "hello\n")
			h.typeLine("g")
			receive(t, started, "the first build")
			receive(t, started, "the second build")
			h.typeLine("q")
			h.waitDone()

			if _, err := os.Stat(filepath.Join(projectDir, "_site", "index.html")); err != nil {
				t.Errorf("the second build did not publish: %v", err)
			}
			wantStart := "> site is taking a gondola ride\n" + test.wantErrors +
				"> asking a gondolier to take _site to port 1094\n" +
				"Server ready at " + siteURL + "\n" +
				"Type g and Enter to rebuild, or q and Enter to close.\n"
			if !strings.HasPrefix(h.output.String(), wantStart) {
				t.Errorf("output:\n got:\n%s\nwant it to start with:\n%s", h.output.String(), wantStart)
			}
		})
	}
}

func TestRunWithoutConfig(t *testing.T) {
	projectDir := writeProject(t, nil)
	h := newHarness(t, projectDir)
	h.start(h.realBuild(make(chan struct{}, 16)), nil)
	if h.waitDone() {
		t.Error("Run reported that it served")
	}
	if len(h.listened) != 0 {
		t.Error("Run listened without a configuration")
	}
	want := "> site is taking a gondola ride\n" +
		"error: gondola.yaml not found in " + filepath.ToSlash(projectDir) + "; it holds the site settings and must set at least config.site-name, config.version and config.canonical-url\n" +
		"build failed: 1 error\n"
	if h.output.String() != want {
		t.Errorf("output:\n got:\n%s\nwant:\n%s", h.output.String(), want)
	}
}

func TestRunPortUnavailable(t *testing.T) {
	projectDir := writeProject(t, map[string]string{"gondola.yaml": minimalConfig, "index.html": "<p>one</p>\n"})
	h := newHarness(t, projectDir)
	h.listenErr = errors.New("listen tcp 127.0.0.1:1094: bind: address already in use")
	h.start(h.realBuild(make(chan struct{}, 16)), nil)
	if h.waitDone() {
		t.Error("Run reported that it served")
	}
	want := "> site is taking a gondola ride\n" +
		"\\gondola/ Shaping ./_site.temp/index.html from ./index.html\n" +
		"\\gondola/ Shipping _site.temp to _site\n" +
		builtLine +
		"> asking a gondolier to take _site to port 1094\n" +
		"error: serving on localhost:1094: listen tcp 127.0.0.1:1094: bind: address already in use; set config.port in gondola.yaml to use another port\n"
	if h.output.String() != want {
		t.Errorf("output:\n got:\n%s\nwant:\n%s", h.output.String(), want)
	}
}

func TestRunConfigChangedWhileServing(t *testing.T) {
	tests := []struct {
		name      string
		newConfig string
		wantError string
	}{
		{
			name:      "port",
			newConfig: minimalConfig + "  port: 8080\n",
			wantError: "error: gondola.yaml: config.port changed from 1094 to 8080 while serving; restart gondola serve to use it\n",
		},
		{
			name:      "output directory",
			newConfig: minimalConfig + "output:\n  directory: public/\n",
			wantError: "error: gondola.yaml: output.directory changed from \"_site\" to \"public\" while serving; restart gondola serve to use it\n",
		},
		{
			name:      "both, of which the port is reported",
			newConfig: minimalConfig + "  port: 8080\noutput:\n  directory: public\n",
			wantError: "error: gondola.yaml: config.port changed from 1094 to 8080 while serving; restart gondola serve to use it\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, map[string]string{"gondola.yaml": minimalConfig, "index.html": "<p>one</p>\n"})
			h := newHarness(t, projectDir)
			h.start(h.realBuild(make(chan struct{}, 16)), nil)
			siteURL := h.waitListening()
			h.waitHelp()

			writeFile(t, filepath.Join(projectDir, "gondola.yaml"), test.newConfig)
			h.typeLine("g")
			h.waitHelp()
			checkWaitingPage(t, get(t, siteURL+"/"), "Build failed")
			checkEmptyDir(t, filepath.Join(projectDir, "_site"))
			h.typeLine("q")
			h.waitDone()

			wantEnd := "> site is taking a gondola ride\n" + test.wantError + "build failed: 1 error\n" +
				"Type g and Enter to rebuild, or q and Enter to close.\n"
			if !strings.HasSuffix(h.output.String(), wantEnd) {
				t.Errorf("output:\n got:\n%s\nwant it to end with:\n%s", h.output.String(), wantEnd)
			}
		})
	}
}

// fakeBuilds stands in for the build when a test needs to hold one while it
// runs. Builds are numbered from 1.
type fakeBuilds struct {
	cfg config.Config
	// block holds the builds that wait for release before returning.
	block map[int]bool
	// fail holds the builds that fail.
	fail    map[int]bool
	started chan int
	release chan struct{}
	// count is only changed by the server's loop; read it after Run
	// returned.
	count int
}

func newFakeBuilds(block, fail map[int]bool) *fakeBuilds {
	return &fakeBuilds{
		cfg:     config.Config{Settings: config.Settings{Port: 1094}, Output: config.Output{Dir: "_site"}},
		block:   block,
		fail:    fail,
		started: make(chan int, 16),
		release: make(chan struct{}),
	}
}

func (f *fakeBuilds) build(configLoaded func(config.Config) error) *build.Failure {
	f.count++
	number := f.count
	if err := configLoaded(f.cfg); err != nil {
		return &build.Failure{Errors: []error{err}}
	}
	f.started <- number
	if f.block[number] {
		<-f.release
	}
	if f.fail[number] {
		return &build.Failure{Errors: []error{errors.New("index.md: broken")}}
	}
	return nil
}

func TestRunBuildingPage(t *testing.T) {
	builds := newFakeBuilds(map[int]bool{2: true}, nil)
	h := newHarness(t, writeProject(t, nil))
	h.start(builds.build, nil)
	siteURL := h.waitListening()

	h.typeLine("g")
	receive(t, builds.started, "the first build")
	receive(t, builds.started, "the second build")
	checkWaitingPage(t, get(t, siteURL+"/"), "Your site is being shipped with Gondola.")
	checkWaitingPage(t, get(t, siteURL+"/style.css"), "Your site is being shipped with Gondola.")
	builds.release <- struct{}{}
	h.typeLine("q")
	h.waitDone()
}

func TestRunFailedRebuild(t *testing.T) {
	builds := newFakeBuilds(nil, map[int]bool{2: true})
	projectDir := writeProject(t, map[string]string{"_site/index.html": "<p>one</p>\n"})
	h := newHarness(t, projectDir)
	h.start(builds.build, nil)
	siteURL := h.waitListening()
	h.waitHelp()
	if got := get(t, siteURL+"/"); got.status != http.StatusOK || got.body != "<p>one</p>\n" {
		t.Errorf("GET / = %d %q, want the site", got.status, got.body)
	}

	h.typeLine("g")
	h.waitHelp()
	checkEmptyDir(t, filepath.Join(projectDir, "_site"))
	checkWaitingPage(t, get(t, siteURL+"/"), "Build failed")

	h.typeLine("g")
	h.waitHelp()
	// The fake build writes nothing, so the file server lists the empty
	// output directory.
	if got := get(t, siteURL+"/"); got.status != http.StatusOK || strings.Contains(got.body, "Build failed") {
		t.Errorf("GET / after a successful build = %d %q, want the file server's answer", got.status, got.body)
	}
	h.typeLine("q")
	h.waitDone()
	if builds.count != 3 {
		t.Errorf("%d builds, want 3", builds.count)
	}
	if !strings.Contains(h.output.String(), "error: index.md: broken\nbuild failed: 1 error\nType g and Enter to rebuild, or q and Enter to close.\n") {
		t.Errorf("the failure was not printed, or not followed by the help line:\n%s", h.output.String())
	}
}

func TestRunClosesAfterTheRunningBuild(t *testing.T) {
	tests := []struct {
		name  string
		close func(h *harness)
	}{
		{"q", func(h *harness) { h.typeLine("q") }},
		{"Ctrl+C", func(h *harness) {
			// A g that waits during the build must not start another one
			// after Ctrl+C.
			h.typeLine("g")
			h.typeLine("x")
			h.cancel()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			builds := newFakeBuilds(map[int]bool{2: true}, nil)
			h := newHarness(t, writeProject(t, nil))
			h.start(builds.build, nil)
			h.waitListening()
			h.typeLine("g")
			receive(t, builds.started, "the first build")
			receive(t, builds.started, "the second build")

			test.close(h)
			if h.isDone() {
				t.Fatal("Run returned before the build finished")
			}
			builds.release <- struct{}{}
			if !h.waitDone() {
				t.Error("Run reported that it did not serve")
			}
			if builds.count != 2 {
				t.Errorf("%d builds, want 2", builds.count)
			}
			// Only the first help line, after Server ready: the build that
			// ends with closing prints none. Ctrl+C reaches the loop at
			// once; a typed q goes through the reading goroutine and may
			// arrive after the build ended, so TestLoopQuitDuringBuild
			// checks q on the loop itself.
			if test.name != "Ctrl+C" {
				return
			}
			help := "Type g and Enter to rebuild, or q and Enter to close.\n"
			if strings.Count(h.output.String(), help) != 1 {
				t.Errorf("output:\n%s\nwant the help line once", h.output.String())
			}
		})
	}
}

// TestLoopQuitDuringBuild checks that a rebuild that ends after q was
// received prints no help line and closes the server.
func TestLoopQuitDuringBuild(t *testing.T) {
	builds := newFakeBuilds(map[int]bool{1: true}, nil)
	output := &strings.Builder{}
	stream := console.Stream{Writer: output}
	s := &devServer{
		options:     Options{Build: builds.build, Printer: console.New(stream, stream, os.DirFS(t.TempDir()))},
		port:        builds.cfg.Settings.Port,
		outputDir:   builds.cfg.Output.Dir,
		configKnown: true,
	}
	received := newCommands()
	signal(received.rebuild)
	closedNormally := make(chan bool, 1)
	go func() { closedNormally <- s.loop(context.Background(), received, nil) }()
	receive(t, builds.started, "the rebuild")
	close(received.quit)
	builds.release <- struct{}{}
	if !receive(t, closedNormally, "the loop to return") {
		t.Error("the loop reported that it did not close normally")
	}
	if output.Len() != 0 {
		t.Errorf("output %q, want nothing", output.String())
	}
}

func TestRunUnknownLinePrintsHelp(t *testing.T) {
	builds := newFakeBuilds(nil, nil)
	h := newHarness(t, writeProject(t, nil))
	h.start(builds.build, nil)
	h.waitListening()
	h.waitHelp()
	h.typeLine("")
	h.waitHelp()
	h.typeLine("rebuild")
	h.waitHelp()
	h.typeLine("q")
	h.waitDone()
	if builds.count != 1 {
		t.Errorf("%d builds, want 1", builds.count)
	}
	help := "Type g and Enter to rebuild, or q and Enter to close.\n"
	if strings.Count(h.output.String(), help) != 3 {
		t.Errorf("output:\n%s\nwant the help line 3 times", h.output.String())
	}
}

func TestRunEndOfInputKeepsServing(t *testing.T) {
	builds := newFakeBuilds(nil, nil)
	projectDir := writeProject(t, map[string]string{"_site/index.html": "<p>one</p>\n"})
	h := newHarness(t, projectDir)
	h.start(builds.build, strings.NewReader(""))
	siteURL := h.waitListening()
	if got := get(t, siteURL+"/"); got.status != http.StatusOK {
		t.Errorf("GET / = %d, want 200", got.status)
	}
	if h.isDone() {
		t.Fatal("Run returned at the end of input")
	}
	h.cancel()
	if !h.waitDone() {
		t.Error("Run reported that it did not serve")
	}
}

func TestCheckConfig(t *testing.T) {
	s := &devServer{port: 1094, outputDir: "_site"}
	tests := []struct {
		name      string
		port      int
		outputDir string
		want      string
	}{
		{"unchanged", 1094, "_site", ""},
		{"port", 8080, "_site", "gondola.yaml: config.port changed from 1094 to 8080 while serving; restart gondola serve to use it"},
		{"output directory", 1094, "build/www", "gondola.yaml: output.directory changed from \"_site\" to \"build/www\" while serving; restart gondola serve to use it"},
		{"letter case counts", 1094, "_Site", "gondola.yaml: output.directory changed from \"_site\" to \"_Site\" while serving; restart gondola serve to use it"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := s.checkConfig(config.Config{Settings: config.Settings{Port: test.port}, Output: config.Output{Dir: test.outputDir}})
			got := ""
			if err != nil {
				got = err.Error()
			}
			if got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
