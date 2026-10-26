// Package testutil contains helpers for Woden's tests.
//
// The helpers never operate on example/ or testdata/ in place: tests that need
// a mutable project copy example/ into a temporary directory first.
package testutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RepoRoot returns the root of the Woden repository (the directory containing
// go.mod).
func RepoRoot(t testing.TB) string {
	t.Helper()
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, ok := findUpward(filepath.Dir(file)); ok {
			return root
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("testutil: cannot determine working directory: %v", err)
	}
	if root, ok := findUpward(wd); ok {
		return root
	}
	t.Fatalf("testutil: cannot locate the repository root (go.mod) from %q", wd)
	return ""
}

// findUpward walks up from dir looking for go.mod.
func findUpward(dir string) (string, bool) {
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Testdata returns the absolute path of a file under testdata/.
func Testdata(t testing.TB, rel string) string {
	t.Helper()
	return filepath.Join(RepoRoot(t), "testdata", filepath.FromSlash(rel))
}

// CopyExample copies example/ into a new temporary directory and returns the
// path of the copy.
func CopyExample(t testing.TB) string {
	t.Helper()
	src := filepath.Join(RepoRoot(t), "example")
	dst := filepath.Join(t.TempDir(), "example")
	CopyDir(t, src, dst)
	return dst
}

// CopyDir copies the directory tree src to dst.
func CopyDir(t testing.TB, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("testutil: cannot copy %s to %s: %v", src, dst, err)
	}
}

// WriteFile writes content to path, creating parent directories as needed.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("testutil: cannot create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("testutil: cannot write %s: %v", path, err)
	}
}

// ReadFile reads path as a string.
func ReadFile(t testing.TB, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("testutil: cannot read %s: %v", path, err)
	}
	return string(data)
}

// ToCRLF converts every LF in s to CRLF. Tests use it to check that Woden
// preserves the line ending style of files it edits; no CRLF file is stored in
// the repository.
func ToCRLF(s string) string {
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// Chdir changes the working directory for the duration of the test.
func Chdir(t testing.TB, dir string) {
	t.Helper()
	t.Chdir(dir)
}

// TempDir returns a fresh temporary directory belonging to the test.
func TempDir(t testing.TB) string {
	t.Helper()
	return t.TempDir()
}

// IsWindows reports whether the tests run on Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}
