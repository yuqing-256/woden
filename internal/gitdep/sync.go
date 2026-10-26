package gitdep

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/lock"
	"woden/internal/ols"
	"woden/internal/project"
	"woden/internal/validate"
)

// Sync reconciles the vendor trees, woden.lock and ols.json with woden.toml.
//
// woden.toml is the source of truth for the set of dependencies; woden.lock
// pins the commits. A normal sync restores missing or incorrect worktrees from
// the lock (it never advances commits), fixes lock entries that disagree with
// the manifest, and never overwrites a worktree with local modifications: such
// worktrees are reported, the remaining dependencies are still processed, and
// the command fails at the end.
//
// Write order (reentrant):
//
//  1. woden.toml gets the deps collection mapping when dependencies exist.
//  2. Stale lock entries and stale staging directories are removed.
//  3. For every dependency the lock entry is repaired first, then the vendor
//     worktree is restored or checked out.
//  4. ols.json is reconciled last.
func (d *Deps) Sync(ctx context.Context, checkOnly bool) error {
	if d.Project.Config == nil {
		return errs.Userf("project configuration is not loaded")
	}
	if checkOnly {
		return d.Check(ctx)
	}
	deps := d.Project.Config.SortedDependencies()
	depsCollection, hasDepsCollection := d.Project.Config.DepsCollection()
	if hasDepsCollection && depsCollection != project.VendorDir {
		return errs.Userf("collection %q maps to %q, but Git dependencies must be collected in %q; edit woden.toml first",
			project.DepsCollection, depsCollection, project.VendorDir)
	}
	if len(deps) == 0 {
		return d.syncNoDependencies(ctx)
	}
	if !hasDepsCollection {
		if err := config.Edit(d.Project.ConfigPath, func(e *config.Editor) error {
			return e.SetCollection(project.DepsCollection, project.VendorDir)
		}); err != nil {
			return err
		}
		if err := d.reload(); err != nil {
			return err
		}
		fmt.Fprintf(d.out(), "added collection %q -> %q to woden.toml\n", project.DepsCollection, project.VendorDir)
	}
	if err := d.cleanStaging(); err != nil {
		return err
	}
	lk, err := d.loadLock()
	if err != nil {
		return err
	}
	declared := make(map[string]bool, len(deps))
	for _, dep := range deps {
		declared[dep.Name] = true
	}
	stale := false
	for _, pkg := range lk.Sorted() {
		if declared[pkg.Name] {
			continue
		}
		lk.Remove(pkg.Name)
		stale = true
		fmt.Fprintf(d.out(), "removed stale lock entry for %s\n", pkg.Name)
	}
	if stale {
		if err := lk.Save(d.Project.LockPath()); err != nil {
			return err
		}
	}
	problems := &problem{w: d.errOut(), what: "woden sync"}
	for _, dep := range deps {
		if err := d.ensureVendor(ctx, dep, lk, problems); err != nil {
			return err
		}
	}
	if _, err := d.updateOLS(); err != nil {
		return err
	}
	if problems.n == 0 {
		fmt.Fprintf(d.out(), "synced %d dependencies\n", len(deps))
	}
	return problems.err()
}

// syncNoDependencies removes the state that only exists for Git dependencies.
func (d *Deps) syncNoDependencies(ctx context.Context) error {
	if _, ok := d.Project.Config.DepsCollection(); ok {
		if err := config.Edit(d.Project.ConfigPath, func(e *config.Editor) error {
			return e.RemoveCollection(project.DepsCollection)
		}); err != nil {
			return err
		}
		if err := d.reload(); err != nil {
			return err
		}
		fmt.Fprintf(d.out(), "removed collection %q from woden.toml\n", project.DepsCollection)
	}
	lockPath := d.Project.LockPath()
	if err := os.Remove(lockPath); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return errs.WrapUser(err, "cannot remove %s", lockPath)
		}
	} else {
		fmt.Fprintf(d.out(), "removed %s\n", lock.FileName)
	}
	if err := d.cleanStaging(); err != nil {
		return err
	}
	if err := d.removeEmptyVendorDir(); err != nil {
		return err
	}
	if _, err := d.updateOLS(); err != nil {
		return err
	}
	fmt.Fprintln(d.out(), "no Git dependencies")
	return nil
}

// ensureVendor makes one dependency's lock entry and vendor worktree match the
// manifest and the lock. Problems that must not be repaired automatically are
// recorded on problems.
func (d *Deps) ensureVendor(ctx context.Context, dep config.Dependency, lk *lock.Lock, problems *problem) error {
	vendorPath := d.vendorPath(dep.Name)
	present, err := exists(vendorPath)
	if err != nil {
		return err
	}
	spec := refFromConfig(dep)
	locked, hasLock := matchingLock(lk, dep)
	if !present {
		// Restore from the lock, or resolve the declared ref when the lock
		// entry is missing or disagrees with the manifest.
		stage, err := d.stagingDir()
		if err != nil {
			return err
		}
		moved := false
		defer func() {
			if !moved {
				removeStaging(stage)
			}
		}()
		if err := d.clone(ctx, dep.Git, stage); err != nil {
			return err
		}
		commit := ""
		if hasLock {
			commit = locked.Commit
		} else {
			commit, err = d.resolveCommit(ctx, stage, spec.cloneRefExpression())
			if err != nil {
				return errs.WrapUser(err, "cannot resolve %s of %s", spec.describe(), dep.Git)
			}
			lk.Upsert(packageFor(dep, commit))
			if err := lk.Save(d.Project.LockPath()); err != nil {
				return err
			}
		}
		if err := d.checkoutCommit(ctx, stage, dep.Git, commit); err != nil {
			return err
		}
		if err := os.Rename(stage, vendorPath); err != nil {
			return errs.WrapUser(err, "cannot move the staged clone to %s", vendorRel(dep.Name))
		}
		moved = true
		fmt.Fprintf(d.out(), "restored %s at %s\n", dep.Name, shortCommit(commit))
		return nil
	}

	// The worktree exists. Never touch one that is not a clean clone of the
	// declared dependency.
	worktree, err := d.isWorktree(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !worktree {
		problems.addf("%s is not a Git worktree; skipped", vendorRel(dep.Name))
		return nil
	}
	origin, hasOrigin, err := d.originURL(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !hasOrigin {
		problems.addf("%s has no origin remote; skipped", vendorRel(dep.Name))
		return nil
	}
	if origin != dep.Git {
		problems.addf("%s has origin %q but woden.toml declares %q; skipped", vendorRel(dep.Name), origin, dep.Git)
		return nil
	}
	clean, err := d.isClean(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !clean {
		problems.addf("%s has local modifications; skipped", vendorRel(dep.Name))
		return nil
	}
	commit := ""
	if hasLock {
		commit = locked.Commit
	} else {
		commit, err = d.fetch(ctx, vendorPath, dep.Git, spec)
		if err != nil {
			return err
		}
		lk.Upsert(packageFor(dep, commit))
		if err := lk.Save(d.Project.LockPath()); err != nil {
			return err
		}
		fmt.Fprintf(d.out(), "pinned %s to %s\n", dep.Name, shortCommit(commit))
	}
	head, err := d.head(ctx, vendorPath)
	if err != nil {
		return err
	}
	if head != commit {
		if err := d.checkoutCommit(ctx, vendorPath, dep.Git, commit); err != nil {
			return err
		}
		fmt.Fprintf(d.out(), "checked out %s at %s\n", dep.Name, shortCommit(commit))
	}
	return nil
}

// checkoutCommit checks out commit in the worktree at dir. When the commit is
// not present locally (a tag or branch was moved on the remote), it is fetched
// explicitly once before giving up.
func (d *Deps) checkoutCommit(ctx context.Context, dir, url, commit string) error {
	err := d.checkout(ctx, dir, commit)
	if err == nil {
		return nil
	}
	if _, ferr := d.runGit(ctx, dir, "fetch", "--quiet", "--no-tags", "--", url, commit); ferr == nil {
		if err = d.checkout(ctx, dir, commit); err == nil {
			return nil
		}
	}
	return errs.Userf("cannot check out the locked commit %s: %v", shortCommit(commit), err)
}

// Check implements "woden sync --check": it verifies the manifest, the lock,
// the vendor worktrees and ols.json without touching the network and without
// writing anything.
func (d *Deps) Check(ctx context.Context) error {
	if d.Project.Config == nil {
		return errs.Userf("project configuration is not loaded")
	}
	deps := d.Project.Config.SortedDependencies()
	depsCollection, hasDepsCollection := d.Project.Config.DepsCollection()
	problems := &problem{w: d.out(), what: "woden sync --check"}
	if hasDepsCollection && depsCollection != project.VendorDir {
		problems.addf("collection %q maps to %q, but Git dependencies must be collected in %q", project.DepsCollection, depsCollection, project.VendorDir)
	}
	if len(deps) > 0 && !hasDepsCollection {
		problems.addf("woden.toml declares dependencies but does not map collection %q to %q", project.DepsCollection, project.VendorDir)
	}
	if len(deps) == 0 && hasDepsCollection {
		problems.addf("woden.toml maps collection %q but declares no dependencies; run `woden sync`", project.DepsCollection)
	}
	lk, err := d.loadLock()
	if err != nil {
		return err
	}
	if _, err := os.Stat(d.Project.LockPath()); err != nil {
		if len(deps) > 0 {
			problems.addf("%s is missing", lock.FileName)
		}
		lk = lock.New()
	}
	declared := make(map[string]bool, len(deps))
	for _, dep := range deps {
		declared[dep.Name] = true
	}
	for _, pkg := range lk.Sorted() {
		if !declared[pkg.Name] {
			problems.addf("%s pins %q, which woden.toml does not declare", lock.FileName, pkg.Name)
		}
	}
	for _, dep := range deps {
		spec := refFromConfig(dep)
		locked, hasLock := matchingLock(lk, dep)
		if !hasLock {
			problems.addf("%s has no usable entry for %q (declared as %s)", lock.FileName, dep.Name, spec.describe())
			continue
		}
		vendorPath := d.vendorPath(dep.Name)
		present, err := exists(vendorPath)
		if err != nil {
			return err
		}
		if !present {
			problems.addf("%s is missing; run `woden sync`", vendorRel(dep.Name))
			continue
		}
		worktree, err := d.isWorktree(ctx, vendorPath)
		if err != nil {
			return err
		}
		if !worktree {
			problems.addf("%s is not a Git worktree; run `woden sync` after removing it", vendorRel(dep.Name))
			continue
		}
		origin, hasOrigin, err := d.originURL(ctx, vendorPath)
		if err != nil {
			return err
		}
		if !hasOrigin {
			problems.addf("%s has no origin remote", vendorRel(dep.Name))
		} else if origin != dep.Git {
			problems.addf("%s has origin %q but woden.toml declares %q", vendorRel(dep.Name), origin, dep.Git)
		}
		clean, err := d.isClean(ctx, vendorPath)
		if err != nil {
			return err
		}
		if !clean {
			problems.addf("%s has local modifications", vendorRel(dep.Name))
		}
		head, err := d.head(ctx, vendorPath)
		if err != nil {
			return err
		}
		if head != locked.Commit {
			problems.addf("%s is checked out at %s but %s pins %s", vendorRel(dep.Name), shortCommit(head), lock.FileName, shortCommit(locked.Commit))
		}
	}
	entries, err := d.vendorEntries()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !declared[entry] {
			problems.addf("%s is not declared in woden.toml", vendorRel(entry))
		}
	}
	consistent, exists, err := ols.Check(d.Project.Root, d.Project.Config.Collections)
	if err != nil {
		return err
	}
	if exists && !consistent {
		problems.addf("%s does not match the collections declared in woden.toml; run `woden sync`", ols.FileName)
	}
	return problems.err()
}

// matchingLock returns the lock entry of dep when it agrees with the manifest
// and pins a full commit ID.
func matchingLock(lk *lock.Lock, dep config.Dependency) (lock.Package, bool) {
	pkg, ok := lk.Find(dep.Name)
	if !ok || pkg.Git != dep.Git {
		return lock.Package{}, false
	}
	if !refFromConfig(dep).matches(pkg) {
		return lock.Package{}, false
	}
	if err := validate.CommitID(pkg.Commit); err != nil {
		return lock.Package{}, false
	}
	return pkg, true
}

// packageFor returns the lock entry for dep pinned at commit.
func packageFor(dep config.Dependency, commit string) lock.Package {
	return lock.Package{
		Name:   dep.Name,
		Git:    dep.Git,
		Branch: dep.Branch,
		Tag:    dep.Tag,
		Rev:    dep.Rev,
		Commit: commit,
	}
}
