package clean_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/clean"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/fsx"
	"github.com/yuqing-256/woden/internal/testutil"
)

// testTree creates a project with a populated target/ directory and an outside
// directory that must survive clean.
func testTree(t *testing.T) (root, outside string) {
	t.Helper()
	root = testutil.TempDir(t)
	outside = filepath.Join(root, "outside")
	testutil.WriteFile(t, filepath.Join(outside, "keep.txt"), "keep\n")
	testutil.WriteFile(t, filepath.Join(root, "woden.toml"), "[package]\nname = \"demo\"\n")
	testutil.WriteFile(t, filepath.Join(root, "target", "debug", "woden-example.exe"), "exe\n")
	testutil.WriteFile(t, filepath.Join(root, "target", "release", "lib.a"), "lib\n")
	testutil.WriteFile(t, filepath.Join(root, "target", "notes.txt"), "notes\n")
	testutil.WriteFile(t, filepath.Join(root, "target", "a", "b", "c.txt"), "c\n")
	return root, outside
}

// symlink creates a symbolic link or skips the test when the platform does not
// allow it (Windows without developer mode).
func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symbolic link %s: %v", link, err)
	}
}

// snapshot describes a whole tree, including file contents, without following
// links. Tests use it to prove that a command changed nothing.
func snapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	var walk func(dir, rel string)
	walk = func(dir, rel string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("cannot read %s: %v", dir, err)
		}
		for _, entry := range entries {
			p := filepath.Join(dir, entry.Name())
			relp := path.Join(rel, entry.Name())
			info, err := os.Lstat(p)
			if err != nil {
				t.Fatalf("cannot inspect %s: %v", p, err)
			}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				dest, err := os.Readlink(p)
				if err != nil {
					t.Fatalf("cannot read link %s: %v", p, err)
				}
				fmt.Fprintf(&b, "%s -> %s\n", relp, dest)
			case info.IsDir():
				fmt.Fprintf(&b, "%s/\n", relp)
				walk(p, relp)
			default:
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatalf("cannot read %s: %v", p, err)
				}
				fmt.Fprintf(&b, "%s = %q\n", relp, data)
			}
		}
	}
	walk(root, "")
	return b.String()
}

// equalSlices reports whether a and b contain the same strings in the same
// order.
func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildPlanAndRemove(t *testing.T) {
	root, outside := testTree(t)
	target := filepath.Join(root, "target")
	// Symlink creation is not allowed on every Windows machine, so the
	// symlink cases are optional; the rest of the test always runs.
	symlinks := false
	if err := os.Symlink(outside, filepath.Join(target, "link-out")); err == nil {
		if err := os.Symlink(filepath.Join(target, "debug", "woden-example.exe"), filepath.Join(target, "link-in")); err == nil {
			symlinks = true
		} else {
			os.Remove(filepath.Join(target, "link-out"))
		}
	}

	before := snapshot(t, root)

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Target != filepath.Clean(target) {
		t.Errorf("plan.Target = %q, want %q", plan.Target, filepath.Clean(target))
	}
	if len(plan.Entries) == 0 {
		t.Fatal("plan has no entries")
	}
	for _, e := range plan.Entries {
		if !fsx.IsWithin(plan.Target, e.Path) {
			t.Errorf("entry %q is outside %s", e.Path, plan.Target)
		}
		if !strings.HasPrefix(e.Rel, "target/") {
			t.Errorf("entry %q has unexpected project-relative path", e.Rel)
		}
	}
	// Children must come before their parent directory.
	for i := range plan.Entries {
		for j := i + 1; j < len(plan.Entries); j++ {
			if fsx.IsWithin(plan.Entries[i].Path, plan.Entries[j].Path) {
				t.Errorf("%s (entry %d) is an ancestor of a later entry %s (entry %d)",
					plan.Entries[i].Rel, i, plan.Entries[j].Rel, j)
			}
		}
	}
	want := []string{
		"target/a/b/c.txt", "target/a/b", "target/a",
		"target/debug/woden-example.exe", "target/debug",
		"target/link-in", "target/link-out",
		"target/notes.txt",
		"target/release/lib.a", "target/release",
	}
	if !symlinks {
		want = []string{
			"target/a/b/c.txt", "target/a/b", "target/a",
			"target/debug/woden-example.exe", "target/debug",
			"target/notes.txt",
			"target/release/lib.a", "target/release",
		}
	}
	if got := plan.RelPaths(); !equalSlices(got, want) {
		t.Errorf("plan.RelPaths() = %v, want %v", got, want)
	}

	// Building the plan is read-only.
	if after := snapshot(t, root); after != before {
		t.Fatalf("building the plan changed the tree:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if got := testutil.ReadFile(t, filepath.Join(outside, "keep.txt")); got != "keep\n" {
		t.Errorf("outside file changed before Remove: %q", got)
	}

	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target still exists after Remove: %v", err)
	}
	if got := testutil.ReadFile(t, filepath.Join(outside, "keep.txt")); got != "keep\n" {
		t.Errorf("outside file changed: %q", got)
	}
	if ok, err := fsx.Exists(filepath.Join(root, "woden.toml")); err != nil || !ok {
		t.Errorf("woden.toml was removed (exists=%v, err=%v)", ok, err)
	}
}

func TestBuildPlanMissingTarget(t *testing.T) {
	root := testutil.TempDir(t)
	testutil.WriteFile(t, filepath.Join(root, "woden.toml"), "[package]\nname = \"demo\"\n")

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Entries) != 0 {
		t.Errorf("plan has entries though target/ is missing: %v", plan.RelPaths())
	}
	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if ok, err := fsx.Exists(filepath.Join(root, "target")); err != nil || ok {
		t.Errorf("target/ exists after removal (exists=%v, err=%v)", ok, err)
	}
	if got := testutil.ReadFile(t, filepath.Join(root, "woden.toml")); got != "[package]\nname = \"demo\"\n" {
		t.Errorf("woden.toml changed: %q", got)
	}
}

func TestBuildPlanEmptyTarget(t *testing.T) {
	root := testutil.TempDir(t)
	if err := os.Mkdir(filepath.Join(root, "target"), 0o755); err != nil {
		t.Fatalf("cannot create target: %v", err)
	}

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Entries) != 0 {
		t.Errorf("plan has entries though target/ is empty: %v", plan.RelPaths())
	}
	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "target")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target still exists after Remove: %v", err)
	}
}

func TestTargetSymlinkOnlyLinkRemoved(t *testing.T) {
	root := testutil.TempDir(t)
	outside := filepath.Join(root, "outside")
	testutil.WriteFile(t, filepath.Join(outside, "keep.txt"), "keep\n")
	target := filepath.Join(root, "target")
	symlink(t, outside, target)

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Entries) != 1 {
		t.Fatalf("plan has %d entries, want 1: %v", len(plan.Entries), plan.RelPaths())
	}
	if plan.Entries[0].Rel != "target" {
		t.Errorf("entry rel = %q, want %q", plan.Entries[0].Rel, "target")
	}
	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target link still exists after Remove: %v", err)
	}
	if got := testutil.ReadFile(t, filepath.Join(outside, "keep.txt")); got != "keep\n" {
		t.Errorf("symlink target changed: %q", got)
	}
}

func TestTargetNotDirectory(t *testing.T) {
	root := testutil.TempDir(t)
	testutil.WriteFile(t, filepath.Join(root, "target"), "not a directory\n")

	if _, err := clean.BuildPlan(root); err == nil {
		t.Fatal("BuildPlan succeeded though target is a file")
	} else if !errs.IsUser(err) {
		t.Errorf("BuildPlan error is not a user error: %v", err)
	}
	if got := testutil.ReadFile(t, filepath.Join(root, "target")); got != "not a directory\n" {
		t.Errorf("target file changed: %q", got)
	}
}

func TestRemoveRefusesEntriesOutsideTarget(t *testing.T) {
	root := testutil.TempDir(t)
	target := filepath.Join(root, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("cannot create target: %v", err)
	}
	outsideFile := filepath.Join(root, "outside", "keep.txt")
	testutil.WriteFile(t, outsideFile, "keep\n")

	plan := &clean.Plan{
		Root:   root,
		Target: target,
		Entries: []clean.Entry{
			{Path: outsideFile, Rel: "outside/keep.txt"},
		},
	}
	if err := plan.Remove(); err == nil {
		t.Fatal("Remove deleted an entry outside target")
	} else if !errs.IsUser(err) {
		t.Errorf("Remove error is not a user error: %v", err)
	}
	if got := testutil.ReadFile(t, outsideFile); got != "keep\n" {
		t.Errorf("outside file changed: %q", got)
	}

	empty := &clean.Plan{}
	if err := empty.Remove(); err == nil {
		t.Error("Remove succeeded for a plan without a target")
	}
}

func TestWeirdNames(t *testing.T) {
	root := testutil.TempDir(t)
	target := filepath.Join(root, "target")
	testutil.WriteFile(t, filepath.Join(target, "..dotfile"), "dot\n")
	testutil.WriteFile(t, filepath.Join(target, "space name.txt"), "space\n")
	testutil.WriteFile(t, filepath.Join(target, "dir..with..dots", "nested file.txt"), "nested\n")
	testutil.WriteFile(t, filepath.Join(target, "héllo.txt"), "unicode\n")

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	want := []string{
		"target/..dotfile",
		"target/dir..with..dots/nested file.txt",
		"target/dir..with..dots",
		"target/héllo.txt",
		"target/space name.txt",
	}
	if got := plan.RelPaths(); !equalSlices(got, want) {
		t.Fatalf("plan.RelPaths() = %v, want %v", got, want)
	}
	for _, e := range plan.Entries {
		if !fsx.IsWithin(plan.Target, e.Path) {
			t.Errorf("entry %q is outside %s", e.Path, plan.Target)
		}
	}
	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target still exists after Remove: %v", err)
	}
}

func TestJunctionIsNotTraversed(t *testing.T) {
	if !testutil.IsWindows() {
		t.Skip("junctions are a Windows feature")
	}
	root := testutil.TempDir(t)
	outside := filepath.Join(root, "outside")
	testutil.WriteFile(t, filepath.Join(outside, "keep.txt"), "keep\n")
	target := filepath.Join(root, "target")
	testutil.WriteFile(t, filepath.Join(target, "notes.txt"), "notes\n")

	junction := filepath.Join(target, "junction")
	// mklink /J creates a junction without requiring administrator rights.
	cmd := exec.Command("cmd", "/c", "mklink", "/J", junction, outside)
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot create a junction: %v", err)
	}

	plan, err := clean.BuildPlan(root)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	found := false
	for _, e := range plan.Entries {
		if e.Rel == "target/junction" {
			found = true
		}
		if strings.HasPrefix(e.Rel, "target/junction/") {
			t.Fatalf("the plan descends into the junction: %s", e.Rel)
		}
	}
	if !found {
		t.Fatalf("the junction is not part of the plan: %v", plan.RelPaths())
	}
	if err := plan.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("target still exists after Remove: %v", err)
	}
	if got := testutil.ReadFile(t, filepath.Join(outside, "keep.txt")); got != "keep\n" {
		t.Errorf("junction target changed: %q", got)
	}
}
