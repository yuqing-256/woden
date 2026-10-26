// Package fsx contains the filesystem primitives used by Woden.
//
// All writes performed by Woden go through AtomicWrite or WriteNew so that an
// interrupted command never leaves a truncated file behind.
package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"woden/internal/errs"
)

// AbsClean returns the absolute, cleaned form of path.
func AbsClean(path string) (string, error) {
	if path == "" {
		return "", errs.Userf("path must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", errs.WrapUser(err, "cannot resolve path %q", path)
	}
	return filepath.Clean(abs), nil
}

// IsWithin reports whether target is root itself or a descendant of root.
// Both paths must be absolute and cleaned; the comparison is lexical, which is
// exactly what Woden wants: it rejects ".." escapes, not symbolic links.
func IsWithin(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if samePath(root, target) {
		return true
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Within resolves path relative to root and checks that the result does not
// escape root. An absolute path is rejected.
func Within(root, path string) (string, error) {
	// Reject rooted paths with either separator so a POSIX-style absolute path
	// is refused on Windows as well.
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) {
		return "", errs.Userf("path %q must be relative to the project root", path)
	}
	target := filepath.Join(root, path)
	if !IsWithin(root, target) {
		return "", errs.Userf("path %q escapes the project root", path)
	}
	return filepath.Clean(target), nil
}

// samePath compares two cleaned absolute paths, ignoring case on Windows.
func samePath(a, b string) bool {
	if osPathEqual(a, b) {
		return true
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// Exists reports whether path exists. Symbolic links are reported as existing
// even when their target is gone (Lstat semantics).
func Exists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// TempFile is a temporary file created next to its final destination.
type TempFile struct {
	*os.File
	path string
	dst  string
}

// Path returns the path of the temporary file.
func (t *TempFile) Path() string { return t.path }

// Commit flushes the temporary file, closes it, and renames it over its
// destination. The permissions of an existing destination are preserved.
func (t *TempFile) Commit() error {
	if t == nil || t.File == nil || t.dst == "" {
		return errors.New("fsx: temporary file has no destination")
	}
	if err := t.Sync(); err != nil {
		t.CloseAndRemove()
		return errs.WrapUser(err, "cannot flush %s", t.dst)
	}
	final := t.path
	if err := t.File.Close(); err != nil {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot write %s", t.dst)
	}
	perm := fs.FileMode(0o644)
	if info, err := os.Stat(t.dst); err == nil {
		perm = info.Mode().Perm()
	}
	if err := os.Chmod(final, perm); err != nil && os.PathSeparator != '\\' {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot set permissions on %s", t.dst)
	}
	if err := os.Rename(final, t.dst); err != nil {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot replace %s", t.dst)
	}
	t.File = nil // CloseAndRemove becomes a no-op once committed.
	return nil
}

// CloseAndRemove closes and deletes the temporary file. It is a no-op after
// Commit.
func (t *TempFile) CloseAndRemove() {
	if t == nil || t.File == nil {
		return
	}
	name := t.Name()
	if err := t.File.Close(); err != nil && !errors.Is(err, fs.ErrClosed) {
		_ = os.Remove(name)
		return
	}
	_ = os.Remove(name)
}

// NewTempFile creates a temporary file in the directory of path. The caller
// must call Commit to publish it or CloseAndRemove to discard it.
func NewTempFile(path string) (*TempFile, error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".woden-tmp-*")
	if err != nil {
		return nil, errs.WrapUser(err, "cannot create a temporary file in %s", dir)
	}
	return &TempFile{File: f, path: f.Name(), dst: path}, nil
}

// AtomicWrite writes data to path atomically: the bytes are written to a
// temporary file in the same directory, fsynced, then renamed over path. The
// permissions of an existing file are preserved; otherwise perm is used.
func AtomicWrite(path string, data []byte, perm fs.FileMode) error {
	if perm == 0 {
		perm = 0o644
	}
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp, err := NewTempFile(path)
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.CloseAndRemove()
		return errs.WrapUser(err, "cannot write %s", path)
	}
	if err := tmp.Sync(); err != nil {
		tmp.CloseAndRemove()
		return errs.WrapUser(err, "cannot flush %s", path)
	}
	final := tmp.Path()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot write %s", path)
	}
	if err := os.Chmod(final, perm); err != nil && os.PathSeparator != '\\' {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot set permissions on %s", path)
	}
	if err := os.Rename(final, path); err != nil {
		_ = os.Remove(final)
		return errs.WrapUser(err, "cannot replace %s", path)
	}
	return nil
}

// WriteNew creates path with data. It fails if path already exists, so Woden
// can never overwrite a file it did not create itself.
func WriteNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return errs.Userf("%s already exists", path)
		}
		return errs.WrapUser(err, "cannot create %s", path)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return errs.WrapUser(err, "cannot write %s", path)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return errs.WrapUser(err, "cannot write %s", path)
	}
	return nil
}

// ReadFile reads path and wraps failures as user errors.
func ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errs.WrapUser(err, "cannot read %s", path)
	}
	return data, nil
}

// osPathEqual compares paths with the host's case sensitivity.
func osPathEqual(a, b string) bool {
	if a == b {
		return true
	}
	return runtime.GOOS == "windows" && strings.EqualFold(a, b)
}
