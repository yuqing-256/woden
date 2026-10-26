//go:build e2e && woden_testing

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"woden/internal/testutil"
)

// requireFileTransport skips the test when git cannot use file:// URLs.
//
// The committed dependency scenarios use testutil.NewDepRepo, i.e. real
// `file://` URLs (docs/DECISIONS.md D-070). CI runners have working file://
// transport. In the DSH sandbox on Windows, however, `git clone file://...`
// spawns Git's sh.exe through named pipes, which the sandbox blocks ("couldn't
// create signal pipe, Win32 error 5"); local git commands still work. The probe
// below is the exact capability the tests need (docs/DECISIONS.md D-096).
func requireFileTransport(t *testing.T, repo *testutil.DepRepo) {
	t.Helper()
	cmd := exec.Command("git", "ls-remote", repo.URL)
	cmd.Dir = repo.Dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git file:// transport unavailable in this sandbox (CI runs it): %v\n%s", err, out)
	}
}

// requireDepsRepo builds a dependency repository and skips when the sandbox
// cannot use it. Every dependency test starts with this helper.
func requireDepsRepo(t *testing.T) *testutil.DepRepo {
	t.Helper()
	requireGit(t)
	repo := testutil.NewDepRepo(t)
	requireFileTransport(t, repo)
	return repo
}

// dependencyProbeFile is a valid Odin file that imports the dependency and
// uses it, so that `woden check` exercises the deps collection.
const dependencyProbeFile = `package main

import "deps:deplib"

dep_probe :: proc() string {
	return deplib.version()
}
`

// lockCommitPattern extracts the single `commit = "<40 hex>"` value of a lock
// file with exactly one package entry.
var lockCommitPattern = regexp.MustCompile(`(?m)^commit = "([0-9a-f]{40})"$`)

// requireLockCommit asserts that woden.lock pins the given full commit.
func requireLockCommit(t *testing.T, project, want string) {
	t.Helper()
	lock := readProjectFile(t, project, "woden.lock")
	matches := lockCommitPattern.FindAllStringSubmatch(lock, -1)
	if len(matches) != 1 {
		t.Fatalf("e2e: expected exactly one full commit in woden.lock, got %d:\n%s", len(matches), lock)
	}
	if matches[0][1] != want {
		t.Errorf("e2e: woden.lock pins commit %s, want %s\n%s", matches[0][1], want, lock)
	}
}

// requireLockRev asserts that the requested ref is recorded verbatim.
func requireLockRev(t *testing.T, project, ref string) {
	t.Helper()
	lock := readProjectFile(t, project, "woden.lock")
	needle := `rev = "` + ref + `"`
	if !strings.Contains(lock, needle) {
		t.Errorf("e2e: woden.lock does not record %s:\n%s", needle, lock)
	}
}

// requireVendoredVersion asserts that the vendored file exists and contains the
// given version marker.
func requireVendoredVersion(t *testing.T, project, marker string) {
	t.Helper()
	path := filepath.Join(project, "vendor", "deplib", "deplib.odin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("e2e: vendored file is missing: %v", err)
	}
	if !strings.Contains(string(data), marker) {
		t.Errorf("e2e: vendored deplib does not contain %q:\n%s", marker, data)
	}
}

// requireDepsCollection asserts that Woden added the deps collection mapping.
func requireDepsCollection(t *testing.T, project string) {
	t.Helper()
	toml := readProjectFile(t, project, "woden.toml")
	if !regexp.MustCompile(`(?m)^deps\s*=\s*"vendor"$`).MatchString(toml) {
		t.Errorf("e2e: woden.toml does not map deps = \"vendor\":\n%s", toml)
	}
}

// addDependency runs woden add on the probe project and verifies the common
// success shape.
func addDependency(t *testing.T, project, repoURL string, extra ...string) {
	t.Helper()
	args := append([]string{"add", "deplib", "--git", repoURL}, extra...)
	requireExit(t, runWoden(t, wodenTestingBin, project, args...), 0, "woden "+strings.Join(args, " "))
	requireDepsCollection(t, project)
}

// newProbeProject copies the example and adds the dependency probe file.
func newProbeProject(t *testing.T) string {
	t.Helper()
	project := testutil.CopyExample(t)
	writeProjectFile(t, project, "src/dep_probe.odin", dependencyProbeFile)
	return project
}

// TestAddDependencyDefaultRef adds a dependency tracked on the default branch.
func TestAddDependencyDefaultRef(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)

	if res := runWoden(t, wodenTestingBin, project, "check"); res.code != 101 {
		t.Fatalf("e2e: woden check before add: exit code %d, want 101\n%s", res.code, res.combined())
	}

	addDependency(t, project, repo.URL)
	requireLockCommit(t, project, repo.C2)
	requireVendoredVersion(t, project, "0.2.0")

	res := runWoden(t, wodenTestingBin, project, "check")
	requireExit(t, res, 0, "woden check after add")
	requireExit(t, runWoden(t, wodenTestingBin, project, "sync", "--check"), 0, "woden sync --check after add")
}

// TestAddDependencyBranch adds a dependency tracked on a named branch.
func TestAddDependencyBranch(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL, "--branch", "dev")
	requireLockCommit(t, project, repo.C3)
	requireVendoredVersion(t, project, "0.3.0-dev")
}

// TestAddDependencyTag adds a dependency pinned to a tag.
func TestAddDependencyTag(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL, "--tag", "v0.1.0")
	requireLockCommit(t, project, repo.C1)
	requireLockRev(t, project, "v0.1.0")
	requireVendoredVersion(t, project, "0.1.0")
}

// TestAddDependencyRev adds a dependency pinned to an abbreviated revision.
func TestAddDependencyRev(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	short := repo.C1[:7]
	addDependency(t, project, repo.URL, "--rev", short)
	requireLockCommit(t, project, repo.C1)
	requireLockRev(t, project, short)
	requireVendoredVersion(t, project, "0.1.0")
}

// TestUpdateAdvancesDefaultBranch checks that update follows the tracked
// branch tip.
func TestUpdateAdvancesDefaultBranch(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL)
	requireLockCommit(t, project, repo.C2)

	advanced := repo.AdvanceTrunk(t)
	requireExit(t, runWoden(t, wodenTestingBin, project, "update"), 0, "woden update")
	requireLockCommit(t, project, advanced)
	requireVendoredVersion(t, project, "0.2.1")
	requireExit(t, runWoden(t, wodenTestingBin, project, "check"), 0, "woden check after update")
}

// TestUpdateRejectsRevDependency checks that a revision-pinned dependency cannot
// be updated implicitly.
func TestUpdateRejectsRevDependency(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL, "--rev", repo.C1[:7])

	before := readProjectFile(t, project, "woden.lock")
	res := runWoden(t, wodenTestingBin, project, "update")
	requireExit(t, res, 101, "woden update on a rev-pinned dependency")
	if got := readProjectFile(t, project, "woden.lock"); got != before {
		t.Errorf("e2e: woden.lock changed although update should refuse:\n%s", got)
	}
}

// TestSyncRestoresMissingVendor checks the "deps configured but vendor missing"
// error and the sync recovery path.
func TestSyncRestoresMissingVendor(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL)
	lockBefore := readProjectFile(t, project, "woden.lock")

	if err := os.RemoveAll(filepath.Join(project, "vendor")); err != nil {
		t.Fatalf("e2e: cannot remove vendor: %v", err)
	}

	res := runWoden(t, wodenTestingBin, project, "check")
	requireExit(t, res, 101, "woden check with vendor/ deleted")
	if !strings.Contains(strings.ToLower(res.combined()), "sync") {
		t.Errorf("e2e: expected the error to suggest `woden sync`, got:\n%s", res.combined())
	}

	requireExit(t, runWoden(t, wodenTestingBin, project, "sync"), 0, "woden sync")
	requireExit(t, runWoden(t, wodenTestingBin, project, "check"), 0, "woden check after sync")
	if got := readProjectFile(t, project, "woden.lock"); got != lockBefore {
		t.Errorf("e2e: sync rewrote woden.lock unexpectedly:\nbefore:\n%s\nafter:\n%s", lockBefore, got)
	}
}

// TestDirtyVendorRefused checks that locally modified vendor files make
// sync/update/remove refuse to touch the dependency.
func TestDirtyVendorRefused(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL)
	lockBefore := readProjectFile(t, project, "woden.lock")

	const vendoredFile = "vendor/deplib/deplib.odin"
	original := readProjectFile(t, project, vendoredFile)
	dirty := original + "\n// local edit that must survive\n"
	writeProjectFile(t, project, vendoredFile, dirty)

	requireExit(t, runWoden(t, wodenTestingBin, project, "sync"), 101, "woden sync with a dirty vendor")
	requireExit(t, runWoden(t, wodenTestingBin, project, "update"), 101, "woden update with a dirty vendor")
	requireExit(t, runWoden(t, wodenTestingBin, project, "remove", "deplib"), 101, "woden remove with a dirty vendor")

	if got := readProjectFile(t, project, vendoredFile); got != dirty {
		t.Errorf("e2e: dirty vendor file was overwritten or deleted:\n%s", got)
	}
	if got := readProjectFile(t, project, "woden.lock"); got != lockBefore {
		t.Errorf("e2e: woden.lock changed although the commands should refuse:\n%s", got)
	}
}

// TestRemoveLastDependency checks the cleanup when the final dependency is
// removed: deps mapping, vendor/, ols.json entry and lock entry.
func TestRemoveLastDependency(t *testing.T) {
	repo := requireDepsRepo(t)
	requireRealTools(t)
	project := newProbeProject(t)
	addDependency(t, project, repo.URL)

	requireExit(t, runWoden(t, wodenTestingBin, project, "remove", "deplib"), 0, "woden remove deplib")

	toml := readProjectFile(t, project, "woden.toml")
	if strings.Contains(toml, "[dependencies.deplib]") || strings.Contains(toml, `deps = "vendor"`) {
		t.Errorf("e2e: woden.toml still references the removed dependency:\n%s", toml)
	}
	if _, err := os.Stat(filepath.Join(project, "vendor")); !os.IsNotExist(err) {
		t.Errorf("e2e: empty vendor/ directory was not removed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(project, "woden.lock")); err == nil {
		if strings.Contains(string(data), "deplib") || strings.Contains(string(data), "[[package]]") {
			t.Errorf("e2e: woden.lock still lists the removed dependency:\n%s", data)
		}
	}
	ols := readProjectFile(t, project, "ols.json")
	if strings.Contains(ols, `"name": "deps"`) || strings.Contains(ols, "vendor") {
		t.Errorf("e2e: ols.json still references the removed dependency:\n%s", ols)
	}
}
