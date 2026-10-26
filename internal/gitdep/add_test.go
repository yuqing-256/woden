package gitdep

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/tomlutil"
)

// TestAddDefaultBranch pins the complete "woden add" flow for a dependency that
// tracks the remote default branch: the exact Git command lines, the lock, the
// manifest, the vendor worktree and ols.json.
func TestAddDefaultBranch(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	fake.resolve["HEAD^{commit}"] = testCommitA

	if err := tp.deps(fake).Add(context.Background(), AddOptions{Name: "deplib", Git: testGitURL}); err != nil {
		t.Fatalf("gitdep: Add failed: %v", err)
	}

	subcommands := fake.subcommands()
	if len(subcommands) != 3 || subcommands[0] != "clone" || subcommands[1] != "rev-parse" || subcommands[2] != "checkout" {
		t.Fatalf("gitdep: unexpected Git command sequence: %v", subcommands)
	}
	clone := fake.calls[0]
	mustHavePrefix(t, clone)
	mustHaveGitEnv(t, clone)
	if clone.opts.Dir != tp.root {
		t.Errorf("gitdep: clone ran in %q, want the project root %q", clone.opts.Dir, tp.root)
	}
	stage := cloneDestination(t, clone)
	if !strings.HasPrefix(filepath.Base(stage), stagingPrefix) {
		t.Errorf("gitdep: clone destination %q does not use the staging prefix", stage)
	}
	mustHaveArgs(t, clone, "-c", "protocol.ext.allow=never", "clone", "--quiet", "--no-checkout", "--", testGitURL, stage)

	resolveCall := fake.calls[1]
	mustHavePrefix(t, resolveCall)
	mustHaveGitEnv(t, resolveCall)
	if resolveCall.opts.Dir != stage {
		t.Errorf("gitdep: rev-parse ran in %q, want the staging directory %q", resolveCall.opts.Dir, stage)
	}
	mustHaveArgs(t, resolveCall, "-c", "protocol.ext.allow=never", "rev-parse", "--verify", "HEAD^{commit}")

	checkoutCall := fake.calls[2]
	mustHavePrefix(t, checkoutCall)
	if checkoutCall.opts.Dir != stage {
		t.Errorf("gitdep: checkout ran in %q, want the staging directory %q", checkoutCall.opts.Dir, stage)
	}
	mustHaveArgs(t, checkoutCall, "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitA)

	if !tp.vendorExists("deplib") {
		t.Errorf("gitdep: vendor/deplib does not exist after add")
	}
	if staging := stagingEntries(t, tp); len(staging) != 0 {
		t.Errorf("gitdep: staging directories were left behind: %v", staging)
	}

	pkg, ok := tp.lockPackages()["deplib"]
	if !ok {
		t.Fatalf("gitdep: woden.lock has no entry for deplib")
	}
	if pkg.Git != testGitURL || pkg.Commit != testCommitA {
		t.Errorf("gitdep: unexpected lock entry: %+v", pkg)
	}
	if pkg.Branch != "" || pkg.Tag != "" || pkg.Rev != "" {
		t.Errorf("gitdep: default branch dependency must not record a ref: %+v", pkg)
	}

	manifest := tp.manifest()
	mustContain(t, manifest, "[dependencies.deplib]")
	mustContain(t, manifest, "git = "+tomlutil.Quote(testGitURL))
	mustContain(t, manifest, "deps = "+tomlutil.Quote("vendor"))

	if collections := tp.olsCollections(); collections["deps"] != "vendor" {
		t.Errorf("gitdep: ols.json deps mapping = %q, want %q (collections: %v)", collections["deps"], "vendor", collections)
	}
	mustContain(t, tp.stdout.String(), `added dependency deplib (the remote default branch) at 111111111111`)
}

// TestAddRequestedRefs pins how --branch, --tag and --rev are resolved and
// recorded. A revision stays abbreviated in woden.lock; the commit is always
// full.
func TestAddRequestedRefs(t *testing.T) {
	t.Run("branch", func(t *testing.T) {
		tp := newTestProject(t)
		fake := newFakeGit(t)
		fake.resolve["refs/remotes/origin/dev^{commit}"] = testCommitB

		opts := AddOptions{Name: "deplib", Git: testGitURL, Branch: "dev"}
		if err := tp.deps(fake).Add(context.Background(), opts); err != nil {
			t.Fatalf("gitdep: Add failed: %v", err)
		}
		mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "rev-parse", "--verify", "refs/remotes/origin/dev^{commit}")
		mustHaveArgs(t, fake.calls[2], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitB)
		pkg := tp.lockPackages()["deplib"]
		if pkg.Branch != "dev" || pkg.Tag != "" || pkg.Rev != "" || pkg.Commit != testCommitB {
			t.Errorf("gitdep: unexpected lock entry: %+v", pkg)
		}
		mustContain(t, tp.manifest(), "branch = "+tomlutil.Quote("dev"))
	})

	t.Run("tag", func(t *testing.T) {
		tp := newTestProject(t)
		fake := newFakeGit(t)
		fake.resolve["refs/tags/v0.1.0^{commit}"] = testCommitC

		opts := AddOptions{Name: "deplib", Git: testGitURL, Tag: "v0.1.0"}
		if err := tp.deps(fake).Add(context.Background(), opts); err != nil {
			t.Fatalf("gitdep: Add failed: %v", err)
		}
		mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "rev-parse", "--verify", "refs/tags/v0.1.0^{commit}")
		mustHaveArgs(t, fake.calls[2], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitC)
		pkg := tp.lockPackages()["deplib"]
		if pkg.Tag != "v0.1.0" || pkg.Branch != "" || pkg.Rev != "" || pkg.Commit != testCommitC {
			t.Errorf("gitdep: unexpected lock entry: %+v", pkg)
		}
		mustContain(t, tp.manifest(), "tag = "+tomlutil.Quote("v0.1.0"))
	})

	t.Run("rev stays abbreviated", func(t *testing.T) {
		tp := newTestProject(t)
		fake := newFakeGit(t)
		fake.resolve["abc1234^{commit}"] = testCommitB

		opts := AddOptions{Name: "deplib", Git: testGitURL, Rev: "abc1234"}
		if err := tp.deps(fake).Add(context.Background(), opts); err != nil {
			t.Fatalf("gitdep: Add failed: %v", err)
		}
		mustHaveArgs(t, fake.calls[1], "-c", "protocol.ext.allow=never", "rev-parse", "--verify", "abc1234^{commit}")
		mustHaveArgs(t, fake.calls[2], "-c", "protocol.ext.allow=never", "checkout", "--quiet", "--detach", testCommitB)
		pkg := tp.lockPackages()["deplib"]
		if pkg.Rev != "abc1234" || pkg.Commit != testCommitB {
			t.Errorf("gitdep: unexpected lock entry: %+v", pkg)
		}
		mustContain(t, tp.manifest(), "rev = "+tomlutil.Quote("abc1234"))
	})
}

// TestAddRejectsDuplicateDependency pins the duplicate-name rule.
func TestAddRejectsDuplicateDependency(t *testing.T) {
	tp := newTestProject(t)
	tp.declare(config.Dependency{Name: "deplib", Git: testGitURL}, true)
	fake := newFakeGit(t)

	err := tp.deps(fake).Add(context.Background(), AddOptions{Name: "deplib", Git: testGitURL})
	if err == nil {
		t.Fatalf("gitdep: expected Add to reject the duplicate dependency")
	}
	if !errs.IsUser(err) {
		t.Errorf("gitdep: expected a user error, got %T: %v", err, err)
	}
	mustContain(t, errs.Message(err), `dependency "deplib" is already declared in woden.toml`)
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Add ran Git commands before rejecting the duplicate: %v", fake.subcommands())
	}
}

// TestAddRejectsDepsCollectionConflict pins the single deps="vendor" rule: a
// deps collection mapped anywhere else is a user error before any Git access.
func TestAddRejectsDepsCollectionConflict(t *testing.T) {
	tp := newTestProject(t)
	if err := config.Edit(tp.proj.ConfigPath, func(e *config.Editor) error {
		return e.SetCollection("deps", "libs")
	}); err != nil {
		t.Fatalf("gitdep: cannot map the deps collection: %v", err)
	}
	tp.reload()
	fake := newFakeGit(t)

	err := tp.deps(fake).Add(context.Background(), AddOptions{Name: "deplib", Git: testGitURL})
	if err == nil {
		t.Fatalf("gitdep: expected Add to reject the conflicting deps collection")
	}
	mustContain(t, errs.Message(err), `Git dependencies must be collected in "vendor"`)
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Add ran Git commands before rejecting the collection conflict: %v", fake.subcommands())
	}
}

// TestAddValidatesBeforeGit pins that malformed input never reaches Git.
func TestAddValidatesBeforeGit(t *testing.T) {
	tests := []struct {
		name string
		opts AddOptions
		want string
	}{
		{
			name: "invalid name",
			opts: AddOptions{Name: "../deplib", Git: testGitURL},
			want: "invalid dependency name",
		},
		{
			name: "invalid url",
			opts: AddOptions{Name: "deplib", Git: "ext::sh -c touch /tmp/pwned"},
			want: "invalid git URL",
		},
		{
			name: "invalid branch",
			opts: AddOptions{Name: "deplib", Git: testGitURL, Branch: "-x"},
			want: "invalid branch",
		},
		{
			name: "invalid tag",
			opts: AddOptions{Name: "deplib", Git: testGitURL, Tag: "bad tag"},
			want: "invalid tag",
		},
		{
			name: "invalid revision",
			opts: AddOptions{Name: "deplib", Git: testGitURL, Rev: "nothex"},
			want: "invalid revision",
		},
		{
			name: "branch and tag",
			opts: AddOptions{Name: "deplib", Git: testGitURL, Branch: "dev", Tag: "v0.1.0"},
			want: "at most one of --branch, --tag and --rev may be given",
		},
		{
			name: "tag and rev",
			opts: AddOptions{Name: "deplib", Git: testGitURL, Tag: "v0.1.0", Rev: testCommitA},
			want: "at most one of --branch, --tag and --rev may be given",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tp := newTestProject(t)
			fake := newFakeGit(t)
			err := tp.deps(fake).Add(context.Background(), test.opts)
			if err == nil {
				t.Fatalf("gitdep: expected Add to fail")
			}
			mustContain(t, errs.Message(err), test.want)
			if len(fake.calls) != 0 {
				t.Errorf("gitdep: Add ran Git commands for rejected input: %v", fake.subcommands())
			}
			if _, ok := tp.lock(); ok {
				t.Errorf("gitdep: Add wrote woden.lock for rejected input")
			}
		})
	}
}

// TestAddRejectsExistingWorktree pins that an unrelated vendor directory is
// never overwritten.
func TestAddRejectsExistingWorktree(t *testing.T) {
	tp := newTestProject(t)
	tp.makeVendor("deplib", "version() -> string {return \"local\"}")
	fake := newFakeGit(t)

	err := tp.deps(fake).Add(context.Background(), AddOptions{Name: "deplib", Git: testGitURL})
	if err == nil {
		t.Fatalf("gitdep: expected Add to reject the existing worktree")
	}
	mustContain(t, errs.Message(err), "already exists")
	if len(fake.calls) != 0 {
		t.Errorf("gitdep: Add ran Git commands before rejecting the existing worktree: %v", fake.subcommands())
	}
}

// TestAddCloneFailureLeavesNoTrace pins that a failed clone leaves neither a
// lock entry nor a staging directory behind.
func TestAddCloneFailureLeavesNoTrace(t *testing.T) {
	tp := newTestProject(t)
	fake := newFakeGit(t)
	fake.fail["clone"] = fakeFailure{code: 128, message: "fatal: repository not found"}

	err := tp.deps(fake).Add(context.Background(), AddOptions{Name: "deplib", Git: testGitURL})
	if err == nil {
		t.Fatalf("gitdep: expected Add to fail when the clone fails")
	}
	mustContain(t, errs.Message(err), "git clone failed")
	mustContain(t, errs.Message(err), "repository not found")
	if _, ok := tp.lock(); ok {
		t.Errorf("gitdep: a failed clone wrote woden.lock")
	}
	mustNotContain(t, tp.manifest(), "[dependencies.deplib]")
	if staging := stagingEntries(t, tp); len(staging) != 0 {
		t.Errorf("gitdep: a failed clone left staging directories behind: %v", staging)
	}
}

// stagingEntries lists the leftover staging directories below vendor/.
func stagingEntries(t *testing.T, tp *testProject) []string {
	t.Helper()
	entries, err := os.ReadDir(tp.proj.VendorDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("gitdep: cannot read vendor/: %v", err)
	}
	var staging []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingPrefix) {
			staging = append(staging, entry.Name())
		}
	}
	return staging
}
