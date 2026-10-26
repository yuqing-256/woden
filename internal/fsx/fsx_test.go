package fsx_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/fsx"
)

func TestAbsClean(t *testing.T) {
	if _, err := fsx.AbsClean(""); err == nil {
		t.Fatal("AbsClean(\"\") should fail")
	}
	got, err := fsx.AbsClean(filepath.Join("a", "b"))
	if err != nil {
		t.Fatalf("AbsClean: %v", err)
	}
	if !filepath.IsAbs(got) || got != filepath.Clean(got) {
		t.Fatalf("AbsClean returned %q, want an absolute cleaned path", got)
	}
}

func TestIsWithin(t *testing.T) {
	root := t.TempDir()
	sibling := filepath.Join(filepath.Dir(root), "woden-fsx-sibling")

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{"root itself", root, true},
		{"direct child", filepath.Join(root, "src"), true},
		{"nested child", filepath.Join(root, "a", "b", "c"), true},
		{"parent", filepath.Dir(root), false},
		{"sibling with shared prefix", sibling, false},
		{"dotdot escape", filepath.Join(root, "..", "other"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fsx.IsWithin(root, tt.target); got != tt.want {
				t.Fatalf("IsWithin(%q, %q) = %v, want %v", root, tt.target, got, tt.want)
			}
		})
	}

	if runtime.GOOS == "windows" {
		if !fsx.IsWithin(strings.ToUpper(root), filepath.Join(root, "src")) {
			t.Fatal("IsWithin must ignore case on Windows")
		}
	}
}

func TestWithin(t *testing.T) {
	root := t.TempDir()
	got, err := fsx.Within(root, "src")
	if err != nil {
		t.Fatalf("Within: %v", err)
	}
	if want := filepath.Join(root, "src"); got != want {
		t.Fatalf("Within = %q, want %q", got, want)
	}
	for _, path := range []string{"../outside", filepath.Join("..", "outside"), "/etc/passwd"} {
		if _, err := fsx.Within(root, path); err == nil {
			t.Fatalf("Within(%q) should fail", path)
		} else if !errs.IsUser(err) {
			t.Fatalf("Within(%q) error is not a user error: %v", path, err)
		}
	}
}

func TestTempFileCommit(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "woden.toml")

	tmp, err := fsx.NewTempFile(dst)
	if err != nil {
		t.Fatalf("NewTempFile: %v", err)
	}
	if _, err := tmp.Write([]byte("[package]\n")); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmp.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(data) != "[package]\n" {
		t.Fatalf("destination content = %q", data)
	}
	if _, err := os.Stat(tmp.Path()); !os.IsNotExist(err) {
		t.Fatalf("temporary file still exists after Commit: %v", err)
	}
	tmp.CloseAndRemove() // must be a no-op after Commit

	discarded := filepath.Join(dir, "discarded")
	tmp2, err := fsx.NewTempFile(discarded)
	if err != nil {
		t.Fatalf("NewTempFile: %v", err)
	}
	tmp2.CloseAndRemove()
	if _, err := os.Stat(discarded); !os.IsNotExist(err) {
		t.Fatalf("CloseAndRemove created the destination: %v", err)
	}
}

func TestWriteNewAndAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "woden.toml")

	if err := fsx.WriteNew(path, []byte("one\n")); err != nil {
		t.Fatalf("WriteNew: %v", err)
	}
	if err := fsx.WriteNew(path, []byte("two\n")); err == nil {
		t.Fatal("WriteNew must never overwrite an existing file")
	} else if !strings.Contains(err.Error(), "already exists") || !errs.IsUser(err) {
		t.Fatalf("unexpected WriteNew error: %v", err)
	}

	if err := fsx.AtomicWrite(path, []byte("three\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != "three\n" {
		t.Fatalf("content = %q", data)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".woden-tmp-*"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestAtomicWritePreservesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "woden.toml")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := fsx.AtomicWrite(path, []byte("new\n"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o, want 600", got)
	}
}
