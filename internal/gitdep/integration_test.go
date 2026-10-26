//go:build woden_testing

package gitdep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"woden/internal/config"
	"woden/internal/fsx"
	"woden/internal/lock"
	"woden/internal/project"
)

// integration tests exercise the real Git CLI. They are build-tagged with
// woden_testing because Git URL validation only accepts file:// URLs (and the
// tests copy them from testutil.NewDepRepo) in that build. Run them with:
//
//	go test -tags woden_testing ./internal/gitdep/

// TestIntegrationAddDefaultBranch checks the whole add flow for a dependency
// without a requested ref: Git default branch, lock, woden.toml, ols.json and a
// checked out vendor worktree.
func TestIntegrationAddDefaultBranch(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, stdout, stderr := integrationProject(t)

	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v\nstderr: %s", err, stderr.String())
	}

	pkg := integrationLock(t, proj, "deplib")
	if pkg.Git != repo.URL {
		t.Errorf("gitdep: lock records git %q, want %q", pkg.Git, repo.URL)
	}
	if pkg.Commit != repo.C2 {
		t.Errorf("gitdep: lock records commit %q, want the default branch tip %q", pkg.Commit, repo.C2)
	}
	if pkg.Branch != "" || pkg.Tag != "" || pkg.Rev != "" {
		t.Errorf("gitdep: lock records a ref for a default-branch dependency: %+v", pkg)
	}
	if got := integrationCommit(t, proj, "deplib"); got != repo.C2 {
		t.Errorf("gitdep: vendor worktree is at %q, want %q", got, repo.C2)
	}
	if _, err := os.Stat(filepath.Join(proj.VendorDir(), "deplib", "deplib.odin")); err != nil {
		t.Errorf("gitdep: vendor worktree does not contain deplib.odin: %v", err)
	}
	cfg := integrationConfig(t, proj)
	dep, ok := cfg.DependencyByName("deplib")
	if !ok || dep.Git != repo.URL || dep.Branch != "" {
		t.Errorf("gitdep: woden.toml declares %+v (exists: %v)", dep, ok)
	}
	if path, ok := cfg.DepsCollection(); !ok || path != "vendor" {
		t.Errorf(`gitdep: woden.toml collection "deps" = %q (exists: %v), want "vendor"`, path, ok)
	}
	if got := integrationOLSCollections(t, proj)["deps"]; got != "vendor" {
		t.Errorf(`gitdep: ols.json collection "deps" = %q, want "vendor"`, got)
	}
	mustContain(t, stdout.String(), "added dependency deplib (the remote default branch) at ")
}

// TestIntegrationAddRequestedRefs adds dependencies pinned to a branch, a tag
// and an abbreviated revision, and pins full commit IDs in woden.lock.
func TestIntegrationAddRequestedRefs(t *testing.T) {
	repo := newIntegrationRepo(t)

	cases := []struct {
		name    string
		options AddOptions
		commit  string
		check   func(t *testing.T, pkg lock.Package)
	}{
		{
			name:    "branch",
			options: AddOptions{Name: "branchlib", Git: repo.URL, Branch: "dev"},
			commit:  repo.C3,
			check: func(t *testing.T, pkg lock.Package) {
				if pkg.Branch != "dev" {
					t.Errorf("gitdep: lock records branch %q, want dev", pkg.Branch)
				}
			},
		},
		{
			name:    "tag",
			options: AddOptions{Name: "taglib", Git: repo.URL, Tag: "v0.1.0"},
			commit:  repo.C1,
			check: func(t *testing.T, pkg lock.Package) {
				if pkg.Tag != "v0.1.0" {
					t.Errorf("gitdep: lock records tag %q, want v0.1.0", pkg.Tag)
				}
			},
		},
		{
			name:    "rev stays abbreviated",
			options: AddOptions{Name: "revlib", Git: repo.URL, Rev: repo.C1[:7]},
			commit:  repo.C1,
			check: func(t *testing.T, pkg lock.Package) {
				if pkg.Rev != repo.C1[:7] {
					t.Errorf("gitdep: lock records rev %q, want the abbreviated request %q", pkg.Rev, repo.C1[:7])
				}
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			proj, deps, _, _ := integrationProject(t)
			if err := deps.Add(context.Background(), testCase.options); err != nil {
				t.Fatalf("gitdep: Add failed: %v", err)
			}
			pkg := integrationLock(t, proj, testCase.options.Name)
			if pkg.Commit != testCase.commit {
				t.Errorf("gitdep: lock records commit %q, want %q", pkg.Commit, testCase.commit)
			}
			if len(pkg.Commit) != 40 {
				t.Errorf("gitdep: lock does not pin a full commit: %q", pkg.Commit)
			}
			if got := integrationCommit(t, proj, testCase.options.Name); got != testCase.commit {
				t.Errorf("gitdep: vendor worktree is at %q, want %q", got, testCase.commit)
			}
			testCase.check(t, pkg)
		})
	}
}

// TestIntegrationAddDuplicateDependency pins the duplicate-name error.
func TestIntegrationAddDuplicateDependency(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, _, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: first Add failed: %v", err)
	}
	err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL, Branch: "dev"})
	if err == nil {
		t.Fatalf("gitdep: Add accepted a duplicate dependency name")
	}
	mustContain(t, err.Error(), `dependency "deplib" is already declared in woden.toml`)
	if pkg := integrationLock(t, proj, "deplib"); pkg.Commit != repo.C2 {
		t.Errorf("gitdep: the failed Add modified the lock: %+v", pkg)
	}
}

// TestIntegrationSyncRestoresMissingWorktree deletes the vendor worktree and
// checks that sync restores it at the locked commit without moving the lock.
func TestIntegrationSyncRestoresMissingWorktree(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, stdout, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	before := integrationLock(t, proj, "deplib")

	if err := os.RemoveAll(filepath.Join(proj.VendorDir(), "deplib")); err != nil {
		t.Fatalf("gitdep: cannot delete the vendor worktree: %v", err)
	}
	if err := deps.Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj.VendorDir(), "deplib", "deplib.odin")); err != nil {
		t.Errorf("gitdep: Sync did not restore the vendor worktree: %v", err)
	}
	if got := integrationCommit(t, proj, "deplib"); got != before.Commit {
		t.Errorf("gitdep: restored worktree is at %q, want the locked %q", got, before.Commit)
	}
	after := integrationLock(t, proj, "deplib")
	if after.Commit != before.Commit {
		t.Errorf("gitdep: Sync moved the locked commit: %q -> %q", before.Commit, after.Commit)
	}
	mustContain(t, stdout.String(), "restored deplib at ")
}

// TestIntegrationSyncDoesNotAdvance pins that a normal sync never advances a
// dependency to the newer remote commit.
func TestIntegrationSyncDoesNotAdvance(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, _, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	repo.AdvanceTrunk(t)

	if err := deps.Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}
	if pkg := integrationLock(t, proj, "deplib"); pkg.Commit != repo.C2 {
		t.Errorf("gitdep: Sync advanced the locked commit to %q, want %q", pkg.Commit, repo.C2)
	}
	if got := integrationCommit(t, proj, "deplib"); got != repo.C2 {
		t.Errorf("gitdep: Sync moved the vendor worktree to %q, want %q", got, repo.C2)
	}
}

// TestIntegrationSyncCheck pins the read-only checker: a clean project passes,
// and every kind of inconsistency is reported without writing anything.
func TestIntegrationSyncCheck(t *testing.T) {
	t.Run("clean", func(t *testing.T) {
		repo := newIntegrationRepo(t)
		proj, deps, _, _ := integrationProject(t)
		if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
			t.Fatalf("gitdep: Add failed: %v", err)
		}
		before := integrationSnapshot(t, proj)
		if err := deps.Sync(context.Background(), true); err != nil {
			t.Errorf("gitdep: sync --check failed on a clean project: %v", err)
		}
		if after := integrationSnapshot(t, proj); after != before {
			t.Errorf("gitdep: sync --check changed the project:\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	scenarios := []struct {
		name    string
		breakIt func(t *testing.T, proj *project.Project, repo *integrationRepo)
		want    string
	}{
		{
			name: "dirty worktree",
			breakIt: func(t *testing.T, proj *project.Project, repo *integrationRepo) {
				writeIntegrationFile(t, filepath.Join(proj.VendorDir(), "deplib", "local.txt"), "local change\n")
			},
			want: "vendor/deplib has local modifications",
		},
		{
			name: "missing worktree",
			breakIt: func(t *testing.T, proj *project.Project, repo *integrationRepo) {
				if err := os.RemoveAll(filepath.Join(proj.VendorDir(), "deplib")); err != nil {
					t.Fatalf("gitdep: cannot delete the vendor worktree: %v", err)
				}
			},
			want: "vendor/deplib is missing; run `woden sync`",
		},
		{
			name: "manifest and lock disagree",
			breakIt: func(t *testing.T, proj *project.Project, repo *integrationRepo) {
				lk, err := lock.Load(proj.LockPath())
				if err != nil {
					t.Fatalf("gitdep: cannot load woden.lock: %v", err)
				}
				pkg, ok := lk.Find("deplib")
				if !ok {
					t.Fatalf("gitdep: woden.lock has no entry for deplib")
				}
				// Point the lock entry at a different repository: the pin no longer
				// matches the source declared in woden.toml.
				pkg.Git = "https://example.com/other.git"
				lk.Upsert(pkg)
				if err := lk.Save(proj.LockPath()); err != nil {
					t.Fatalf("gitdep: cannot save woden.lock: %v", err)
				}
			},
			want: "no usable entry",
		},
		{
			name: "ols.json is inconsistent",
			breakIt: func(t *testing.T, proj *project.Project, repo *integrationRepo) {
				doc := map[string]any{}
				if err := json.Unmarshal([]byte(readIntegrationFile(t, proj.OLSJSONPath())), &doc); err != nil {
					t.Fatalf("gitdep: cannot parse ols.json: %v", err)
				}
				collections, _ := doc["collections"].([]any)
				kept := make([]any, 0, len(collections))
				for _, entry := range collections {
					object, _ := entry.(map[string]any)
					if object["name"] != "deps" {
						kept = append(kept, entry)
					}
				}
				doc["collections"] = kept
				data, err := json.MarshalIndent(doc, "", "  ")
				if err != nil {
					t.Fatalf("gitdep: cannot render ols.json: %v", err)
				}
				writeIntegrationFile(t, proj.OLSJSONPath(), string(data)+"\n")
			},
			want: "ols.json does not match the collections declared in woden.toml",
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			repo := newIntegrationRepo(t)
			proj, deps, stdout, _ := integrationProject(t)
			if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
				t.Fatalf("gitdep: Add failed: %v", err)
			}
			scenario.breakIt(t, proj, repo)
			before := integrationSnapshot(t, proj)
			err := deps.Sync(context.Background(), true)
			if err == nil {
				t.Fatalf("gitdep: sync --check accepted an inconsistent project")
			}
			mustContain(t, err.Error(), "woden sync --check")
			mustContain(t, stdout.String(), scenario.want)
			if after := integrationSnapshot(t, proj); after != before {
				t.Errorf("gitdep: sync --check changed the project:\nbefore:\n%s\nafter:\n%s", before, after)
			}
		})
	}
}

// TestIntegrationUpdateAdvancesDefaultBranch pins that update advances a
// default-branch dependency after the remote moved.
func TestIntegrationUpdateAdvancesDefaultBranch(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, stdout, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	advanced := repo.AdvanceTrunk(t)

	if err := deps.Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	if pkg := integrationLock(t, proj, "deplib"); pkg.Commit != advanced {
		t.Errorf("gitdep: Update left the lock at %q, want %q", pkg.Commit, advanced)
	}
	if got := integrationCommit(t, proj, "deplib"); got != advanced {
		t.Errorf("gitdep: vendor worktree is at %q, want %q", got, advanced)
	}
	mustContain(t, stdout.String(), fmt.Sprintf("updated deplib: %s -> %s", shortCommit(repo.C2), shortCommit(advanced)))
}

// TestIntegrationUpdateSkipsRevPinned pins that a rev-pinned dependency is
// skipped by an unnamed update but rejected when named explicitly.
func TestIntegrationUpdateSkipsRevPinned(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, stdout, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "pinned", Git: repo.URL, Rev: repo.C2[:7]}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	if err := deps.Add(context.Background(), AddOptions{Name: "tracked", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: second Add failed: %v", err)
	}
	advanced := repo.AdvanceTrunk(t)

	if err := deps.Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	if pkg := integrationLock(t, proj, "pinned"); pkg.Commit != repo.C2 {
		t.Errorf("gitdep: Update moved the rev-pinned lock entry: %+v", pkg)
	}
	if got := integrationCommit(t, proj, "pinned"); got != repo.C2 {
		t.Errorf("gitdep: Update moved the rev-pinned worktree to %q", got)
	}
	if pkg := integrationLock(t, proj, "tracked"); pkg.Commit != advanced {
		t.Errorf("gitdep: Update did not advance the tracked dependency: %+v", pkg)
	}
	mustContain(t, stdout.String(), "skipped pinned: pinned to revision "+repo.C2[:7])

	err := deps.Update(context.Background(), []string{"pinned"})
	if err == nil {
		t.Fatalf("gitdep: Update accepted an explicitly named rev-pinned dependency")
	}
	mustContain(t, err.Error(), "cannot be updated")
}

// TestIntegrationUpdateDirtyWorktree pins that update never overwrites local
// modifications.
func TestIntegrationUpdateDirtyWorktree(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, _, stderrOut := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	repo.AdvanceTrunk(t)
	local := filepath.Join(proj.VendorDir(), "deplib", "local.odin")
	writeIntegrationFile(t, local, "package deplib\n")

	err := deps.Update(context.Background(), nil)
	if err == nil {
		t.Fatalf("gitdep: Update overwrote a dirty worktree")
	}
	mustContain(t, err.Error(), "woden update: 1 problem(s) found")
	mustContain(t, stderrOut.String(), "vendor/deplib has local modifications; skipped")
	if _, statErr := os.Stat(local); statErr != nil {
		t.Errorf("gitdep: Update destroyed the local file: %v", statErr)
	}
	if pkg := integrationLock(t, proj, "deplib"); pkg.Commit != repo.C2 {
		t.Errorf("gitdep: Update moved the lock of a dirty dependency: %+v", pkg)
	}
}

// TestIntegrationRemoveLastDependency pins the full cleanup of the last
// dependency: worktree, lock file, manifest section, deps mapping, vendor
// directory and the managed ols.json entry.
func TestIntegrationRemoveLastDependency(t *testing.T) {
	repo := newIntegrationRepo(t)
	proj, deps, stdout, _ := integrationProject(t)
	if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}
	if err := deps.Remove(context.Background(), "deplib"); err != nil {
		t.Fatalf("gitdep: Remove failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(proj.VendorDir(), "deplib")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("gitdep: vendor/deplib still exists (err: %v)", err)
	}
	if _, err := os.Stat(proj.VendorDir()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("gitdep: vendor/ still exists (err: %v)", err)
	}
	if _, err := os.Stat(proj.LockPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("gitdep: woden.lock still exists (err: %v)", err)
	}
	cfg := integrationConfig(t, proj)
	if _, ok := cfg.DependencyByName("deplib"); ok {
		t.Errorf("gitdep: woden.toml still declares deplib")
	}
	if _, ok := cfg.DepsCollection(); ok {
		t.Errorf("gitdep: woden.toml still maps the deps collection")
	}
	if path, ok := cfg.Collections["app"]; !ok || path != "src" {
		t.Errorf("gitdep: Remove did not keep the app collection: %+v", cfg.Collections)
	}
	collections := integrationOLSCollections(t, proj)
	if _, ok := collections["deps"]; ok {
		t.Errorf("gitdep: ols.json still declares the deps collection")
	}
	if collections["app"] != "src" || collections["libs"] != "libs" {
		t.Errorf("gitdep: Remove changed the unmanaged ols.json collections: %+v", collections)
	}
	mustContain(t, stdout.String(), "removed dependency deplib")
}

// TestIntegrationRemoveRefusals pins that remove refuses to delete worktrees it
// cannot prove untouched.
func TestIntegrationRemoveRefusals(t *testing.T) {
	cases := []struct {
		name    string
		breakIt func(t *testing.T, proj *project.Project)
		want    string
	}{
		{
			name: "local modifications",
			breakIt: func(t *testing.T, proj *project.Project) {
				writeIntegrationFile(t, filepath.Join(proj.VendorDir(), "deplib", "local.odin"), "package deplib\n")
			},
			want: "local modifications",
		},
		{
			name: "origin mismatch",
			breakIt: func(t *testing.T, proj *project.Project) {
				integrationGit(t, filepath.Join(proj.VendorDir(), "deplib"), "remote", "set-url", "origin", "https://example.com/other.git")
			},
			want: "has origin",
		},
		{
			name: "local commits",
			breakIt: func(t *testing.T, proj *project.Project) {
				dir := filepath.Join(proj.VendorDir(), "deplib")
				integrationGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "--allow-empty", "-m", "local change")
			},
			want: "local commits",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newIntegrationRepo(t)
			proj, deps, _, _ := integrationProject(t)
			if err := deps.Add(context.Background(), AddOptions{Name: "deplib", Git: repo.URL}); err != nil {
				t.Fatalf("gitdep: Add failed: %v", err)
			}
			testCase.breakIt(t, proj)

			err := deps.Remove(context.Background(), "deplib")
			if err == nil {
				t.Fatalf("gitdep: Remove accepted a worktree it cannot confirm as untouched")
			}
			mustContain(t, err.Error(), testCase.want)
			if _, statErr := os.Stat(filepath.Join(proj.VendorDir(), "deplib")); statErr != nil {
				t.Errorf("gitdep: Remove deleted the refused worktree: %v", statErr)
			}
			if _, err := os.Stat(proj.LockPath()); err != nil {
				t.Errorf("gitdep: Remove deleted woden.lock when refusing: %v", err)
			}
			if cfg := integrationConfig(t, proj); cfg != nil {
				if _, ok := cfg.DependencyByName("deplib"); !ok {
					t.Errorf("gitdep: Remove deleted the manifest entry when refusing")
				}
			}
		})
	}
}

// integrationConfig reloads woden.toml from disk.
func integrationConfig(t *testing.T, proj *project.Project) *config.Config {
	t.Helper()
	cfg, _, err := config.Load(proj.ConfigPath)
	if err != nil {
		t.Fatalf("gitdep: cannot reload woden.toml: %v", err)
	}
	return cfg
}

// integrationLock returns the lock package for name, failing when it is absent.
func integrationLock(t *testing.T, proj *project.Project, name string) lock.Package {
	t.Helper()
	lk, err := lock.Load(proj.LockPath())
	if err != nil {
		t.Fatalf("gitdep: cannot load woden.lock: %v", err)
	}
	pkg, ok := lk.Find(name)
	if !ok {
		t.Fatalf("gitdep: woden.lock has no entry for %q: %+v", name, lk.Packages)
	}
	return pkg
}

// integrationOLSCollections reads the managed collections of ols.json.
func integrationOLSCollections(t *testing.T, proj *project.Project) map[string]string {
	t.Helper()
	data, err := fsx.ReadFile(proj.OLSJSONPath())
	if err != nil {
		t.Fatalf("gitdep: cannot read ols.json: %v", err)
	}
	var doc struct {
		Collections []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"collections"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("gitdep: cannot parse ols.json: %v", err)
	}
	collections := make(map[string]string, len(doc.Collections))
	for _, entry := range doc.Collections {
		collections[entry.Name] = entry.Path
	}
	return collections
}

// integrationSnapshot renders every file and directory the dependency commands
// may write, so a read-only command can be proven not to change anything.
func integrationSnapshot(t *testing.T, proj *project.Project) string {
	t.Helper()
	var builder strings.Builder
	for _, path := range []string{proj.ConfigPath, proj.LockPath(), proj.OLSJSONPath()} {
		data, err := os.ReadFile(path)
		if err != nil {
			builder.WriteString(path + ": absent\n")
			continue
		}
		builder.WriteString(path + ":\n" + string(data) + "\n")
	}
	entries, err := os.ReadDir(proj.VendorDir())
	if err != nil {
		builder.WriteString("vendor: absent\n")
		return builder.String()
	}
	for _, entry := range entries {
		builder.WriteString("vendor/" + entry.Name() + "\n")
	}
	return builder.String()
}

// readIntegrationFile reads a file that the test expects to exist.
func readIntegrationFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("gitdep: cannot read %s: %v", path, err)
	}
	return string(data)
}

// writeIntegrationFile writes a file inside the test project.
func writeIntegrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("gitdep: cannot create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("gitdep: cannot write %s: %v", path, err)
	}
}

// integrationGit runs a Git command in dir and fails the test on errors.
func integrationGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("gitdep: git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
