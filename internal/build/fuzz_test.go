package build

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// fuzzConfig is the gondola.yaml of the fuzzed projects: Sass off (the
// fuzzer must not depend on Dart Sass), fenced blocks on, and a default
// layout so that pages go through both template rounds.
const fuzzConfig = minimalConfig + "  fenced-blocks: true\n" +
	"default:\n  layout: page\n"

// fuzzSeeds are pages and layouts that reach every stage of the build, and
// front matter values for a second page (see fuzzProjectFiles).
var fuzzSeeds = []struct {
	page     string
	layout   string
	markdown bool
	other    otherPage
}{
	{"", "", false, otherPage{}},
	{"<p>{{ title }}</p>\n", "<main>{{ copy|raw }}</main>\n", false, otherPage{}},
	{"---\ntitle: Home\n---\n# {{ title }}\n", "<main>{{ copy|raw }}</main>\n", true, otherPage{}},
	{"---\npermalink: /\n---\n:::\\note\ntext\n:::\n", "{{ copy|raw }}", true, otherPage{}},
	{"---\nlayout: nil\n---\n{{ not Jet }}\n", "", true, otherPage{}},
	{"---\nlayout: nil\n---\n<p>{{ raw }}</p>\n", "", false, otherPage{}},
	{"---\ntitle: Post\ndate: 2024-05-01\ntags: [news]\npermalink: date\n---\ntext\n", "{{ copy|raw }}{{ len(collections[\"news\"]) }}", true, otherPage{}},
	{"{{ range _, entry := fleet }}{{ entry.url }} {{ end }}", "{{ copy|raw }}", true, otherPage{}},
	{"{{ include \"snippet.md\" }}", "{{ copy|raw }}", true, otherPage{}},
	{"{{ data.team[0].name }}", "---\nlayout: page\n---\n{{ copy|raw }}", false, otherPage{}},
	{"---\ntitle: [\n---\n", "{{ copy|raw }}", true, otherPage{}},
	{"{{ undefinedVariable }}", "{{ copy|raw }}", false, otherPage{}},
	{"text", "{{ if }}", true, otherPage{}},
	{"---\npermalink: /list.html\n---\n", "{{ copy|raw }}", false, otherPage{}},
	{"---\npermalink: /a\n---\n", "{{ copy|raw }}", false, otherPage{}},
	{"\xEF\xBB\xBF---\ntitle: BOM\n---\r\nCRLF\r\n", "{{ copy|raw }}", true, otherPage{}},
	{"text", "{{ copy|raw }}", true, otherPage{permalink: "/about/", title: "About"}},
	{"text", "{{ copy|raw }}", true, otherPage{permalink: "date", title: "Post", date: "2024-05-01"}},
	{"{{ range _, entry := fleet }}{{ entry.title }}{{ end }}", "{{ copy|raw }}", true, otherPage{permalink: "slug", title: "Città di mare"}},
	{"text", "{{ copy|raw }}", false, otherPage{permalink: "/list.html"}},
	{"text", "{{ copy|raw }}", false, otherPage{permalink: "/index.html"}},
	{"text", "{{ copy|raw }}", false, otherPage{permalink: "/a/../b"}},
	{"text", "{{ copy|raw }}", false, otherPage{permalink: "slug", title: "!!!"}},
	{"text", "{{ copy|raw }}", false, otherPage{permalink: "path", date: "2024-13-01"}},
}

// otherPage holds the front matter values of the second fuzzed page. They
// are separate arguments, written into valid YAML by fuzzProjectFiles,
// because random edits of a whole page rarely keep its front matter valid:
// a first canary on raw text never turned "permalink: /" into a different
// valid permalink (M9 log).
type otherPage struct {
	permalink string
	title     string
	date      string
}

// frontMatter returns a front matter block that sets values, written with
// yaml.v3 so that any text is a valid YAML string.
func frontMatter(t *testing.T, values map[string]string) string {
	t.Helper()
	block, err := yaml.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	return "---\n" + string(block) + "---\n"
}

// fuzzProjectFiles returns the files of a project whose page "index" holds
// page and whose default layout holds layout. The page "other.md" has the
// front matter of other, and "list.html" lists the fleet, so that every
// page's front matter reaches the others.
func fuzzProjectFiles(t *testing.T, page, layout string, markdown bool, other otherPage) map[string]string {
	pageName := "index.html"
	if markdown {
		pageName = "index.md"
	}
	return map[string]string{
		"gondola.yaml":                    fuzzConfig,
		"alias.squero":                    ":::\\note . aside +note\n",
		"_data/team.json":                 `[{"name": "Ada"}]`,
		"_components/includes/page.jet":   layout,
		"_components/includes/snippet.md": ":::\\note\n*fragment*\n:::\n",
		"list.html":                       "{{ range _, entry := fleet }}{{ entry.url }} {{ end }}",
		pageName:                          page,
		"other.md": frontMatter(t, map[string]string{
			"permalink": other.permalink, "title": other.title, "date": other.date,
		}) + "{{ title }}\n",
		"_components/includes/unused/x.html": "{{",
	}
}

// FuzzBuild builds a small project around a fuzzed page and layout, twice,
// and checks what every build must do whatever the input: it never panics;
// a success publishes exactly the pages it printed and removes the temp
// output; a failure publishes nothing, keeps the temp output, and reports
// between 1 and maxCollectedErrors errors that do not leak paths on disk;
// and the two builds are identical (SPEC §4, deterministic output).
func FuzzBuild(f *testing.F) {
	for _, seed := range fuzzSeeds {
		f.Add(seed.page, seed.layout, seed.markdown, seed.other.permalink, seed.other.title, seed.other.date)
	}
	f.Fuzz(func(t *testing.T, page, layout string, markdown bool, permalink, title, date string) {
		files := fuzzProjectFiles(t, page, layout, markdown, otherPage{permalink: permalink, title: title, date: date})
		first := buildFuzzProject(t, files)
		second := buildFuzzProject(t, files)
		checkFuzzBuild(t, first)
		if !reflect.DeepEqual(first.errors, second.errors) || first.stdout != second.stdout ||
			first.stderr != second.stderr || !reflect.DeepEqual(first.files, second.files) {
			t.Errorf("two builds of the same project differ:\n%+v\n%+v", first, second)
		}
	})
}

// FuzzCanaryBuild is written to fail: it claims that the page index.md,
// whose permalink is fuzzed, is always written as index.html when the build
// succeeds, which any custom permalink such as "/a" makes false. It shows
// that the fuzzer reaches routing through the whole build. It is
// skipped unless GONDOLA_FUZZ_CANARY is set, so that go test stays green
// with the failing input saved under testdata/fuzz:
//
//	GONDOLA_FUZZ_CANARY=1 go test ./internal/build -run '^$' -fuzz FuzzCanaryBuild -fuzztime 1m
func FuzzCanaryBuild(f *testing.F) {
	if os.Getenv("GONDOLA_FUZZ_CANARY") == "" {
		f.Skip("the canary fails on purpose; set GONDOLA_FUZZ_CANARY to run it")
	}
	// Only seeds that keep the claim true: the fuzzer has to find the
	// counterexample itself.
	f.Add("/")
	f.Add("")
	f.Add("path")
	f.Fuzz(func(t *testing.T, permalink string) {
		page := frontMatter(t, map[string]string{"permalink": permalink}) + "text\n"
		result := buildFuzzProject(t, fuzzProjectFiles(t, page, "{{ copy|raw }}", true, otherPage{}))
		if len(result.errors) > 0 {
			return
		}
		for _, line := range shapingLines(result.stdout) {
			if strings.Contains(line, " from ./index.md") && !strings.Contains(line, "./_site.temp/index.html ") {
				t.Errorf("the page was not written as index.html: %s", line)
			}
		}
	})
}

// fuzzBuildResult is what one build of a fuzzed project did, with the paths
// that depend on the temporary directory taken out, so that two builds can
// be compared.
type fuzzBuildResult struct {
	projectDir   string
	errors       []string
	limitReached bool
	stdout       string
	stderr       string
	// files are the files of the output after a success, or of the temp
	// output after a failure.
	files            map[string]string
	outputExists     bool
	tempOutputExists bool
}

func buildFuzzProject(t *testing.T, files map[string]string) fuzzBuildResult {
	t.Helper()
	projectDir := writeProject(t, files)
	result := runBuild(t, projectDir)
	fuzzResult := fuzzBuildResult{
		projectDir:       projectDir,
		errors:           result.errorTexts(),
		stdout:           result.stdout,
		stderr:           result.stderr,
		outputExists:     dirExists(t, filepath.Join(projectDir, "_site")),
		tempOutputExists: dirExists(t, filepath.Join(projectDir, "_site.temp")),
	}
	if result.failure != nil {
		fuzzResult.limitReached = result.failure.LimitReached
		fuzzResult.files = readTree(t, filepath.Join(projectDir, "_site.temp"))
	} else {
		fuzzResult.files = readTree(t, filepath.Join(projectDir, "_site"))
	}
	return fuzzResult
}

func checkFuzzBuild(t *testing.T, result fuzzBuildResult) {
	t.Helper()
	if len(result.errors) == 0 {
		checkFuzzSuccess(t, result)
		return
	}
	if len(result.errors) > maxCollectedErrors {
		t.Errorf("%d errors, more than the limit of %d", len(result.errors), maxCollectedErrors)
	}
	if result.limitReached != (len(result.errors) == maxCollectedErrors) {
		t.Errorf("LimitReached is %v with %d errors", result.limitReached, len(result.errors))
	}
	if result.outputExists || !result.tempOutputExists {
		t.Errorf("after a failure: output exists %v, temp output exists %v", result.outputExists, result.tempOutputExists)
	}
	if strings.Contains(result.stdout, "Shipping") || strings.Contains(result.stdout, "Built") {
		t.Errorf("a failed build printed the lines of a published one:\n%s", result.stdout)
	}
	for _, text := range result.errors {
		if text == "" {
			t.Error("an error has no message")
		}
		// Messages name files relative to the project; the os package
		// would put the whole path on disk into them.
		if strings.Contains(text, result.projectDir) {
			t.Errorf("an error names a path on disk: %s", text)
		}
	}
}

func checkFuzzSuccess(t *testing.T, result fuzzBuildResult) {
	t.Helper()
	if !result.outputExists || result.tempOutputExists {
		t.Errorf("after a success: output exists %v, temp output exists %v", result.outputExists, result.tempOutputExists)
	}
	if result.stderr != "" {
		t.Errorf("standard error is not empty: %q", result.stderr)
	}
	// The output holds exactly the files that the Shaping lines name.
	printed := map[string]bool{}
	for _, line := range shapingLines(result.stdout) {
		outputFilePath, _, found := strings.Cut(strings.TrimPrefix(line, "\\gondola/ Shaping ./"), " from ")
		if !found {
			t.Fatalf("malformed line: %s", line)
		}
		relative := strings.TrimPrefix(outputFilePath, "_site.temp/")
		if printed[relative] {
			t.Errorf("%s was printed twice", relative)
		}
		printed[relative] = true
	}
	for name := range result.files {
		if !printed[name] {
			t.Errorf("%s is in the output without a Shaping line", name)
		}
	}
	for name := range printed {
		if _, ok := result.files[name]; !ok {
			t.Errorf("%s has a Shaping line but is not in the output", name)
		}
	}
	lines := strings.Split(strings.TrimSuffix(result.stdout, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "\\gondola/ Built ") {
		t.Errorf("the last line is %q", last)
	}
}
