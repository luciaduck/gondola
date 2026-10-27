package config

import (
	"github.com/luciaduck/gondola/internal/diag"
	"github.com/luciaduck/gondola/internal/yamlnode"
)

// yamlError turns an error from yaml.v3 into a diag error naming
// gondola.yaml, at the line it names. yaml.v3 gives no column, so the column
// is 1, as for front matter.
func yamlError(err error) error {
	return diag.WithFile(yamlnode.SyntaxError(err), FileName)
}
