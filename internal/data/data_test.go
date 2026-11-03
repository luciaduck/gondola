package data

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/luciaduck/gondola/internal/config"
)

// defaultInput is input.data left out of gondola.yaml.
var defaultInput = config.Input{
	DataDirs:         config.DirList{Dirs: []string{"_data"}, FromDefault: true},
	DataDirPositions: []config.Position{{}},
}

// writtenInput is input.data written as a list of dirs, each at line 6 of
// gondola.yaml, at the columns of "  data: [a, b]".
func writtenInput(dirs ...string) config.Input {
	input := config.Input{DataDirs: config.DirList{Dirs: dirs}}
	col := len("  data: [") + 1
	for _, dir := range dirs {
		input.DataDirPositions = append(input.DataDirPositions, config.Position{Line: 6, Col: col})
		col += len(dir) + len(", ")
	}
	return input
}

// makeTree creates a project directory holding files, a map from a path
// written with "/" to its contents. A path ending in "/" is an empty
// directory.
func makeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	projectDir := t.TempDir()
	for slashPath, contents := range files {
		full := filepath.Join(projectDir, filepath.FromSlash(slashPath))
		if strings.HasSuffix(slashPath, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectDir
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		input config.Input
		want  map[string]any
	}{
		{"file names become template names",
			map[string]string{
				"_data/site-info.json": `{"name": "Lindos"}`,
				"_data/nav_links.yml":  "- home\n",
				"_data/Motto.yaml":     "Sail on\n",
				"_data/URL-list.json":  `[]`,
			},
			defaultInput,
			map[string]any{
				"siteInfo": map[string]any{"name": "Lindos"},
				"navLinks": []any{"home"},
				"motto":    "Sail on",
				"urlList":  []any{},
			}},
		{"subdirectories nest",
			map[string]string{
				"_data/blog/authors.yaml":      "ada: Ada\n",
				"_data/blog/tags/list.json":    `["news"]`,
				"_data/blog-roll/friends.json": `["Bo"]`,
			},
			defaultInput,
			map[string]any{
				"blog":     map[string]any{"authors": map[string]any{"ada": "Ada"}, "tags": map[string]any{"list": []any{"news"}}},
				"blogRoll": map[string]any{"friends": []any{"Bo"}},
			}},
		{"an empty subdirectory is an empty map",
			map[string]string{"_data/later/": ""},
			defaultInput,
			map[string]any{"later": map[string]any{}}},
		{"empty files are placeholders",
			map[string]string{"_data/a.yaml": "", "_data/b.json": "", "_data/c.yml": "# soon\n"},
			defaultInput,
			map[string]any{"a": "", "b": "", "c": ""}},
		{"extensions in capitals",
			map[string]string{"_data/a.JSON": `1`, "_data/b.YML": "2\n", "_data/c.Yaml": "3\n"},
			defaultInput,
			map[string]any{"a": 1, "b": 2, "c": 3}},
		{"other files are ignored",
			map[string]string{
				"_data/site.json":     `{}`,
				"_data/README.md":     "# Data\n",
				"_data/base.jet":      "{{ copy|raw }}",
				"_data/page.html":     "<p>",
				"_data/site.toml":     "a = 1\n",
				"_data/notes":         "x",
				"_data/site.json.bak": "{",
			},
			defaultInput,
			map[string]any{"site": map[string]any{}}},
		{"names beginning with a dot or an underscore are skipped",
			map[string]string{
				"_data/site.json":         `{}`,
				"_data/.hidden.json":      "{",
				"_data/_draft.yaml":       "a: [",
				"_data/.git/config.json":  "{",
				"_data/_old/site.json":    "{",
				"_data/blog/_wip.json":    "{",
				"_data/blog/.DS_Store":    "x",
				"_data/blog/authors.json": `[]`,
			},
			defaultInput,
			map[string]any{"site": map[string]any{}, "blog": map[string]any{"authors": []any{}}}},
		{"a missing default directory is skipped",
			map[string]string{"other/site.json": `{}`},
			defaultInput,
			map[string]any{}},
		{"no data directories",
			map[string]string{"_data/site.json": `{}`},
			config.Input{DataDirs: config.DirList{Dirs: []string{}}, DataDirPositions: []config.Position{}},
			map[string]any{}},
		{"a directory written by hand",
			map[string]string{"shared/data/site.json": `{"a": 1}`, "_data/ignored.json": `{}`},
			writtenInput("shared/data"),
			map[string]any{"site": map[string]any{"a": 1}}},
		{"two data directories",
			map[string]string{"one/site.json": `{"a": 1}`, "two/nav.yaml": "- home\n"},
			writtenInput("one", "two"),
			map[string]any{"site": map[string]any{"a": 1}, "nav": []any{"home"}}},
		{"the same subdirectory in two data directories is merged",
			map[string]string{
				"one/blog/authors.json": `["Ada"]`,
				"two/blog/tags.yaml":    "- news\n",
				"two/blog/deep/x.json":  `1`,
				"one/blog/deep/y.json":  `2`,
				"two/Blog-roll/a.json":  `3`,
				"one/blog_roll/b.json":  `4`,
			},
			writtenInput("one", "two"),
			map[string]any{
				"blog": map[string]any{
					"authors": []any{"Ada"},
					"tags":    []any{"news"},
					"deep":    map[string]any{"x": 1, "y": 2},
				},
				"blogRoll": map[string]any{"a": 3, "b": 4},
			}},
		{"a data directory inside the content directory",
			map[string]string{"site/_data/site.json": `{}`},
			writtenInput("site/_data"),
			map[string]any{"site": map[string]any{}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := makeTree(t, test.files)
			got, err := Load(projectDir, test.input)
			if err != nil {
				t.Fatalf("Load returned error: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("got  %#v\nwant %#v", got, test.want)
			}
		})
	}
}

func TestLoadWithoutPositions(t *testing.T) {
	// A Config built by hand may have no positions; the error then has none.
	projectDir := makeTree(t, nil)
	_, err := Load(projectDir, config.Input{DataDirs: config.DirList{Dirs: []string{"shared"}}})
	if want := `gondola.yaml: input.data "shared" does not exist`; err == nil || err.Error() != want {
		t.Errorf("error = %v, want %s", err, want)
	}
}

type loadErrorTest struct {
	name  string
	files map[string]string
	input config.Input
	want  string
}

func runLoadErrorTests(t *testing.T, tests []loadErrorTest, prepare func(t *testing.T, projectDir string)) {
	t.Helper()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := makeTree(t, test.files)
			if prepare != nil {
				prepare(t, projectDir)
			}
			_, err := Load(projectDir, test.input)
			if err == nil {
				t.Fatal("Load returned no error")
			}
			if got := err.Error(); got != test.want {
				t.Errorf("error = %s\n         want %s", got, test.want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	runLoadErrorTests(t, []loadErrorTest{
		// The data directories themselves.
		{"a written directory is missing", nil, writtenInput("shared"),
			`gondola.yaml:6:10: input.data "shared" does not exist`},
		{"the second written directory is missing", map[string]string{"one/": ""}, writtenInput("one", "two"),
			`gondola.yaml:6:15: input.data "two" does not exist`},
		{"a written directory is a file", map[string]string{"shared": "x"}, writtenInput("shared"),
			`gondola.yaml:6:10: input.data "shared" is a file, not a directory`},
		{"the default directory is a file", map[string]string{"_data": "x"}, defaultInput,
			`gondola.yaml: input.data "_data" (the default) is a file, not a directory`},

		// Two sources for one key.
		{"two files with one name", map[string]string{"_data/a.json": `1`, "_data/a.yaml": "2\n"}, defaultInput,
			`data files "_data/a.json" and "_data/a.yaml" both become data.a; please rename one`},
		{"two names that become one", map[string]string{"_data/site-info.json": `1`, "_data/site_info.yaml": "2\n"}, defaultInput,
			`data files "_data/site-info.json" and "_data/site_info.yaml" both become data.siteInfo; please rename one`},
		{"one name in two data directories", map[string]string{"one/site.json": `1`, "two/site.yml": "2\n"}, writtenInput("one", "two"),
			`data files "one/site.json" and "two/site.yml" both become data.site; please rename one`},
		{"a directory and a file", map[string]string{"_data/blog.json": `1`, "_data/blog/authors.json": `2`}, defaultInput,
			`data directory "_data/blog" and data file "_data/blog.json" both become data.blog; please rename one`},
		{"a file and a directory in two data directories", map[string]string{"one/blog.json": `1`, "two/blog/authors.json": `2`}, writtenInput("one", "two"),
			`data file "one/blog.json" and data directory "two/blog" both become data.blog; please rename one`},
		{"an empty directory and a file", map[string]string{"one/blog/": "", "two/blog.yaml": ""}, writtenInput("one", "two"),
			`data directory "one/blog" and data file "two/blog.yaml" both become data.blog; please rename one`},
		{"two files in merged subdirectories", map[string]string{"one/blog/a.json": `1`, "two/blog/a.yml": "2\n"}, writtenInput("one", "two"),
			`data files "one/blog/a.json" and "two/blog/a.yml" both become data.blog.a; please rename one`},

		// Names that cannot be template names (SPEC §4).
		{"file name starts with a digit", map[string]string{"_data/2-col.json": `1`}, defaultInput,
			`_data/2-col.json: data file name "2-col" becomes "2Col" in templates, which is not a valid name (names must start with a letter); please rename it`},
		{"file name with a dot", map[string]string{"_data/site.info.json": `1`}, defaultInput,
			`_data/site.info.json: data file name "site.info" becomes "site.info" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"file name with an empty word", map[string]string{"_data/site--info.yaml": ""}, defaultInput,
			`_data/site--info.yaml: data file name "site--info" has an empty word; please remove the extra "-" or "_"`},
		{"file name is a Jet keyword", map[string]string{"_data/range.json": `1`}, defaultInput,
			`_data/range.json: data file name "range" is a Jet keyword and cannot be a template variable; please rename it`},
		{"file name with a space", map[string]string{"_data/my site.json": `1`}, defaultInput,
			`_data/my site.json: data file name "my site" becomes "my site" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"directory name with a dot", map[string]string{"_data/v1.2/x.json": `1`}, defaultInput,
			`_data/v1.2: data directory name "v1.2" becomes "v1.2" in templates, which is not a valid name (names may contain only letters and digits); please rename it`},
		{"directory name is a Jet keyword", map[string]string{"_data/import/x.json": `1`}, defaultInput,
			`_data/import: data directory name "import" is a Jet keyword and cannot be a template variable; please rename it`},

		// Files that do not parse.
		{"malformed JSON", map[string]string{"_data/site.json": "{\n  \"a\": 1,\n}\n"}, defaultInput,
			`_data/site.json:3:1: invalid JSON: invalid character '}' looking for beginning of object key string`},
		{"malformed YAML in a subdirectory", map[string]string{"_data/blog/authors.yaml": "a: 1\na: 2\n"}, defaultInput,
			`_data/blog/authors.yaml:2:1: key "a" appears twice in the data file (first at line 1); please keep only one`},
		{"malformed file in the second data directory", map[string]string{"one/a.json": `1`, "two/b.yml": "[\n"}, writtenInput("one", "two"),
			`two/b.yml:1:1: invalid YAML: did not find expected node content`},
		{"a UTF-16 file", map[string]string{"_data/site.json": "\xFF\xFE{\x00"}, defaultInput,
			`_data/site.json: the file is saved as UTF-16; please save it as UTF-8`},
	}, nil)
}

func TestLoadNameInLetterCase(t *testing.T) {
	if fileSystemIgnoresCase(t) {
		t.Skip("this file system cannot hold two names that differ only in letter case")
	}
	runLoadErrorTests(t, []loadErrorTest{
		// os.ReadDir sorts by bytes, so capitals come first.
		{"two files", map[string]string{"_data/Site.json": `1`, "_data/site.json": `2`}, defaultInput,
			`data files "_data/Site.json" and "_data/site.json" both become data.site; please rename one`},
	}, nil)
}

func fileSystemIgnoresCase(t *testing.T) bool {
	t.Helper()
	dir := makeTree(t, map[string]string{"probe": ""})
	_, err := os.Stat(filepath.Join(dir, "PROBE"))
	return err == nil
}

func makeLink(t *testing.T, projectDir, target, linkPath string) {
	t.Helper()
	full := filepath.Join(projectDir, filepath.FromSlash(linkPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash(target), full); err != nil {
		t.Skipf("cannot create a symbolic link: %v", err)
	}
}

func TestLoadSymbolicLinks(t *testing.T) {
	const refused = " is a symbolic link; Gondola does not follow links, so please replace it with the file or directory itself"
	tests := []struct {
		name     string
		target   string
		linkPath string
		input    config.Input
		want     string // "" when the link is skipped
	}{
		{"link to a data file", "../elsewhere/site.json", "_data/site.json", defaultInput, "_data/site.json" + refused},
		{"link named as a data file, to another file", "../elsewhere/notes.txt", "_data/notes.yaml", defaultInput, "_data/notes.yaml" + refused},
		{"broken link named as a data file", "../nothing.json", "_data/gone.json", defaultInput, "_data/gone.json" + refused},
		{"link to a directory", "../elsewhere", "_data/more", defaultInput, "_data/more" + refused},
		{"the data directory is a link", "elsewhere", "_data", defaultInput, "_data" + refused},
		{"a written data directory is a link", "elsewhere", "shared", writtenInput("shared"), "shared" + refused},
		{"link with another extension is skipped", "../elsewhere/notes.txt", "_data/notes.txt", defaultInput, ""},
		{"link under an underscore is skipped", "../elsewhere", "_data/_more", defaultInput, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"elsewhere/site.json": `{}`,
				"elsewhere/notes.txt": "x",
			}
			// A link that is itself the data directory needs the place free.
			if strings.HasPrefix(test.linkPath, "_data/") {
				files["_data/real.json"] = `1`
			}
			projectDir := makeTree(t, files)
			makeLink(t, projectDir, test.target, test.linkPath)
			got, err := Load(projectDir, test.input)
			if test.want == "" {
				if err != nil {
					t.Fatalf("Load returned error: %v", err)
				}
				if want := map[string]any{"real": 1}; !reflect.DeepEqual(got, want) {
					t.Errorf("got %#v, want %#v", got, want)
				}
				return
			}
			if err == nil || err.Error() != test.want {
				t.Errorf("error = %v\n         want %s", err, test.want)
			}
		})
	}
}

func skipUnlessNamesMayBeAnyBytes(t *testing.T) {
	t.Helper()
	const notUTF8Name = "caf\xe9"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, notUTF8Name), nil, 0o644); err != nil {
		t.Skipf("the file system refuses names that are not UTF-8: %v", err)
	}
	// Windows stores names as UTF-16, and Go writes U+FFFD there for each
	// byte that is not UTF-8 instead of failing.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != notUTF8Name {
		t.Skip("the file system does not keep names that are not UTF-8")
	}
}

func TestLoadNamesNotUTF8(t *testing.T) {
	skipUnlessNamesMayBeAnyBytes(t)
	runLoadErrorTests(t, []loadErrorTest{
		{"file name", map[string]string{"_data/caf\xe9.json": `1`}, defaultInput,
			`_data/caf\xe9.json: the file name is not valid UTF-8; please rename it`},
		{"directory name", map[string]string{"_data/caf\xe9/menu.yaml": "a: 1\n"}, defaultInput,
			`_data/caf\xe9: the name of the directory "caf\xe9" is not valid UTF-8; please rename it`},
	}, nil)

	// Files that would be ignored anyway are left alone, as in package scan.
	projectDir := makeTree(t, map[string]string{
		"_data/caf\xe9.txt":        "x",
		"_data/_caf\xe9.json":      "{",
		"_data/.caf\xe9/site.json": "{",
	})
	got, err := Load(projectDir, defaultInput)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %#v, want an empty map", got)
	}
}
