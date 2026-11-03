//go:build !windows

package build

// isInUse reports whether err is the system refusing to delete or rename a
// directory that is in use; only Windows refuses.
func isInUse(err error) bool {
	return false
}
