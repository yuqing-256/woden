package testutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/testutil"
)

func TestRepoRoot(t *testing.T) {
	root := testutil.RepoRoot(t)
	if !filepath.IsAbs(root) {
		t.Fatalf("RepoRoot = %q, want an absolute path", root)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("RepoRoot does not point at the repository: %v", err)
	}
}

func TestTestdata(t *testing.T) {
	path := testutil.Testdata(t, filepath.Join("toml", "vaild-minimal.toml"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read testdata file: %v", err)
	}
	if !strings.Contains(string(data), `name = "my-game"`) {
		t.Fatalf("unexpected fixture content: %q", data)
	}
}

func TestCopyExample(t *testing.T) {
	dir := testutil.CopyExample(t)
	if filepath.Base(dir) != "example" {
		t.Fatalf("CopyExample returned %q, want a directory named example", dir)
	}
	for _, rel := range []string{"woden.toml", "ols.json", ".gitignore", filepath.Join("src", "main.odin"), filepath.Join("tests", "main_test.odin")} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatalf("copied example is missing %s: %v", rel, err)
		}
	}
	// The copy must be independent from the checked-in example.
	extra := filepath.Join(dir, "copy-marker.txt")
	if err := os.WriteFile(extra, []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if _, err := os.Stat(filepath.Join(testutil.RepoRoot(t), "example", "copy-marker.txt")); !os.IsNotExist(err) {
		t.Fatalf("CopyExample modified the checked-in example: %v", err)
	}
}

func TestToCRLF(t *testing.T) {
	got := testutil.ToCRLF("a\nb\n")
	if got != "a\r\nb\r\n" {
		t.Fatalf("ToCRLF = %q", got)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Fatalf("ToCRLF left a bare LF in %q", got)
	}
}
