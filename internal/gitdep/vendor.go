package gitdep

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"woden/internal/errs"
	"woden/internal/fsx"
	"woden/internal/project"
)

// vendorPath returns the absolute worktree path of dependency name.
func (d *Deps) vendorPath(name string) string {
	return filepath.Join(d.Project.VendorDir(), name)
}

// vendorRel returns the project relative path of the worktree of name, for
// messages.
func vendorRel(name string) string {
	return filepath.ToSlash(filepath.Join(project.VendorDir, name))
}

// stagingDir reserves a fresh staging path below vendor/ and returns it. The
// directory itself is created by git clone, so it must not exist yet.
func (d *Deps) stagingDir() (string, error) {
	vendor := d.Project.VendorDir()
	if err := os.MkdirAll(vendor, 0o755); err != nil {
		return "", errs.WrapUser(err, "cannot create %s", vendor)
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return filepath.Join(vendor, stagingPrefix+hex.EncodeToString(buf[:])), nil
}

// removeStaging removes a staging directory. It is called on failure paths and
// must not report errors of its own.
func removeStaging(dir string) {
	_ = os.RemoveAll(dir)
}

// cleanStaging removes staging directories that an interrupted clone left
// behind. Only names created by stagingDir are removed.
func (d *Deps) cleanStaging() error {
	entries, err := os.ReadDir(d.Project.VendorDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errs.WrapUser(err, "cannot read %s", d.Project.VendorDir())
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), stagingPrefix) {
			continue
		}
		path := filepath.Join(d.Project.VendorDir(), entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return errs.WrapUser(err, "cannot remove the interrupted clone %s", path)
		}
	}
	return nil
}

// vendorEntries returns the names of the entries below vendor/ that are not
// staging directories, sorted as reported by ReadDir.
func (d *Deps) vendorEntries() ([]string, error) {
	entries, err := os.ReadDir(d.Project.VendorDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, errs.WrapUser(err, "cannot read %s", d.Project.VendorDir())
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingPrefix) {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// removeEmptyVendorDir removes vendor/ when it is empty. Removing the last
// dependency must not leave a collection mapped to a nonexistent directory.
func (d *Deps) removeEmptyVendorDir() error {
	vendor := d.Project.VendorDir()
	entries, err := os.ReadDir(vendor)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return errs.WrapUser(err, "cannot read %s", vendor)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(vendor); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return errs.WrapUser(err, "cannot remove the empty directory %s", vendor)
	}
	return nil
}

// exists reports whether path exists, mapping errors to user errors where the
// caller needs the distinction.
func exists(path string) (bool, error) {
	ok, err := fsx.Exists(path)
	if err != nil {
		return false, errs.WrapUser(err, "cannot inspect %s", path)
	}
	return ok, nil
}
