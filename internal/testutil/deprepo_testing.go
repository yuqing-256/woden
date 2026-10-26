//go:build woden_testing

package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// DepRepo is a local Git repository used as a dependency source by the tests.
//
// It is only available in builds with the woden_testing tag: released binaries
// refuse file:// Git URLs, and the tests are the only reason Woden ever needs
// them.
type DepRepo struct {
	// URL is the file:// URL of the repository.
	URL string
	// Dir is the repository working directory.
	Dir string
	// C1 is the commit containing testdata/deplib/rev1.
	C1 string
	// C2 is the commit containing testdata/deplib/rev2.
	C2 string
	// C3 is the commit containing testdata/deplib/rev3 (on branch "dev").
	C3 string
	// DefaultBranch is the name of the remote default branch ("trunk").
	DefaultBranch string
}

const depRepoDefaultBranch = "trunk"

// NewDepRepo creates the Git repository described by docs/FIXTURES.md.
func NewDepRepo(t testing.TB) *DepRepo {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "deplib")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("testutil: cannot create %s: %v", dir, err)
	}
	repo := &DepRepo{Dir: dir, DefaultBranch: depRepoDefaultBranch}
	repo.git(t, "init", "-b", depRepoDefaultBranch, ".")
	repo.commitRev(t, 1, "C1")
	repo.C1 = repo.revParse(t, "HEAD")
	repo.git(t, "tag", "-a", "v0.1.0", "-m", "v0.1.0")
	repo.commitRev(t, 2, "C2")
	repo.C2 = repo.revParse(t, "HEAD")
	repo.git(t, "tag", "v0.2.0")
	repo.git(t, "checkout", "-b", "dev")
	repo.commitRev(t, 3, "C3")
	repo.C3 = repo.revParse(t, "HEAD")
	repo.git(t, "checkout", depRepoDefaultBranch)
	repo.URL = fileURL(dir)
	return repo
}

// AdvanceTrunk commits testdata/deplib/rev4 on the default branch and returns
// the new commit ID.
func (r *DepRepo) AdvanceTrunk(t testing.TB) string {
	t.Helper()
	r.git(t, "checkout", depRepoDefaultBranch)
	r.commitRev(t, 4, "C4")
	return r.revParse(t, "HEAD")
}

// MoveTag force-moves tag to commit.
func (r *DepRepo) MoveTag(t testing.TB, tag, commit string) {
	t.Helper()
	r.git(t, "tag", "-f", "-a", tag, "-m", tag, commit)
}

// commitRev writes testdata/deplib/revN as deplib.odin and commits it.
func (r *DepRepo) commitRev(t testing.TB, rev int, message string) {
	t.Helper()
	src := Testdata(t, "deplib/rev"+itoa(rev)+"/deplib.odin")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("testutil: cannot read %s: %v", src, err)
	}
	if err := os.WriteFile(filepath.Join(r.Dir, "deplib.odin"), data, 0o644); err != nil {
		t.Fatalf("testutil: cannot write deplib.odin: %v", err)
	}
	r.git(t, "add", "-A")
	r.git(t, "commit", "-m", message)
}

// revParse resolves revision to a full commit ID.
func (r *DepRepo) revParse(t testing.TB, revision string) string {
	t.Helper()
	return r.git(t, "rev-parse", revision)
}

// git runs a Git command in the repository with a deterministic environment.
func (r *DepRepo) git(t testing.TB, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"GIT_AUTHOR_NAME=Woden Test",
		"GIT_AUTHOR_EMAIL=woden@example.com",
		"GIT_COMMITTER_NAME=Woden Test",
		"GIT_COMMITTER_EMAIL=woden@example.com",
		"GIT_AUTHOR_DATE=2020-01-01T00:00:00+0000",
		"GIT_COMMITTER_DATE=2020-01-01T00:00:00+0000",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("testutil: git %s in %s failed: %v\n%s", strings.Join(args, " "), r.Dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// fileURL converts a local directory into a file:// URL.
func fileURL(dir string) string {
	slashed := filepath.ToSlash(dir)
	if runtime.GOOS == "windows" {
		return "file:///" + slashed
	}
	return "file://" + slashed
}

// itoa converts a small positive integer to its decimal representation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
