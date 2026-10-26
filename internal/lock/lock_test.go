package lock_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/lock"
	"github.com/yuqing-256/woden/internal/testutil"
)

func readLockFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("lock", name)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestParseValidLock(t *testing.T) {
	data := readLockFixture(t, "vaild.lock")
	l, err := lock.Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if l.Version != lock.Version {
		t.Fatalf("version = %d, want %d", l.Version, lock.Version)
	}
	if got, want := len(l.Packages), 4; got != want {
		t.Fatalf("package count = %d, want %d", got, want)
	}
	alpha, ok := l.Find("alpha")
	if !ok {
		t.Fatal("missing alpha")
	}
	if alpha.Branch != "main" || alpha.Commit != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("alpha = %+v", alpha)
	}
	delta, ok := l.Find("delta")
	if !ok {
		t.Fatal("missing delta")
	}
	if delta.Rev != "fedcba9" || delta.Commit != "fedcba9876543210fedcba9876543210fedcba98" {
		t.Fatalf("delta = %+v", delta)
	}
	if kind, value := delta.RefKind(), delta.RefValue(); kind != "rev" || value != "fedcba9" {
		t.Fatalf("delta ref = (%q, %q)", kind, value)
	}
	gamma, ok := l.Find("gamma")
	if !ok {
		t.Fatal("missing gamma")
	}
	if kind, _ := gamma.RefKind(), gamma.RefValue(); kind != "" {
		t.Fatalf("gamma ref kind = %q, want no ref", kind)
	}

	if got := l.Bytes(); !bytes.Equal(got, data) {
		t.Fatalf("Bytes() = %q, want the original file %q", got, data)
	}
}

func TestLockBuildSortAndRoundTrip(t *testing.T) {
	l := lock.New()
	l.Upsert(lock.Package{Name: "zeta", Git: "https://example.com/zeta.git", Commit: strings.Repeat("a", 40)})
	l.Upsert(lock.Package{Name: "alpha", Git: "https://example.com/alpha.git", Branch: "main", Commit: strings.Repeat("b", 40)})

	got := string(l.Bytes())
	if !strings.HasPrefix(got, "version = 1\n") {
		t.Fatalf("lock = %q, want it to start with version = 1", got)
	}
	if strings.Index(got, `name = "alpha"`) > strings.Index(got, `name = "zeta"`) {
		t.Fatalf("lock packages are not sorted by name:\n%s", got)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "woden.lock")
	if err := l.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := lock.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !bytes.Equal(loaded.Bytes(), l.Bytes()) {
		t.Fatalf("round trip mismatch:\n%s\n---\n%s", loaded.Bytes(), l.Bytes())
	}

	l.Remove("zeta")
	if _, ok := l.Find("zeta"); ok {
		t.Fatal("Remove did not remove zeta")
	}
}

func TestParseRejectsInvalidLocks(t *testing.T) {
	fixtures := []string{"unknown-version.lock", "bad-syntax.lock", "missing-commit.lock", "dup-name.lock", "short-commit.lock"}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			if _, err := lock.Parse(readLockFixture(t, name)); err == nil {
				t.Fatal("Parse succeeded, want an error")
			}
		})
	}

	_, err := lock.Parse(readLockFixture(t, "unknown-version.lock"))
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unknown version error = %v", err)
	}
	_, err = lock.Parse(readLockFixture(t, "dup-name.lock"))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate name error = %v", err)
	}
}

func TestLoadMissingLock(t *testing.T) {
	_, err := lock.Load(filepath.Join(t.TempDir(), "woden.lock"))
	if err == nil || !lock.IsNotFound(err) {
		t.Fatalf("Load of a missing file = %v, want a not-found error", err)
	}
}
