package gitdep

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/lock"
	"github.com/yuqing-256/woden/internal/project"
)

// TestSyncRestoresMissingVendorFromLock pins that sync restores a deleted
// worktree at the locked commit and does not advance the lock.
func TestSyncRestoresMissingVendorFromLock(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	tp.declare(dep, true)
	tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}

	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Sync did not restore vendor/deplib")
	}
	if staging := stagingEntries(t, tp); len(staging) != 0 {
		t.Errorf("gitdep: Sync left staging directories behind: %v", staging)
	}
	pkg, ok := tp.lockPackages()["deplib"]
	if !ok || pkg.Commit != testCommitA {
		t.Errorf("gitdep: Sync changed the locked commit: %+v (exists: %v)", pkg, ok)
	}
	mustContain(t, tp.stdout.String(), "restored deplib at 111111111111")
	mustContain(t, tp.manifest(), "deps = "+tomlutilQuote("vendor"))
	if collections := tp.olsCollections(); collections["deps"] != "vendor" {
		t.Errorf("gitdep: Sync did not reconcile ols.json: %v", collections)
	}

	subcommands := fake.subcommands()
	if len(subcommands) != 2 || subcommands[0] != "clone" || subcommands[1] != "checkout" {
		t.Fatalf("gitdep: unexpected Git command sequence: %v", subcommands)
	}
	mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitA)
}

// TestSyncResolvesMissingLockEntry pins that sync creates a lock entry from the
// declared ref when the lock does not pin the dependency yet.
func TestSyncResolvesMissingLockEntry(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	fake.resolve["refs/remotes/origin/dev^{commit}"] = testCommitC
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "dev"}
	tp.declare(dep, true)

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}

	pkg, ok := tp.lockPackages()["deplib"]
	if !ok {
		t.Fatalf("gitdep: Sync did not create a lock entry")
	}
	if pkg.Branch != "dev" || pkg.Commit != testCommitC || pkg.Git != testGitURL {
		t.Errorf("gitdep: unexpected lock entry: %+v", pkg)
	}
	mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "rev-parse", "--verify", "refs/remotes/origin/dev^{commit}")
	mustHaveArgs(t, fake.calls[2], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitC)
	mustContain(t, tp.stdout.String(), "restored deplib at 333333333333")
}

// TestSyncRepinsLockEntryThatDisagreesWithManifest pins that a lock entry whose
// requested ref changed in woden.toml is resolved again and the worktree is
// checked out at the new commit.
func TestSyncRepinsLockEntryThatDisagreesWithManifest(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "dev"}
	tp.declare(dep, true)
	dir := tp.makeVendor(dep.Name, "version() -> string {return \"deplib\"}")
	fake.setHead(dir, testCommitA)
	fake.setOrigin(dir, dep.Git)
	// The lock still pins the previous branch (main).
	tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: "main", Commit: testCommitA})
	fake.resolve["refs/remotes/origin/dev^{commit}"] = testCommitC

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}

	pkg, ok := tp.lockPackages()["deplib"]
	if !ok || pkg.Commit != testCommitC || pkg.Branch != "dev" {
		t.Errorf("gitdep: Sync did not repin the lock entry: %+v (exists: %v)", pkg, ok)
	}
	mustContain(t, tp.stdout.String(), "pinned deplib to 333333333333")
	mustContain(t, tp.stdout.String(), "checked out deplib at 333333333333")
	fetchCalls := fake.callsFor("fetch")
	if len(fetchCalls) != 1 {
		t.Fatalf("gitdep: expected one fetch, got %d", len(fetchCalls))
	}
	mustHaveArgs(t, fetchCalls[0], "-c", "protocol.ext.allow=never", "fetch", "--quiet", "--no-tags", "--", testGitURL, "+refs/heads/dev:refs/remotes/origin/dev")
}

// TestSyncDoesNotAdvanceLockedCommit pins that a consistent lock is left alone:
// sync never advances commits.
func TestSyncDoesNotAdvanceLockedCommit(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}

	pkg, ok := tp.lockPackages()["deplib"]
	if !ok || pkg.Commit != testCommitA {
		t.Errorf("gitdep: Sync moved the locked commit: %+v", pkg)
	}
	mustContain(t, tp.stdout.String(), "synced 1 dependencies")
	for _, call := range fake.calls {
		switch gitSubcommand(call.args) {
		case "clone", "fetch", "checkout":
			t.Errorf("gitdep: Sync issued an unexpected command: %v", call.args)
		}
	}
}

// TestSyncDirtyWorktreeSkippedOthersProcessed pins that one dirty worktree does
// not stop the remaining dependencies from being restored and fails the command
// at the end.
func TestSyncDirtyWorktreeSkippedOthersProcessed(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	first := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	second := config.Dependency{Name: "otherlib", Git: testGitURL2, Branch: "main"}
	dir := vendoredDependency(tp, fake, first, testCommitA, true)
	secondDir := vendoredDependency(tp, fake, second, testCommitB, false)
	fake.setDirty(dir)
	// The second worktree is missing: sync must restore it from the lock.
	if err := os.RemoveAll(secondDir); err != nil {
		t.Fatalf("gitdep: cannot delete the second test worktree: %v", err)
	}

	err := tp.deps(fake).Sync(context.Background(), false)
	if err == nil {
		t.Fatalf("gitdep: expected Sync to fail because of the dirty worktree")
	}
	if !errs.IsUser(err) {
		t.Errorf("gitdep: expected a user error, got %T: %v", err, err)
	}
	mustContain(t, errs.Message(err), "woden sync: 1 problem(s) found")
	mustContain(t, tp.stderr.String(), "vendor/deplib has local modifications; skipped")
	mustContain(t, tp.stdout.String(), "restored otherlib at 222222222222")
	if pkg, ok := tp.lockPackages()["deplib"]; !ok || pkg.Commit != testCommitA {
		t.Errorf("gitdep: Sync changed the lock of the dirty dependency: %+v", pkg)
	}
}

// TestSyncNoDependenciesCleansState pins that sync removes dependency-only
// state once the last dependency is gone: lock file, deps mapping and the empty
// vendor directory.
func TestSyncNoDependenciesCleansState(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	if err := config.Edit(tp.proj.ConfigPath, func(e *config.Editor) error {
		return e.SetCollection(project.DepsCollection, project.VendorDir)
	}); err != nil {
		t.Fatalf("gitdep: cannot map the deps collection: %v", err)
	}
	tp.reload()
	tp.writeLock(lock.Package{Name: "ghost", Git: testGitURL, Branch: "main", Commit: testCommitA})
	staging := filepath.Join(tp.proj.VendorDir(), stagingPrefix+"deadbeefdeadbeef")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatalf("gitdep: cannot create the staging stand-in: %v", err)
	}

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}

	if _, ok := tp.lock(); ok {
		t.Errorf("gitdep: Sync did not remove woden.lock without dependencies")
	}
	mustNotContain(t, tp.manifest(), "deps = "+tomlutilQuote("vendor"))
	if tp.vendorDirExists() {
		t.Errorf("gitdep: Sync did not remove the empty vendor directory")
	}
	mustContain(t, tp.stdout.String(), "no Git dependencies")
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Sync ran Git commands without dependencies: %v", fake.subcommands())
	}
}

// TestSyncAddsMissingDepsCollection pins that sync repairs a manifest that
// declares dependencies but misses the deps collection mapping.
func TestSyncAddsMissingDepsCollection(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	tp.declare(dep, false)
	tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})

	if err := tp.deps(fake).Sync(context.Background(), false); err != nil {
		t.Fatalf("gitdep: Sync failed: %v", err)
	}
	mustContain(t, tp.manifest(), "deps = "+tomlutilQuote("vendor"))
	mustContain(t, tp.stdout.String(), `added collection "deps" -> "vendor" to woden.toml`)
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Sync did not restore the worktree after repairing the manifest")
	}
}

// TestCheckPassesOnCleanState pins that "sync --check" succeeds on a consistent
// project without writing anything or touching the network.
func TestCheckPassesOnCleanState(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	syncOLS(tp)
	before := tp.snapshot()

	if err := tp.deps(fake).Check(context.Background()); err != nil {
		t.Fatalf("gitdep: Check failed on a clean project: %v\nstdout:\n%s\nstderr:\n%s", err, tp.stdout.String(), tp.stderr.String())
	}
	if after := tp.snapshot(); after != before {
		t.Errorf("gitdep: Check modified the project state\nafter:\n%s\nbefore:\n%s", after, before)
	}
	fake.assertNoRemoteCommands(t)
	for _, call := range fake.calls {
		if gitSubcommand(call.args) == "checkout" {
			t.Errorf("gitdep: Check checked out a commit: %v", call.args)
		}
	}
}

// TestCheckReportsProblems pins that "sync --check" reports every inconsistency
// and fails, without writing or touching the network.
func TestCheckReportsProblems(t *testing.T) {
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	tests := []struct {
		name  string
		setup func(tp *testProject, fake *fakeGit)
		want  string
	}{
		{
			name: "missing vendor worktree",
			setup: func(tp *testProject, fake *fakeGit) {
				tp.declare(dep, true)
				tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})
				syncOLS(tp)
			},
			want: "vendor/deplib is missing; run `woden sync`",
		},
		{
			name: "dirty worktree",
			setup: func(tp *testProject, fake *fakeGit) {
				dir := vendoredDependency(tp, fake, dep, testCommitA, true)
				syncOLS(tp)
				fake.setDirty(dir)
			},
			want: "vendor/deplib has local modifications",
		},
		{
			name: "wrong checked out commit",
			setup: func(tp *testProject, fake *fakeGit) {
				dir := vendoredDependency(tp, fake, dep, testCommitA, true)
				syncOLS(tp)
				fake.setHead(dir, testCommitB)
			},
			want: "vendor/deplib is checked out at 222222222222 but woden.lock pins 111111111111",
		},
		{
			name: "lock entry disagrees with manifest",
			setup: func(tp *testProject, fake *fakeGit) {
				devDep := config.Dependency{Name: dep.Name, Git: dep.Git, Branch: "dev"}
				tp.declare(devDep, true)
				dir := tp.makeVendor(dep.Name, "version() -> string {return \"deplib\"}")
				fake.setHead(dir, testCommitA)
				fake.setOrigin(dir, dep.Git)
				tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: "main", Commit: testCommitA})
				syncOLS(tp)
			},
			want: `woden.lock has no usable entry for "deplib" (declared as branch dev)`,
		},
		{
			name: "missing lock file",
			setup: func(tp *testProject, fake *fakeGit) {
				tp.declare(dep, true)
				syncOLS(tp)
			},
			want: "woden.lock is missing",
		},
		{
			name: "missing deps collection",
			setup: func(tp *testProject, fake *fakeGit) {
				tp.declare(dep, false)
				dir := tp.makeVendor(dep.Name, "version() -> string {return \"deplib\"}")
				fake.setHead(dir, testCommitA)
				fake.setOrigin(dir, dep.Git)
				tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})
			},
			want: `woden.toml declares dependencies but does not map collection "deps" to "vendor"`,
		},
		{
			name: "stale lock entry",
			setup: func(tp *testProject, fake *fakeGit) {
				tp.declare(dep, true)
				dir := tp.makeVendor(dep.Name, "version() -> string {return \"deplib\"}")
				fake.setHead(dir, testCommitA)
				fake.setOrigin(dir, dep.Git)
				tp.writeLock(
					lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA},
					lock.Package{Name: "ghost", Git: testGitURL2, Branch: "main", Commit: testCommitB},
				)
				syncOLS(tp)
			},
			want: `woden.lock pins "ghost", which woden.toml does not declare`,
		},
		{
			name: "undeclared worktree",
			setup: func(tp *testProject, fake *fakeGit) {
				tp.makeVendor("leftover", "")
			},
			want: "vendor/leftover is not declared in woden.toml",
		},
		{
			name: "ols json is inconsistent",
			setup: func(tp *testProject, fake *fakeGit) {
				vendoredDependency(tp, fake, dep, testCommitA, true)
			},
			want: "ols.json does not match the collections declared in woden.toml; run `woden sync`",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tp := newTestProject(t)
			fake := newFakeGit(t)
			test.setup(tp, fake)
			before := tp.snapshot()

			err := tp.deps(fake).Check(context.Background())
			if err == nil {
				t.Fatalf("gitdep: expected Check to fail\nstdout:\n%s", tp.stdout.String())
			}
			if !errs.IsUser(err) {
				t.Errorf("gitdep: expected a user error, got %T: %v", err, err)
			}
			mustContain(t, errs.Message(err), "woden sync --check")
			mustContain(t, tp.stdout.String(), test.want)
			if after := tp.snapshot(); after != before {
				t.Errorf("gitdep: Check modified the project state\nafter:\n%s\nbefore:\n%s", after, before)
			}
			fake.assertNoRemoteCommands(t)
		})
	}
}
