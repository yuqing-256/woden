//go:build !windows

package clean

import "io/fs"

// isReparsePoint reports whether info describes a link that os.Lstat does not
// report as a symbolic link. On non-Windows systems there are none: symbolic
// links are covered by fs.FileMode's ModeSymlink bit.
func isReparsePoint(info fs.FileInfo) bool {
	return false
}
