package gitdep

import (
	"context"
	"testing"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/lock"
	"woden/internal/ols"
)

// vendoredDependency declares dep with the deps collection, pins commit in
// woden.lock and creates a matching clean worktree in vendor/.
func vendoredDependency(tp *testProject, fake *fakeGit, dep config.Dependency, commit string, withDepsCollection bool) string {
	tp.t.Helper()
	tp.declare(dep, withDepsCollection)
	pkg := lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Tag: dep.Tag, Rev: dep.Rev, Commit: commit}
	lk := lock.New()
	if existing, ok := tp.lock(); ok {
		for _, pkg := range existing.Packages {
			lk.Upsert(pkg)
		}
	}
	lk.Upsert(pkg)
	if err := lk.Save(tp.proj.LockPath()); err != nil {
		tp.t.Fatalf("gitdep: cannot write woden.lock: %v", err)
	}
	dir := tp.makeVendor(dep.Name, "version() -> string {return \"deplib\"}")
	fake.setHead(dir, commit)
	fake.setOrigin(dir, dep.Git)
	tp.reload()
	return dir
}

// syncOLS reconciles ols.json before an operation that expects the deps entry
// to be present.
func syncOLS(tp *testProject) {
	tp.t.Helper()
	if _, _, err := ols.Sync(tp.root, tp.proj.Config.Collections); err != nil {
		tp.t.Fatalf("gitdep: cannot prepare ols.json: %v", err)
	}
}

// TestRemoveLastDependencyCleansEverything pins the full cleanup of the last
// dependency: worktree, lock file, manifest section, deps mapping, empty vendor
// directory and the ols.json deps entry.
func TestRemoveLastDependencyCleansEverything(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	syncOLS(tp)
	if collections := tp.olsCollections(); collections["deps"] != "vendor" {
		t.Fatalf("gitdep: test setup did not add the deps entry: %v", collections)
	}

	if err := tp.deps(fake).Remove(context.Background(), "deplib"); err != nil {
		t.Fatalf("gitdep: Remove failed: %v", err)
	}

	if tp.vendorExists("deplib") {
		t.Errorf("gitdep: vendor/deplib still exists after remove")
	}
	if tp.vendorDirExists() {
		t.Errorf("gitdep: vendor/ still exists after removing the last dependency")
	}
	if _, ok := tp.lock(); ok {
		t.Errorf("gitdep: woden.lock still exists after removing the last dependency")
	}
	manifest := tp.manifest()
	mustNotContain(t, manifest, "[dependencies")
	mustNotContain(t, manifest, "deps = "+tomlutilQuote("vendor"))
	mustContain(t, manifest, "app = "+tomlutilQuote("src"))
	if collections := tp.olsCollections(); collections["deps"] != "" {
		t.Errorf("gitdep: ols.json still maps deps: %v", collections)
	}
	if collections := tp.olsCollections(); collections["app"] != "src" || collections["libs"] != "libs" {
		t.Errorf("gitdep: remove dropped unrelated ols.json collections: %v", collections)
	}
	mustContain(t, tp.stdout.String(), "removed dependency deplib")

	subcommands := fake.subcommands()
	want := []string{"rev-parse", "remote", "status", "rev-parse"}
	if len(subcommands) != len(want) {
		t.Fatalf("gitdep: unexpected ownership probe sequence: %v", subcommands)
	}
	for i := range want {
		if subcommands[i] != want[i] {
			t.Fatalf("gitdep: unexpected ownership probe sequence: %v", subcommands)
		}
	}
	mustHaveArgs(t, fake.calls[0], "-c", "protocol.ext.allow=never", "rev-parse", "--git-dir")
	mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "remote", "get-url", "--", "origin")
	mustHaveArgs(t, fake.calls[2], "-c", "protocol.ext.allow=never", "status", "--porcelain", "--untracked-files=normal")
	fake.assertNoRemoteCommands(t)
}

// TestRemoveKeepsOtherDependencies pins that removing one dependency keeps the
// lock entry, worktree and deps mapping of the remaining ones.
func TestRemoveKeepsOtherDependencies(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	first := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	second := config.Dependency{Name: "otherlib", Git: testGitURL2, Tag: "v0.2.0"}
	vendoredDependency(tp, fake, first, testCommitA, true)
	vendoredDependency(tp, fake, second, testCommitB, false)
	syncOLS(tp)

	if err := tp.deps(fake).Remove(context.Background(), "deplib"); err != nil {
		t.Fatalf("gitdep: Remove failed: %v", err)
	}

	if tp.vendorExists("deplib") {
		t.Errorf("gitdep: vendor/deplib still exists after remove")
	}
	if !tp.vendorExists("otherlib") {
		t.Errorf("gitdep: Remove deleted the unrelated worktree vendor/otherlib")
	}
	if !tp.vendorDirExists() {
		t.Errorf("gitdep: Remove deleted vendor/ while a dependency remains")
	}
	packages := tp.lockPackages()
	if _, ok := packages["deplib"]; ok {
		t.Errorf("gitdep: woden.lock still pins deplib: %+v", packages["deplib"])
	}
	if pkg, ok := packages["otherlib"]; !ok || pkg.Commit != testCommitB {
		t.Errorf("gitdep: woden.lock lost the other dependency: %+v", packages)
	}
	manifest := tp.manifest()
	mustNotContain(t, manifest, "[dependencies.deplib]")
	mustContain(t, manifest, "[dependencies.otherlib]")
	mustContain(t, manifest, "deps = "+tomlutilQuote("vendor"))
	if collections := tp.olsCollections(); collections["deps"] != "vendor" {
		t.Errorf("gitdep: Remove dropped the deps mapping while a dependency remains: %v", collections)
	}
}

// TestRemoveRefusesDirtyWorktree pins that local modifications block the
// removal instead of being destroyed.
func TestRemoveRefusesDirtyWorktree(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	dir := vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.setDirty(dir)

	err := tp.deps(fake).Remove(context.Background(), "deplib")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to refuse the dirty worktree")
	}
	mustContain(t, errs.Message(err), "local modifications; commit or discard them first")
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Remove deleted the dirty worktree")
	}
	if _, ok := tp.lockPackages()["deplib"]; !ok {
		t.Errorf("gitdep: Remove dropped the lock entry of a refused dependency")
	}
	mustContain(t, tp.manifest(), "[dependencies.deplib]")
}

// TestRemoveRefusesOriginMismatch pins the ownership check on the origin URL.
func TestRemoveRefusesOriginMismatch(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	dir := vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.setOrigin(dir, testGitURL2)

	err := tp.deps(fake).Remove(context.Background(), "deplib")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to refuse the foreign worktree")
	}
	mustContain(t, errs.Message(err), "has origin")
	mustContain(t, errs.Message(err), testGitURL2)
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Remove deleted a worktree with a different origin")
	}
	if _, ok := tp.lockPackages()["deplib"]; !ok {
		t.Errorf("gitdep: Remove dropped the lock entry of a refused dependency")
	}
}

// TestRemoveRefusesLocalCommits pins that a worktree checked out at another
// commit than the locked one is never removed.
func TestRemoveRefusesLocalCommits(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	dir := vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.setHead(dir, testCommitB)

	err := tp.deps(fake).Remove(context.Background(), "deplib")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to refuse the worktree with local commits")
	}
	mustContain(t, errs.Message(err), "local commits")
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Remove deleted a worktree with local commits")
	}
}

// TestRemoveRefusesNotAWorktree pins that a plain directory in vendor/ is
// reported instead of deleted.
func TestRemoveRefusesNotAWorktree(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	tp.declare(config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}, true)
	tp.makeVendor("deplib", "")

	err := tp.deps(fake).Remove(context.Background(), "deplib")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to refuse a plain directory")
	}
	mustContain(t, errs.Message(err), "is not a Git worktree")
}

// TestRemoveRefusesMissingLockEntry pins that ownership cannot be proven
// without a lock entry.
func TestRemoveRefusesMissingLockEntry(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	tp.declare(config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}, true)
	dir := tp.makeVendor("deplib", "")
	fake.setHead(dir, testCommitA)
	fake.setOrigin(dir, testGitURL)

	err := tp.deps(fake).Remove(context.Background(), "deplib")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to refuse without a lock entry")
	}
	mustContain(t, errs.Message(err), "cannot be confirmed as the untouched worktree")
}

// TestRemoveMissingWorktree pins that remove repairs a state without a
// worktree.
func TestRemoveMissingWorktree(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	tp.declare(dep, true)
	tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})

	if err := tp.deps(fake).Remove(context.Background(), "deplib"); err != nil {
		t.Fatalf("gitdep: Remove failed without a worktree: %v", err)
	}
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Remove ran Git commands without a worktree: %v", fake.subcommands())
	}
	if _, ok := tp.lock(); ok {
		t.Errorf("gitdep: woden.lock still exists after removing the last dependency")
	}
	mustNotContain(t, tp.manifest(), "[dependencies")
}

// TestRemoveUnknownDependency pins the unknown-name error and that no Git
// command runs for it.
func TestRemoveUnknownDependency(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)

	err := tp.deps(fake).Remove(context.Background(), "nope")
	if err == nil {
		t.Fatalf("gitdep: expected Remove to reject an unknown dependency")
	}
	mustContain(t, errs.Message(err), `dependency "nope" is not declared in woden.toml`)
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Remove ran Git commands for an unknown dependency: %v", fake.subcommands())
	}
}
