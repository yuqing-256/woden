//go:build woden_testing

package testutil_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/testutil"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestNewDepRepo(t *testing.T) {
	repo := testutil.NewDepRepo(t)

	if !strings.HasPrefix(repo.URL, "file://") {
		t.Fatalf("URL = %q, want a file:// URL", repo.URL)
	}
	if repo.DefaultBranch != "trunk" {
		t.Fatalf("DefaultBranch = %q, want trunk", repo.DefaultBranch)
	}
	if len(repo.C1) != 40 || len(repo.C2) != 40 || len(repo.C3) != 40 {
		t.Fatalf("commit IDs are not full: C1=%q C2=%q C3=%q", repo.C1, repo.C2, repo.C3)
	}
	if repo.C1 == repo.C2 || repo.C2 == repo.C3 {
		t.Fatalf("commits are not distinct: %q %q %q", repo.C1, repo.C2, repo.C3)
	}

	// The worktree is checked out on trunk at C2.
	if got := gitIn(t, repo.Dir, "rev-parse", "HEAD"); got != repo.C2 {
		t.Fatalf("HEAD = %q, want C2 %q", got, repo.C2)
	}
	data, err := os.ReadFile(filepath.Join(repo.Dir, "deplib.odin"))
	if err != nil {
		t.Fatalf("read deplib.odin: %v", err)
	}
	if !strings.Contains(string(data), "0.2.0") {
		t.Fatalf("trunk content = %q, want rev2 (0.2.0)", data)
	}

	if got := gitIn(t, repo.Dir, "rev-parse", "refs/tags/v0.1.0^{commit}"); got != repo.C1 {
		t.Fatalf("v0.1.0 = %q, want C1 %q", got, repo.C1)
	}
	if got := gitIn(t, repo.Dir, "rev-parse", "refs/tags/v0.2.0^{commit}"); got != repo.C2 {
		t.Fatalf("v0.2.0 = %q, want C2 %q", got, repo.C2)
	}
	if got := gitIn(t, repo.Dir, "rev-parse", "refs/heads/dev"); got != repo.C3 {
		t.Fatalf("dev = %q, want C3 %q", got, repo.C3)
	}
}

func TestAdvanceTrunkAndMoveTag(t *testing.T) {
	repo := testutil.NewDepRepo(t)

	newID := repo.AdvanceTrunk(t)
	if newID == repo.C2 {
		t.Fatal("AdvanceTrunk returned the old commit")
	}
	if got := gitIn(t, repo.Dir, "rev-parse", "HEAD"); got != newID {
		t.Fatalf("HEAD after AdvanceTrunk = %q, want %q", got, newID)
	}
	data, err := os.ReadFile(filepath.Join(repo.Dir, "deplib.odin"))
	if err != nil {
		t.Fatalf("read deplib.odin: %v", err)
	}
	if !strings.Contains(string(data), "0.2.1") {
		t.Fatalf("advanced content = %q, want rev4 (0.2.1)", data)
	}

	repo.MoveTag(t, "v0.1.0", repo.C2)
	if got := gitIn(t, repo.Dir, "rev-parse", "refs/tags/v0.1.0^{commit}"); got != repo.C2 {
		t.Fatalf("moved tag = %q, want %q", got, repo.C2)
	}
}
