package gitdep

import (
	"context"
	"fmt"
	"os"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/lock"
	"github.com/yuqing-256/woden/internal/validate"
)

// Update advances dependencies that track a branch, a tag or the remote default
// branch to the commit the remote resolves to now. Dependencies pinned with
// --rev are never updated automatically: without a NAME they are reported as
// skipped, with an explicit NAME the command fails.
//
// A worktree with local modifications is never overwritten. Missing worktrees
// are restored at the new commit.
//
// Write order per dependency: woden.lock is written before the worktree is
// checked out, so an interrupted update leaves a state that "woden sync"
// repairs by checking out the locked commit.
func (d *Deps) Update(ctx context.Context, names []string) error {
	if d.Project.Config == nil {
		return errs.Userf("project configuration is not loaded")
	}
	deps := d.Project.Config.SortedDependencies()
	if len(deps) == 0 {
		fmt.Fprintln(d.out(), "no Git dependencies to update")
		return nil
	}
	selected, err := d.updateSelection(names)
	if err != nil {
		return err
	}
	lk, err := d.loadLock()
	if err != nil {
		return err
	}
	problems := &problem{w: d.errOut(), what: "woden update"}
	for _, dep := range deps {
		if selected != nil && !selected[dep.Name] {
			continue
		}
		spec := refFromConfig(dep)
		if spec.kind == "rev" {
			fmt.Fprintf(d.out(), "skipped %s: pinned to revision %s\n", dep.Name, spec.value)
			continue
		}
		if err := d.updateOne(ctx, dep, spec, lk, problems); err != nil {
			return err
		}
	}
	if _, err := d.updateOLS(); err != nil {
		return err
	}
	return problems.err()
}

// updateSelection validates the requested dependency names. It returns nil when
// every dependency should be updated.
func (d *Deps) updateSelection(names []string) (map[string]bool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	selected := make(map[string]bool, len(names))
	for _, name := range names {
		if err := validate.DependencyName(name); err != nil {
			return nil, errs.WrapUser(err, "invalid dependency name")
		}
		dep, ok := d.Project.Config.DependencyByName(name)
		if !ok {
			return nil, errs.Userf("dependency %q is not declared in woden.toml", name)
		}
		if kind, value := dep.Ref(); kind == "rev" {
			return nil, errs.Userf("dependency %q is pinned to revision %s and cannot be updated", name, value)
		}
		selected[name] = true
	}
	return selected, nil
}

// updateOne updates one dependency that is not pinned to a revision.
func (d *Deps) updateOne(ctx context.Context, dep config.Dependency, spec refSpec, lk *lock.Lock, problems *problem) error {
	vendorPath := d.vendorPath(dep.Name)
	present, err := exists(vendorPath)
	if err != nil {
		return err
	}
	locked, hasLock := matchingLock(lk, dep)
	if !present {
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
		commit, err := d.resolveCommit(ctx, stage, spec.cloneRefExpression())
		if err != nil {
			return errs.WrapUser(err, "cannot resolve %s of %s", spec.describe(), dep.Git)
		}
		lk.Upsert(packageFor(dep, commit))
		if err := lk.Save(d.Project.LockPath()); err != nil {
			return err
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
	if !hasOrigin || origin != dep.Git {
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
	commit, err := d.fetch(ctx, vendorPath, dep.Git, spec)
	if err != nil {
		return err
	}
	old := ""
	if hasLock {
		old = locked.Commit
	}
	if old == commit {
		fmt.Fprintf(d.out(), "%s is up to date at %s\n", dep.Name, shortCommit(commit))
		return nil
	}
	lk.Upsert(packageFor(dep, commit))
	if err := lk.Save(d.Project.LockPath()); err != nil {
		return err
	}
	if err := d.checkout(ctx, vendorPath, commit); err != nil {
		return err
	}
	if old == "" {
		fmt.Fprintf(d.out(), "updated %s to %s\n", dep.Name, shortCommit(commit))
	} else {
		fmt.Fprintf(d.out(), "updated %s: %s -> %s\n", dep.Name, shortCommit(old), shortCommit(commit))
	}
	return nil
}
