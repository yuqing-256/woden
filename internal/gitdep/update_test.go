package gitdep

import (
	"context"
	"testing"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/lock"
)

// TestUpdateAdvancesBranch pins the happy path: the remote branch moved, so the
// lock and the vendor worktree advance.
func TestUpdateAdvancesBranch(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitB

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}

	pkg, ok := tp.lockPackages()["deplib"]
	if !ok || pkg.Commit != testCommitB || pkg.Branch != "main" {
		t.Errorf("gitdep: Update did not advance the lock entry: %+v (exists: %v)", pkg, ok)
	}
	checkouts := fake.callsFor("checkout")
	if len(checkouts) != 1 {
		t.Fatalf("gitdep: expected one checkout, got %d", len(checkouts))
	}
	mustHaveArgs(t, checkouts[0], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitB)

	fetches := fake.callsFor("fetch")
	if len(fetches) != 1 {
		t.Fatalf("gitdep: expected one fetch, got %d", len(fetches))
	}
	mustHaveArgs(t, fetches[0], "-c", "protocol.ext.allow=never", "fetch", "--quiet", "--no-tags", "--", testGitURL, "+refs/heads/main:refs/remotes/origin/main")
	mustContain(t, tp.stdout.String(), "updated deplib: 111111111111 -> 222222222222")
}

// TestUpdateWritesLockBeforeCheckout pins the write ordering: when the checkout
// fails, the lock already records the new commit, and a later sync repairs the
// worktree.
func TestUpdateWritesLockBeforeCheckout(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitB
	fake.fail["checkout"] = fakeFailure{code: 1, message: "fatal: cannot checkout"}

	err := tp.deps(fake).Update(context.Background(), nil)
	if err == nil {
		t.Fatalf("gitdep: expected Update to fail when the checkout fails")
	}
	pkg, ok := tp.lockPackages()["deplib"]
	if !ok || pkg.Commit != testCommitB {
		t.Errorf("gitdep: Update did not record the new commit before checking out: %+v", pkg)
	}
}

// TestUpdateTagForceFetches pins that a moved tag is fetched into refs/tags.
func TestUpdateTagForceFetches(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Tag: "v0.1.0"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.resolve["refs/tags/v0.1.0^{commit}"] = testCommitC

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	fetches := fake.callsFor("fetch")
	if len(fetches) != 1 {
		t.Fatalf("gitdep: expected one fetch, got %d", len(fetches))
	}
	mustHaveArgs(t, fetches[0], "-c", "protocol.ext.allow=never", "fetch", "--quiet", "--no-tags", "--", testGitURL, "+refs/tags/v0.1.0:refs/tags/v0.1.0")
	if pkg := tp.lockPackages()["deplib"]; pkg.Commit != testCommitC || pkg.Tag != "v0.1.0" {
		t.Errorf("gitdep: unexpected lock entry after updating a tag: %+v", pkg)
	}
	mustContain(t, tp.stdout.String(), "updated deplib: 111111111111 -> 333333333333")
}

// TestUpdateDefaultBranchFetchesHEAD pins the fetch refspec of a dependency
// without a requested ref.
func TestUpdateDefaultBranchFetchesHEAD(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.resolve["FETCH_HEAD^{commit}"] = testCommitB

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	fetches := fake.callsFor("fetch")
	if len(fetches) != 1 {
		t.Fatalf("gitdep: expected one fetch, got %d", len(fetches))
	}
	mustHaveArgs(t, fetches[0], "-c", "protocol.ext.allow=never", "fetch", "--quiet", "--no-tags", "--", testGitURL, "HEAD")
	if pkg := tp.lockPackages()["deplib"]; pkg.Commit != testCommitB {
		t.Errorf("gitdep: Update did not advance the lock: %+v", pkg)
	}
}

// TestUpdateSkipsRevPinnedDependencies pins that rev-pinned dependencies are
// reported as skipped and never fetched or moved.
func TestUpdateSkipsRevPinnedDependencies(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	revDep := config.Dependency{Name: "deplib", Git: testGitURL, Rev: "abc1234"}
	branchDep := config.Dependency{Name: "otherlib", Git: testGitURL2, Branch: "main"}
	vendoredDependency(tp, fake, revDep, testCommitA, true)
	vendoredDependency(tp, fake, branchDep, testCommitB, false)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitC

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	mustContain(t, tp.stdout.String(), "skipped deplib: pinned to revision abc1234")
	packages := tp.lockPackages()
	if packages["deplib"].Commit != testCommitA || packages["deplib"].Rev != "abc1234" {
		t.Errorf("gitdep: Update moved the rev-pinned dependency: %+v", packages["deplib"])
	}
	if packages["otherlib"].Commit != testCommitC {
		t.Errorf("gitdep: Update did not advance the branch dependency: %+v", packages["otherlib"])
	}
	// The revision-pinned worktree is never touched.
	for _, call := range fake.calls {
		if call.opts.Dir == tp.proj.VendorDir()+"/deplib" || call.opts.Dir == tp.proj.VendorDir()+`\deplib` {
			if gitSubcommand(call.args) == "fetch" {
				t.Errorf("gitdep: Update fetched the rev-pinned dependency: %v", call.args)
			}
		}
	}
}

// TestUpdateNamedRevPinnedDependencyIsUserError pins that naming a rev-pinned
// dependency is an error, not a silent skip.
func TestUpdateNamedRevPinnedDependencyIsUserError(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	tp.declare(config.Dependency{Name: "deplib", Git: testGitURL, Rev: "abc1234"}, true)
	tp.writeLock(lock.Package{Name: "deplib", Git: testGitURL, Rev: "abc1234", Commit: testCommitA})

	err := tp.deps(fake).Update(context.Background(), []string{"deplib"})
	if err == nil {
		t.Fatalf("gitdep: expected Update to reject an explicitly named rev-pinned dependency")
	}
	mustContain(t, errs.Message(err), `dependency "deplib" is pinned to revision abc1234 and cannot be updated`)
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Update ran Git commands before rejecting the request: %v", fake.subcommands())
	}
}

// TestUpdateSelection pins that only the named dependency is updated.
func TestUpdateSelection(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	first := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	second := config.Dependency{Name: "otherlib", Git: testGitURL2, Branch: "main"}
	vendoredDependency(tp, fake, first, testCommitA, true)
	vendoredDependency(tp, fake, second, testCommitB, false)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitC

	if err := tp.deps(fake).Update(context.Background(), []string{"deplib"}); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	packages := tp.lockPackages()
	if packages["deplib"].Commit != testCommitC {
		t.Errorf("gitdep: Update did not update the named dependency: %+v", packages["deplib"])
	}
	if packages["otherlib"].Commit != testCommitB {
		t.Errorf("gitdep: Update touched an unnamed dependency: %+v", packages["otherlib"])
	}
	if fetches := fake.callsFor("fetch"); len(fetches) != 1 {
		t.Errorf("gitdep: expected exactly one fetch, got %d", len(fetches))
	}
}

// TestUpdateUnknownDependency pins the unknown-name error.
func TestUpdateUnknownDependency(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	tp.declare(config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}, true)

	err := tp.deps(fake).Update(context.Background(), []string{"nope"})
	if err == nil {
		t.Fatalf("gitdep: expected Update to reject an unknown dependency")
	}
	mustContain(t, errs.Message(err), `dependency "nope" is not declared in woden.toml`)
}

// TestUpdateDirtyWorktreeSkippedAndOthersProcessed pins that a dirty worktree
// is reported, never overwritten, and the remaining dependencies are updated.
func TestUpdateDirtyWorktreeSkippedAndOthersProcessed(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	first := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	second := config.Dependency{Name: "otherlib", Git: testGitURL2, Branch: "main"}
	dir := vendoredDependency(tp, fake, first, testCommitA, true)
	vendoredDependency(tp, fake, second, testCommitB, false)
	fake.setDirty(dir)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitC

	err := tp.deps(fake).Update(context.Background(), nil)
	if err == nil {
		t.Fatalf("gitdep: expected Update to fail because of the dirty worktree")
	}
	mustContain(t, errs.Message(err), "woden update: 1 problem(s) found")
	mustContain(t, tp.stderr.String(), "vendor/deplib has local modifications; skipped")
	if packages := tp.lockPackages(); packages["deplib"].Commit != testCommitA {
		t.Errorf("gitdep: Update moved the lock of the dirty dependency: %+v", packages["deplib"])
	}
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Update deleted the dirty worktree")
	}
}

// TestUpdateRestoresMissingWorktree pins that a missing worktree is restored at
// the resolved commit and the lock is advanced.
func TestUpdateRestoresMissingWorktree(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	tp.declare(dep, true)
	tp.writeLock(lock.Package{Name: dep.Name, Git: dep.Git, Branch: dep.Branch, Commit: testCommitA})
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitB

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: Update did not restore the missing worktree")
	}
	if pkg := tp.lockPackages()["deplib"]; pkg.Commit != testCommitB {
		t.Errorf("gitdep: Update did not pin the resolved commit: %+v", pkg)
	}
	mustContain(t, tp.stdout.String(), "restored deplib at 222222222222")
	if fetches := fake.callsFor("fetch"); len(fetches) != 0 {
		t.Errorf("gitdep: Update fetched instead of cloning a missing worktree: %v", fetches)
	}
}

// TestUpdateUpToDate pins that an unchanged remote is reported and nothing is
// written.
func TestUpdateUpToDate(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	dep := config.Dependency{Name: "deplib", Git: testGitURL, Branch: "main"}
	vendoredDependency(tp, fake, dep, testCommitA, true)
	fake.resolve["refs/remotes/origin/main^{commit}"] = testCommitA

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	mustContain(t, tp.stdout.String(), "deplib is up to date at 111111111111")
	if checkouts := fake.callsFor("checkout"); len(checkouts) != 0 {
		t.Errorf("gitdep: Update checked out an up-to-date dependency: %v", checkouts)
	}
	if pkg := tp.lockPackages()["deplib"]; pkg.Commit != testCommitA {
		t.Errorf("gitdep: Update moved an up-to-date lock entry: %+v", pkg)
	}
}

// TestUpdateWithoutDependencies pins the message of a project without Git
// dependencies.
func TestUpdateWithoutDependencies(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)

	if err := tp.deps(fake).Update(context.Background(), nil); err != nil {
		t.Fatalf("gitdep: Update failed: %v", err)
	}
	mustContain(t, tp.stdout.String(), "no Git dependencies to update")
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Update ran Git commands without dependencies: %v", fake.subcommands())
	}
}
