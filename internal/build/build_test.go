package build

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// layoutPage is a layout that wraps the page in a <main>, for tests whose
// pages use Jet.
const layoutPage = "<main>{{ copy|raw }}</main>\n"

// withLayout is minimalConfig with every page wrapped in layoutPage.
const withLayout = minimalConfig + "default:\n  layout: page\n"

func TestBuildWritesPages(t *testing.T) {
	tests := []struct {
		name       string
		files      map[string]string
		wantOutput map[string]string
		wantLog    string
	}{
		{
			name:       "an HTML page without layout is written without its front matter",
			files:      map[string]string{"index.html": "---\ntitle: Home\n---\n<p>{{ not Jet }}</p>\n"},
			wantOutput: map[string]string{"index.html": "<p>{{ not Jet }}</p>\n"},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shaping ./_site.temp/index.html from ./index.html\n" +
				"\\gondola/ Shipping _site.temp to _site\n" +
				"\\gondola/ Built 1 file and Copied 0 in 0.04 seconds (40.00ms each, v0.0.1)\n",
		},
		{
			name: "a Markdown page in a layout",
			files: map[string]string{
				"gondola.yaml":                    withLayout,
				"_components/includes/page.jet":   layoutPage,
				"blog/post.md":                    "---\ntitle: Post\n---\n# {{ title }}\n",
				"blog/_draft.md":                  "# skipped\n",
				"_components/includes/unused.jet": "{{ never read }}",
			},
			wantOutput: map[string]string{"blog/post.html": "<main><h1 id=\"post\">Post</h1>\n</main>\n"},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shaping ./_site.temp/blog/post.html from ./blog/post.md (jet)\n" +
				"\\gondola/ Shipping _site.temp to _site\n" +
				"\\gondola/ Built 1 file and Copied 0 in 0.04 seconds (40.00ms each, v0.0.1)\n",
		},
		{
			name: "fenced blocks are off unless the configuration turns them on",
			files: map[string]string{
				"index.md": "::: +note\ntext\n:::\n",
			},
			wantOutput: map[string]string{"index.html": "<p>::: +note\ntext\n:::</p>\n"},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shaping ./_site.temp/index.html from ./index.md\n" +
				"\\gondola/ Shipping _site.temp to _site\n" +
				"\\gondola/ Built 1 file and Copied 0 in 0.04 seconds (40.00ms each, v0.0.1)\n",
		},
		{
			name: "routing directory and an output directory of the user's",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "output:\n  routing: directory\n  directory: public\n  passthrough: [favicon.ico]\n",
				"about.html":   "<p>About</p>\n",
				"favicon.ico":  "icon",
			},
			wantOutput: map[string]string{"about/index.html": "<p>About</p>\n", "favicon.ico": "icon"},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shaping ./public.temp/about/index.html from ./about.html\n" +
				"\\gondola/ Shipping public.temp to public\n" +
				"\\gondola/ Built 1 file and Copied 1 in 0.04 seconds (20.00ms each, v0.0.1)\n",
		},
		{
			name: "only passthrough files",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "output:\n  passthrough: [robots.txt]\n",
				"robots.txt":   "User-agent: *\n",
			},
			wantOutput: map[string]string{"robots.txt": "User-agent: *\n"},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shipping _site.temp to _site\n" +
				"\\gondola/ Built 0 files and Copied 1 in 0.04 seconds (40.00ms each, v0.0.1)\n",
		},
		{
			name:       "an empty site is not an error",
			files:      map[string]string{},
			wantOutput: map[string]string{},
			wantLog: "> site is taking a gondola ride\n" +
				"\\gondola/ Shipping _site.temp to _site\n" +
				"\\gondola/ Built 0 files and Copied 0 in 0.04 seconds (v0.0.1)\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, test.files)
			result := runBuild(t, projectDir)
			if result.failure != nil {
				t.Fatalf("build failed: %q", result.errorTexts())
			}
			if result.stdout != test.wantLog {
				t.Errorf("log:\n got:\n%s\nwant:\n%s", result.stdout, test.wantLog)
			}
			if result.stderr != "" {
				t.Errorf("standard error: %q", result.stderr)
			}
			outputDir := "_site"
			if strings.Contains(test.files["gondola.yaml"], "directory: public") {
				outputDir = "public"
			}
			got := readTree(t, filepath.Join(projectDir, outputDir))
			if got == nil {
				got = map[string]string{}
			}
			if !reflect.DeepEqual(got, test.wantOutput) {
				t.Errorf("output:\n got %q\nwant %q", got, test.wantOutput)
			}
			if dirExists(t, filepath.Join(projectDir, outputDir+".temp")) {
				t.Error("the temp output still exists")
			}
		})
	}
}

// TestBuildPagesErrors checks which errors are collected and where the
// build stops: errors in preparing stop it before Sass and rendering;
// errors in rendering are collected for every page.
func TestBuildPagesErrors(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantErrors  []string
		wantShaping []string
	}{
		{
			name: "errors while preparing several pages are collected; no page is rendered",
			files: map[string]string{
				"gondola.yaml":                  withLayout,
				"_components/includes/page.jet": layoutPage,
				"a.md":                          "---\ntitle: [x\n---\n",
				"b.md":                          "---\nlayout: missing\n---\n",
				"c.md":                          "{{ undefinedVariable }}\n",
				"d.md":                          "fine\n",
			},
			wantErrors: []string{
				"a.md:2:1: invalid YAML: did not find expected ',' or ']'",
				`b.md: layout "missing" was not found; looked for missing.jet and missing.html in _components/includes (the default)`,
			},
		},
		{
			name: "errors while rendering are collected; the other pages are written",
			files: map[string]string{
				"gondola.yaml":                  withLayout,
				"_components/includes/page.jet": layoutPage,
				"a.md":                          "{{ undefinedVariable }}\n",
				"b.md":                          "fine\n",
				"c.html":                        "\n{{ if }}\n",
			},
			wantErrors: []string{
				"a.md:1: variable \"undefinedVariable\" is not defined; the available variables are author, collections, data, date, excludeFromFleet, fleet, layout, page, permalink, site, sitemap, tags, title",
				"c.html:2: parsing if: unexpected token '}}' (expected term)",
			},
			wantShaping: []string{"\\gondola/ Shaping ./_site.temp/b.html from ./b.md (jet)"},
		},
		{
			name: "a page that writes the same file as a passthrough file",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "output:\n  passthrough: [\"*.html\"]\n",
				"404.html":     "<p>copied</p>\n",
				"404.md":       "rendered\n",
			},
			wantErrors: []string{
				`passthrough file "404.html" and page "404.md" both write "404.html" in the output; please rename or remove one`,
			},
		},
		{
			name: "two pages that write the same file",
			files: map[string]string{
				"about.html": "---\npermalink: /about-us.html\n---\n",
				"other.md":   "---\npermalink: /About-Us.html\n---\n",
			},
			wantErrors: []string{
				`page "about.html" and page "other.md" both write "about-us.html" and "About-Us.html", which differ only in letter case, in the output; please rename or remove one`,
			},
		},
		{
			name: "a page whose permalink needs a title",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "default:\n  permalink: slug\n",
				"post.md":      "text\n",
			},
			wantErrors: []string{
				`post.md: permalink "slug" needs a title, but the page has none; please set title or choose another permalink`,
			},
		},
		{
			name: "a page cannot write a file where another page needs a directory",
			files: map[string]string{
				"about.html":     "---\npermalink: /blog\n---\n",
				"blog/post.html": "<p>post</p>\n",
			},
			wantErrors: []string{
				"blog/post.html: writing _site.temp/blog/post.html: not a directory",
			},
			wantShaping: []string{"\\gondola/ Shaping ./_site.temp/blog from ./about.html"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := writeProject(t, test.files)
			result := runBuild(t, projectDir)
			if result.failure == nil {
				t.Fatal("the build did not fail")
			}
			if got := result.errorTexts(); !reflect.DeepEqual(got, test.wantErrors) {
				t.Errorf("errors:\n got %q\nwant %q", got, test.wantErrors)
			}
			if result.failure.LimitReached {
				t.Error("LimitReached is true")
			}
			if got := shapingLines(result.stdout); !reflect.DeepEqual(got, test.wantShaping) {
				t.Errorf("Shaping lines:\n got %q\nwant %q", got, test.wantShaping)
			}
			checkNotPublished(t, projectDir, result)
		})
	}
}

// shapingLines returns the Shaping lines of a log, or nil.
func shapingLines(log string) []string {
	var lines []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, " Shaping ") {
			lines = append(lines, line)
		}
	}
	return lines
}

// checkNotPublished checks what a failed build leaves: no output directory
// (it is deleted at the start of every build), the temp output for
// inspection, and no Shipping or Built line.
func checkNotPublished(t *testing.T, projectDir string, result buildResult) {
	t.Helper()
	if dirExists(t, filepath.Join(projectDir, "_site")) {
		t.Error("the output directory exists after a failed build")
	}
	if !dirExists(t, filepath.Join(projectDir, "_site.temp")) {
		t.Error("the temp output was removed after a failed build")
	}
	if strings.Contains(result.stdout, "Shipping") || strings.Contains(result.stdout, "Built") {
		t.Errorf("a failed build printed the lines of a published one:\n%s", result.stdout)
	}
}

func TestBuildErrorLimit(t *testing.T) {
	tests := []struct {
		brokenPages      int
		wantErrors       int
		wantLimitReached bool
	}{
		{1, 1, false},
		{maxCollectedErrors - 1, maxCollectedErrors - 1, false},
		// The build stops at the limit, so it cannot know whether there
		// were more: exactly the limit counts as reaching it.
		{maxCollectedErrors, maxCollectedErrors, true},
		{maxCollectedErrors + 1, maxCollectedErrors, true},
		{3 * maxCollectedErrors, maxCollectedErrors, true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprint(test.brokenPages), func(t *testing.T) {
			files := map[string]string{}
			for i := range test.brokenPages {
				files[fmt.Sprintf("page%03d.md", i)] = "---\ntitle: [\n---\n"
			}
			result := runBuild(t, writeProject(t, files))
			if result.failure == nil {
				t.Fatal("the build did not fail")
			}
			if len(result.failure.Errors) != test.wantErrors || result.failure.LimitReached != test.wantLimitReached {
				t.Errorf("got %d errors, limit reached %v; want %d, %v",
					len(result.failure.Errors), result.failure.LimitReached, test.wantErrors, test.wantLimitReached)
			}
			// The pages are prepared in the order of the walk, so the
			// errors name the first pages.
			if first := result.errorTexts()[0]; !strings.HasPrefix(first, "page000.md:") {
				t.Errorf("first error = %q", first)
			}
		})
	}
}

func TestBuildErrorLimitWhileRendering(t *testing.T) {
	files := map[string]string{
		"gondola.yaml":                  withLayout,
		"_components/includes/page.jet": layoutPage,
	}
	for i := range maxCollectedErrors + 5 {
		files[fmt.Sprintf("page%03d.md", i)] = "{{ missing }}\n"
	}
	result := runBuild(t, writeProject(t, files))
	if result.failure == nil || len(result.failure.Errors) != maxCollectedErrors || !result.failure.LimitReached {
		t.Fatalf("failure = %+v", result.failure)
	}
}

// TestBuildStopsAtOnce checks errors outside the pages: they stop the build
// at once, alone.
func TestBuildStopsAtOnce(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantError   string
		wantDeleted bool
	}{
		{
			name:      "gondola.yaml is invalid",
			files:     map[string]string{"gondola.yaml": minimalConfig + "  sas: true\n"},
			wantError: `gondola.yaml:6:3: unknown key "sas" in config; please remove it or check its spelling`,
		},
		{
			name: "a broken alias file when fenced blocks are on",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "  fenced-blocks: true\n",
				"alias.squero": "not an alias\n",
				"a.md":         "---\ntitle: [\n---\n",
			},
			wantError: "alias.squero:1:1: alias definition must start with at least three colons",
		},
		{
			name: "a pattern that matches no file",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "output:\n  passthrough: [robot.txt]\n",
			},
			wantError:   `gondola.yaml:7:17: output.passthrough "robot.txt" matches no file (role directories and symbolic links are never copied)`,
			wantDeleted: true,
		},
		{
			name: "a template directory written in gondola.yaml that does not exist",
			files: map[string]string{
				"gondola.yaml": minimalConfig + "input:\n  templates: layouts\n",
				"a.md":         "---\ntitle: [\n---\n",
			},
			wantError:   `gondola.yaml:7:14: input.templates "layouts" does not exist`,
			wantDeleted: true,
		},
		{
			name: "a malformed data file, after every page was prepared",
			files: map[string]string{
				"_data/team.json": "{",
				"index.html":      "<p>ok</p>\n",
			},
			wantError:   "_data/team.json:1:1: invalid JSON: unexpected end of JSON input",
			wantDeleted: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{"_site/old.html": "old"}
			for name, content := range test.files {
				files[name] = content
			}
			projectDir := writeProject(t, files)
			result := runBuild(t, projectDir)
			texts := result.errorTexts()
			if len(texts) != 1 || texts[0] != test.wantError {
				t.Fatalf("errors = %q, want only %q", texts, test.wantError)
			}
			if deleted := !dirExists(t, filepath.Join(projectDir, "_site")); deleted != test.wantDeleted {
				t.Errorf("output deleted = %v, want %v", deleted, test.wantDeleted)
			}
		})
	}
}

func TestBuildWithoutConfig(t *testing.T) {
	projectDir := writeProject(t, map[string]string{"_site/old.html": "old"})
	if err := os.Remove(filepath.Join(projectDir, "gondola.yaml")); err != nil {
		t.Fatal(err)
	}
	result := runBuild(t, projectDir)
	want := []string{"gondola.yaml not found in the current directory; it holds the site settings and must set at least config.site-name, config.version and config.canonical-url"}
	if got := result.errorTexts(); !reflect.DeepEqual(got, want) {
		t.Errorf("errors:\n got %q\nwant %q", got, want)
	}
	if result.stdout != "> site is taking a gondola ride\n" {
		t.Errorf("log = %q", result.stdout)
	}
	// Without a configuration the output directory is unknown, so nothing
	// is deleted.
	if !dirExists(t, filepath.Join(projectDir, "_site")) {
		t.Error("the output directory was deleted")
	}
}

// TestBuildReplacesOldOutput checks the cleaning at the start and the
// publishing at the end: files of an earlier build and a leftover temp
// output disappear.
func TestBuildReplacesOldOutput(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"index.html":             "<p>new</p>\n",
		"_site/stale.html":       "old",
		"_site/index.html":       "<p>old</p>\n",
		"_site.temp/leftover.md": "from a failed build",
	})
	result := runBuild(t, projectDir)
	if result.failure != nil {
		t.Fatalf("build failed: %q", result.errorTexts())
	}
	want := map[string]string{"index.html": "<p>new</p>\n"}
	if got := readTree(t, filepath.Join(projectDir, "_site")); !reflect.DeepEqual(got, want) {
		t.Errorf("output = %q, want %q", got, want)
	}
	if dirExists(t, filepath.Join(projectDir, "_site.temp")) {
		t.Error("the temp output still exists")
	}
}

// TestBuildFailureDeletesOldOutput checks the answer to M9's Q1: after a
// failed build the old output is gone and the temp output holds the pages
// written so far.
func TestBuildFailureDeletesOldOutput(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml":                  withLayout,
		"_components/includes/page.jet": layoutPage,
		"a.md":                          "fine\n",
		"b.md":                          "{{ missing }}\n",
		"_site/index.html":              "<p>old</p>\n",
	})
	result := runBuild(t, projectDir)
	if result.failure == nil {
		t.Fatal("the build did not fail")
	}
	checkNotPublished(t, projectDir, result)
	want := map[string]string{"a.html": "<main><p>fine</p>\n</main>\n"}
	if got := readTree(t, filepath.Join(projectDir, "_site.temp")); !reflect.DeepEqual(got, want) {
		t.Errorf("temp output = %q, want %q", got, want)
	}
}

// TestBuildPassthroughCopiesContentsOnly checks that a copy does not keep
// the source's permissions, so the output does not depend on them.
func TestBuildPassthroughCopiesContentsOnly(t *testing.T) {
	projectDir := writeProject(t, map[string]string{
		"gondola.yaml": minimalConfig + "output:\n  passthrough: [run.sh]\n",
		"run.sh":       "#!/bin/sh\n",
	})
	if err := os.Chmod(filepath.Join(projectDir, "run.sh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if result := runBuild(t, projectDir); result.failure != nil {
		t.Fatalf("build failed: %q", result.errorTexts())
	}
	info, err := os.Stat(filepath.Join(projectDir, "_site", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	// The umask may remove bits from filePermissions but never adds any.
	if info.Mode().Perm()&^filePermissions != 0 {
		t.Errorf("permissions = %v, want at most %v", info.Mode().Perm(), os.FileMode(filePermissions))
	}
}

// TestBuildSass checks stage 4 inside the build: compile errors are
// collected and do not stop the pages from rendering (M9, Q2); warnings go
// to standard error and do not stop the build.
func TestBuildSass(t *testing.T) {
	requireSass(t)
	sassConfig := strings.Replace(withLayout, "  sass: false\n", "", 1)
	t.Run("a compile error is collected with the page errors", func(t *testing.T) {
		projectDir := writeProject(t, map[string]string{
			"gondola.yaml":                  sassConfig,
			"_components/includes/page.jet": layoutPage,
			"style/broken.scss":             "a {\n  color: $undefined;\n}\n",
			"style/fine.scss":               "a { color: red; }\n",
			"a.md":                          "{{ missing }}\n",
			"b.md":                          "fine\n",
		})
		result := runBuild(t, projectDir)
		want := []string{
			"style/broken.scss:2:10: Undefined variable.",
			"a.md:1: variable \"missing\" is not defined; the available variables are author, collections, data, date, excludeFromFleet, fleet, layout, page, permalink, site, sitemap, tags, title",
		}
		if got := result.errorTexts(); !reflect.DeepEqual(got, want) {
			t.Errorf("errors:\n got %q\nwant %q", got, want)
		}
		checkNotPublished(t, projectDir, result)
		wantTemp := map[string]string{"b.html": "<main><p>fine</p>\n</main>\n", "style/fine.css": "a{color:red}\n"}
		if got := readTree(t, filepath.Join(projectDir, "_site.temp")); !reflect.DeepEqual(got, wantTemp) {
			t.Errorf("temp output = %q, want %q", got, wantTemp)
		}
	})
	t.Run("a warning does not stop the build", func(t *testing.T) {
		projectDir := writeProject(t, map[string]string{
			"gondola.yaml":    sassConfig,
			"style/main.scss": "@warn \"check the colors\";\na { color: red; }\n",
		})
		result := runBuild(t, projectDir)
		if result.failure != nil {
			t.Fatalf("build failed: %q", result.errorTexts())
		}
		if want := "warning: check the colors\n"; result.stderr != want {
			t.Errorf("standard error = %q, want %q", result.stderr, want)
		}
		if !strings.Contains(result.stdout, "Built 0 files and Copied 0") {
			t.Errorf("log:\n%s", result.stdout)
		}
	})
	t.Run("sass: false ignores Sass files", func(t *testing.T) {
		projectDir := writeProject(t, map[string]string{"style/broken.scss": "a {"})
		if result := runBuild(t, projectDir); result.failure != nil {
			t.Fatalf("build failed: %q", result.errorTexts())
		}
		if got := readTree(t, filepath.Join(projectDir, "_site")); len(got) != 0 {
			t.Errorf("output = %q", got)
		}
	})
}
