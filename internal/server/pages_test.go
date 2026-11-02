package server

import (
	"bytes"
	"encoding/base64"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// update rewrites the golden files of the waiting pages:
// go test ./internal/server -run TestWaitingPages -update. Read the diff of
// the golden files before keeping it.
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// dataURI matches the inlined photo of a page. The golden files hold a
// placeholder instead of hundreds of kilobytes of base64.
var dataURI = regexp.MustCompile(`data:image/jpeg;base64,[A-Za-z0-9+/=]+`)

const dataURIPlaceholder = "data:image/jpeg;base64,…"

func TestWaitingPages(t *testing.T) {
	pages, err := renderWaitingPages("v0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		rendered  []byte
		photoPath string
	}{
		{"building", pages.building, buildingPhoto.FilePath},
		{"failed", pages.failed, failedPhoto.FilePath},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			uris := dataURI.FindAll(test.rendered, -1)
			if len(uris) != 1 {
				t.Fatalf("found %d data: URIs, want 1", len(uris))
			}
			decoded, err := base64.StdEncoding.DecodeString(string(bytes.TrimPrefix(uris[0], []byte("data:image/jpeg;base64,"))))
			if err != nil {
				t.Fatal(err)
			}
			photoFile, err := pageFiles.ReadFile(test.photoPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, photoFile) {
				t.Errorf("the inlined photo is not %s", test.photoPath)
			}
			got := dataURI.ReplaceAll(test.rendered, []byte(dataURIPlaceholder))
			goldenPath := filepath.Join("testdata", "golden", test.name+".html")
			if *update {
				if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from %s:\n%s", test.name, goldenPath, got)
			}
		})
	}
}

func TestWaitingPagesEscapeTheVersion(t *testing.T) {
	pages, err := renderWaitingPages("v1<script>")
	if err != nil {
		t.Fatal(err)
	}
	for _, page := range [][]byte{pages.building, pages.failed} {
		if !bytes.Contains(page, []byte("<title>Gondola SSG v1&lt;script&gt;</title>")) {
			t.Errorf("the version is not escaped in the title")
		}
	}
}
