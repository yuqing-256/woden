//go:build windows

package clean

import (
	"io/fs"
	"syscall"
)

// isReparsePoint reports whether info describes a Windows reparse point.
//
// Junctions (mount points) are directories that are not reported as symbolic
// links by os.Lstat, so the file attributes are the only reliable way to tell
// them apart from ordinary directories. Everything carrying the reparse-point
// attribute is treated as a link: it is deleted itself and never traversed.
func isReparsePoint(info fs.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false
	}
	return data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
