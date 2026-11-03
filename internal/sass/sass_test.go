package sass

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
	"github.com/luciaduck/gondola/internal/scan"
)

// writeSite writes files (path with "/" → content) into a new project
// directory and returns it.
func writeSite(t *testing.T, files map[string]string) string {
	t.Helper()
	projectDir := t.TempDir()
	for name, content := range files {
		filePath := filepath.Join(projectDir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir
}

// defaultInput is the input section of a gondola.yaml that sets nothing.
func defaultInput() config.Input {
	return config.Input{
		ContentDir:          ".",
		PartialDirs:         config.DirList{Dirs: []string{"_components/sass"}, FromDefault: true},
		PartialDirPositions: []config.Position{{}},
	}
}

// entryPoints returns the entry points for source paths, with the output
// path that scan gives them.
func entryPoints(sourcePaths ...string) []scan.SassEntryPoint {
	var entries []scan.SassEntryPoint
	for _, sourcePath := range sourcePaths {
		outputPath := strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".css"
		entries = append(entries, scan.SassEntryPoint{SourcePath: sourcePath, OutputPath: outputPath})
	}
	return entries
}

// compileSite compiles the entry points of a site written by writeSite and
// returns the messages of the compile errors and the warnings printed.
func compileSite(t *testing.T, files map[string]string, input config.Input, sourcePaths ...string) (compileErrors, warnings []string, outputDir string) {
	t.Helper()
	executablePath := requireSass(t)
	projectDir := writeSite(t, files)
	compiler, err := Start(Options{
		ProjectDir:     projectDir,
		ExecutablePath: executablePath,
		Input:          input,
		Style:          Expanded,
		PrintWarning:   func(text string) { warnings = append(warnings, text) },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer closeCompiler(t, compiler)
	outputDir = t.TempDir()
	errs, err := compiler.CompileAll(entryPoints(sourcePaths...), outputDir)
	if err != nil {
		t.Fatalf("unexpected error that stops the build: %v", err)
	}
	for _, compileErr := range errs {
		compileErrors = append(compileErrors, compileErr.Error())
	}
	return compileErrors, warnings, outputDir
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		entry string
		want  string
	}{
		{
			name:  "missing semicolon",
			files: map[string]string{"assets/main.scss": "a {\n  color: red\n  b: c;\n}\n"},
			entry: "assets/main.scss",
			want:  `assets/main.scss:3:4: expected ";".`,
		},
		{
			name:  "end of file",
			files: map[string]string{"assets/main.scss": "a {"},
			entry: "assets/main.scss",
			want:  `assets/main.scss:1:4: expected end of rule.`,
		},
		{
			name: "in a partial of the partials directory, after an emoji",
			files: map[string]string{
				"assets/main.scss":              "@use 'broken';\n",
				"_components/sass/_broken.scss": "/* 😀 */ a { b: $nope; }\n",
			},
			entry: "assets/main.scss",
			want:  `_components/sass/_broken.scss:1:19: Undefined variable.`,
		},
		{
			name: "in a partial next to the entry point, with a byte order mark",
			files: map[string]string{
				"assets/main.scss": "@use 'bom';\n",
				"assets/_bom.scss": "\xEF\xBB\xBFa { b: $nope; }\n",
			},
			entry: "assets/main.scss",
			want:  `assets/_bom.scss:1:8: Undefined variable.`,
		},
		{
			name:  "entry point with a byte order mark",
			files: map[string]string{"assets/main.scss": "\xEF\xBB\xBFa { b: $nope; }\n"},
			entry: "assets/main.scss",
			want:  `assets/main.scss:1:8: Undefined variable.`,
		},
		{
			name:  "CRLF",
			files: map[string]string{"assets/main.scss": "a {\r\n  b: $nope;\r\n}\r\n"},
			entry: "assets/main.scss",
			want:  `assets/main.scss:2:6: Undefined variable.`,
		},
		{
			name:  "missing import",
			files: map[string]string{"assets/main.scss": "@use 'nope';\n"},
			entry: "assets/main.scss",
			want:  `assets/main.scss:1:1: Can't find stylesheet to import.`,
		},
		{
			name:  "indented syntax",
			files: map[string]string{"assets/print.sass": "body\n  color: $nope\n"},
			entry: "assets/print.sass",
			want:  `assets/print.sass:2:10: Undefined variable.`,
		},
		{
			name:  "SCSS in a .sass file",
			files: map[string]string{"assets/print.sass": "body { color: red; }\n"},
			entry: "assets/print.sass",
			want:  `assets/print.sass:1:6: Expected newline.`,
		},
		{
			name:  "UTF-16",
			files: map[string]string{"assets/main.scss": "\xFF\xFEa\x00"},
			entry: "assets/main.scss",
			want:  `assets/main.scss: the file is saved as UTF-16; please save it as UTF-8`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compileErrors, warnings, outputDir := compileSite(t, test.files, defaultInput(), test.entry)
			if want := []string{test.want}; !reflect.DeepEqual(compileErrors, want) {
				t.Errorf("errors\n got %q\nwant %q", compileErrors, want)
			}
			if len(warnings) > 0 {
				t.Errorf("unexpected warnings %q", warnings)
			}
			if files := readTree(t, outputDir); len(files) > 0 {
				t.Errorf("a failed entry point wrote %v", sortedKeys(files))
			}
		})
	}
}

func TestCompileErrorsAreReturnedOnce(t *testing.T) {
	files := map[string]string{
		"a.scss":                        "@use 'broken';\n",
		"b.scss":                        "@use 'broken';\n",
		"c.scss":                        "x { y: $own; }\n",
		"d.scss":                        "ok { y: z; }\n",
		"_components/sass/_broken.scss": "a { b: $nope; }\n",
	}
	compileErrors, _, outputDir := compileSite(t, files, defaultInput(), "a.scss", "b.scss", "c.scss", "d.scss")
	want := []string{
		`_components/sass/_broken.scss:1:8: Undefined variable.`,
		`c.scss:1:8: Undefined variable.`,
	}
	if !reflect.DeepEqual(compileErrors, want) {
		t.Errorf("errors\n got %q\nwant %q", compileErrors, want)
	}
	// The other entry points are still compiled.
	if got := sortedKeys(readTree(t, outputDir)); !reflect.DeepEqual(got, []string{"d.css"}) {
		t.Errorf("written %v, want [d.css]", got)
	}
}

func TestCompileStopsWhenAnEntryPointCannotBeRead(t *testing.T) {
	executablePath := requireSass(t)
	compiler, err := Start(Options{ProjectDir: t.TempDir(), ExecutablePath: executablePath, Input: defaultInput(), Style: Compressed})
	if err != nil {
		t.Fatal(err)
	}
	defer closeCompiler(t, compiler)
	compileErrors, err := compiler.CompileAll(entryPoints("assets/gone.scss"), t.TempDir())
	if len(compileErrors) > 0 {
		t.Errorf("unexpected compile errors %v", compileErrors)
	}
	if !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "reading assets/gone.scss: ") {
		t.Errorf("error %v, want one that reads assets/gone.scss and wraps fs.ErrNotExist", err)
	}
}

func TestWarnings(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		entries []string
		// want are the warnings printed; a want ending in "..." is a
		// prefix, for messages whose end is Dart Sass's text.
		want []string
	}{
		{
			name: "deprecated @import",
			files: map[string]string{
				"assets/main.scss":               "@import 'spacing';\na { margin: $gap; }\n",
				"_components/sass/_spacing.scss": "$gap: 1rem;\n",
			},
			entries: []string{"assets/main.scss"},
			want:    []string{"warning: assets/main.scss:1:9: Sass @import rules are deprecated..."},
		},
		{
			name:    "@debug after an emoji",
			files:   map[string]string{"assets/main.scss": "a { b: c; }\n/* 😀 */ @debug 'x';\n"},
			entries: []string{"assets/main.scss"},
			want:    []string{"debug: assets/main.scss:2:12: x"},
		},
		{
			name:    "@warn has no position",
			files:   map[string]string{"assets/main.scss": "@warn 'careful';\n"},
			entries: []string{"assets/main.scss"},
			want:    []string{"warning: careful"},
		},
		{
			name: "a partial used twice warns once",
			files: map[string]string{
				"a.scss":                      "@use 'loud';\n",
				"b.scss":                      "@use 'loud';\n",
				"_components/sass/_loud.scss": "@debug 'once';\n",
			},
			entries: []string{"a.scss", "b.scss"},
			want:    []string{"debug: _components/sass/_loud.scss:1:1: once"},
		},
		{
			name: "warnings keep their order",
			files: map[string]string{
				"a.scss": "@debug 'first';\n",
				"b.scss": "@debug 'second';\n@debug 'third';\n",
			},
			entries: []string{"a.scss", "b.scss"},
			want:    []string{"debug: a.scss:1:1: first", "debug: b.scss:1:1: second", "debug: b.scss:2:1: third"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compileErrors, warnings, _ := compileSite(t, test.files, defaultInput(), test.entries...)
			if len(compileErrors) > 0 {
				t.Fatalf("unexpected errors %q", compileErrors)
			}
			if len(warnings) != len(test.want) {
				t.Fatalf("warnings\n got %q\nwant %q", warnings, test.want)
			}
			for i, want := range test.want {
				prefix, isPrefix := strings.CutSuffix(want, "...")
				if (isPrefix && !strings.HasPrefix(warnings[i], prefix)) || (!isPrefix && warnings[i] != want) {
					t.Errorf("warning %d\n got %q\nwant %q", i, warnings[i], want)
				}
			}
		})
	}
}

func TestWarningsWithoutPrinter(t *testing.T) {
	executablePath := requireSass(t)
	projectDir := writeSite(t, map[string]string{"a.scss": "@warn 'x';\nb { c: d; }\n"})
	compiler, err := Start(Options{ProjectDir: projectDir, ExecutablePath: executablePath, Input: defaultInput(), Style: Compressed})
	if err != nil {
		t.Fatal(err)
	}
	defer closeCompiler(t, compiler)
	compileErrors, err := compiler.CompileAll(entryPoints("a.scss"), t.TempDir())
	if err != nil || len(compileErrors) > 0 {
		t.Fatalf("errors %v, %v", err, compileErrors)
	}
}

func TestPartialDirsAreSearchedInOrder(t *testing.T) {
	files := map[string]string{
		"main.scss":          "@use 'x';\na { b: x.$v; }\n",
		"first/_x.scss":      "$v: first;\n",
		"second/_x.scss":     "$v: second;\n",
		"second/_only.scss":  "$w: only;\n",
		"other.scss":         "@use 'only';\na { b: only.$w; }\n",
		"_components/sass/z": "the default is not used when partials are written",
	}
	input := defaultInput()
	input.PartialDirs = config.DirList{Dirs: []string{"first", "second"}}
	input.PartialDirPositions = []config.Position{{Line: 6, Col: 14}, {Line: 6, Col: 21}}
	compileErrors, _, outputDir := compileSite(t, files, input, "main.scss", "other.scss")
	if len(compileErrors) > 0 {
		t.Fatalf("unexpected errors %q", compileErrors)
	}
	want := map[string]string{
		"main.css":  "a {\n  b: first;\n}\n",
		"other.css": "a {\n  b: only;\n}\n",
	}
	if got := readTree(t, outputDir); !reflect.DeepEqual(got, want) {
		t.Errorf("written %q, want %q", got, want)
	}
}

func TestCheckPartialDirs(t *testing.T) {
	written := func(dirs ...string) config.Input {
		input := defaultInput()
		input.PartialDirs = config.DirList{Dirs: dirs}
		input.PartialDirPositions = nil
		for i := range dirs {
			input.PartialDirPositions = append(input.PartialDirPositions, config.Position{Line: 6, Col: 13 + 8*i})
		}
		return input
	}
	tests := []struct {
		name     string
		files    map[string]string
		links    map[string]string // link name → target
		input    config.Input
		wantDirs []string
		wantErr  string
	}{
		{
			name:     "default that does not exist is skipped",
			input:    defaultInput(),
			wantDirs: nil,
		},
		{
			name:     "default that exists",
			files:    map[string]string{"_components/sass/_x.scss": ""},
			input:    defaultInput(),
			wantDirs: []string{"_components/sass"},
		},
		{
			name:     "written ones that exist, in order",
			files:    map[string]string{"b/_x.scss": "", "a/_x.scss": ""},
			input:    written("b", "a"),
			wantDirs: []string{"b", "a"},
		},
		{
			name:     "empty list",
			input:    written(),
			wantDirs: nil,
		},
		{
			name:    "written one that does not exist",
			files:   map[string]string{"a/_x.scss": ""},
			input:   written("a", "styles"),
			wantErr: `gondola.yaml:6:21: input.partials "styles" does not exist`,
		},
		{
			name:    "written one that is a file",
			files:   map[string]string{"styles": ""},
			input:   written("styles"),
			wantErr: `gondola.yaml:6:13: input.partials "styles" is a file, not a directory`,
		},
		{
			name:    "default that is a file",
			files:   map[string]string{"_components/sass": ""},
			input:   defaultInput(),
			wantErr: `gondola.yaml: input.partials "_components/sass" (the default) is a file, not a directory`,
		},
		{
			name:    "link",
			files:   map[string]string{"real/_x.scss": ""},
			links:   map[string]string{"styles": "real"},
			input:   written("styles"),
			wantErr: `styles is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeSite(t, test.files)
			for name, target := range test.links {
				if err := os.Symlink(target, filepath.Join(projectDir, name)); err != nil {
					t.Skipf("cannot create a symbolic link: %v", err)
				}
			}
			dirs, err := existingPartialDirs(projectDir, test.input)
			checkError(t, err, test.wantErr)
			if test.wantErr != "" {
				return
			}
			var wantDirs []string
			for _, dir := range test.wantDirs {
				wantDirs = append(wantDirs, filepath.Join(projectDir, filepath.FromSlash(dir)))
			}
			if !reflect.DeepEqual(dirs, wantDirs) {
				t.Errorf("dirs %q, want %q", dirs, wantDirs)
			}
		})
	}
}

func TestStartChecksPartialDirsBeforeDartSass(t *testing.T) {
	input := defaultInput()
	input.PartialDirs = config.DirList{Dirs: []string{"styles"}}
	input.PartialDirPositions = []config.Position{{Line: 7, Col: 13}}
	// No executable is given: the check must fail before Dart Sass starts.
	_, err := Start(Options{ProjectDir: t.TempDir(), ExecutablePath: filepath.Join(t.TempDir(), "no-sass"), Input: input, Style: Compressed})
	checkError(t, err, `gondola.yaml:7:13: input.partials "styles" does not exist`)
}

func TestStartRefusesUnknownStyle(t *testing.T) {
	_, err := Start(Options{ProjectDir: t.TempDir(), Input: defaultInput()})
	checkError(t, err, "unknown Sass output style 0")
}

func TestStartWithMissingExecutable(t *testing.T) {
	_, err := Start(Options{ProjectDir: t.TempDir(), ExecutablePath: filepath.Join(t.TempDir(), "no-sass"), Input: defaultInput(), Style: Compressed})
	if err == nil || !strings.HasPrefix(err.Error(), "starting Dart Sass: ") {
		t.Errorf("error %v, want one that starts with %q", err, "starting Dart Sass: ")
	}
}

func TestDisplayedPath(t *testing.T) {
	projectDir := t.TempDir()
	compiler := &Compiler{projectDir: projectDir}
	outside := filepath.Join(filepath.Dir(projectDir), "elsewhere", "x.scss")
	tests := []struct {
		filePath string
		want     string
	}{
		{filepath.Join(projectDir, "assets", "main.scss"), "assets/main.scss"},
		{filepath.Join(projectDir, "..x", "a.scss"), "..x/a.scss"},
		{outside, filepath.ToSlash(outside)},
	}
	for _, test := range tests {
		if got := compiler.displayedPath(test.filePath); got != test.want {
			t.Errorf("displayedPath(%q) = %q, want %q", test.filePath, got, test.want)
		}
	}
}

// TestDisplayedPathResolvedRoot checks that a file Dart Sass names through
// the resolved project directory is shown relative to the project too, as
// on Windows when the project is reached through a short name.
func TestDisplayedPathResolvedRoot(t *testing.T) {
	baseDir := t.TempDir()
	writtenDir := filepath.Join(baseDir, "RUNNER~1", "site")
	resolvedDir := filepath.Join(baseDir, "runneradmin", "site")
	compiler := &Compiler{projectDir: writtenDir, resolvedProjectDir: resolvedDir}
	elsewhere := filepath.Join(baseDir, "runneradmin", "other", "x.scss")
	tests := []struct {
		filePath string
		want     string
	}{
		{filepath.Join(writtenDir, "assets", "main.scss"), "assets/main.scss"},
		{filepath.Join(resolvedDir, "_components", "sass", "_broken.scss"), "_components/sass/_broken.scss"},
		{elsewhere, filepath.ToSlash(elsewhere)},
	}
	for _, test := range tests {
		if got := compiler.displayedPath(test.filePath); got != test.want {
			t.Errorf("displayedPath(%q) = %q, want %q", test.filePath, got, test.want)
		}
	}
}

// TestStartResolvesProjectDir checks that Start keeps the resolved project
// directory when it differs from the written one, here through a link.
func TestStartResolvesProjectDir(t *testing.T) {
	executablePath := requireSass(t)
	realDir := t.TempDir()
	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
	compiler, err := Start(Options{ProjectDir: linkDir, ExecutablePath: executablePath, Input: defaultInput(), Style: Compressed})
	if err != nil {
		t.Fatal(err)
	}
	defer compiler.Close()
	wantResolved, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}
	if compiler.resolvedProjectDir != wantResolved {
		t.Errorf("resolvedProjectDir = %q, want %q", compiler.resolvedProjectDir, wantResolved)
	}
}
