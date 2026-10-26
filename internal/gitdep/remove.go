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
	"woden/internal/project"
	"woden/internal/validate"
)

// Remove deletes a Git dependency from the manifest, the lock and the vendor
// tree.
//
// Ownership is verified before anything is deleted: a worktree is only removed
// when it is a Git worktree, its origin URL is the declared git URL, it has no
// local modifications and it is checked out at the commit woden.lock pins.
// Anything else is reported instead of silently destroyed.
//
// Write order (reentrant; "woden sync" repairs a partial removal):
//
//  1. The vendor worktree is removed.
//  2. The woden.lock entry is dropped (the lock file itself is deleted with the
//     last dependency).
//  3. woden.toml loses the dependency section, and with the last dependency
//     also the deps collection mapping.
//  4. vendor/ is removed once it is empty, so no collection points at a
//     missing directory.
//  5. ols.json is reconciled; ols.Sync drops the deps collection entry once
//     woden.toml no longer maps it.
func (d *Deps) Remove(ctx context.Context, name string) error {
	if d.Project.Config == nil {
		return errs.Userf("project configuration is not loaded")
	}
	if err := validate.DependencyName(name); err != nil {
		return errs.WrapUser(err, "invalid dependency name")
	}
	dep, ok := d.Project.Config.DependencyByName(name)
	if !ok {
		return errs.Userf("dependency %q is not declared in woden.toml", name)
	}
	lk, err := d.loadLock()
	if err != nil {
		return err
	}
	locked, hasLocked := lk.Find(name)
	vendorPath := d.vendorPath(name)
	present, err := exists(vendorPath)
	if err != nil {
		return err
	}
	if present {
		if err := d.verifyOwnership(ctx, dep, locked, hasLocked, vendorPath); err != nil {
			return err
		}
	}

	// 1. The worktree. Ownership has been verified above.
	if present {
		if err := os.RemoveAll(vendorPath); err != nil {
			return errs.WrapUser(err, "cannot remove %s", vendorRel(name))
		}
	}

	// 2. The lock entry, including the lock file with the last dependency.
	lockPath := d.Project.LockPath()
	if hasLocked {
		lk.Remove(name)
		if len(lk.Packages) == 0 {
			if err := os.Remove(lockPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return errs.WrapUser(err, "cannot remove %s", lockPath)
			}
		} else if err := lk.Save(lockPath); err != nil {
			return err
		}
	}

	// 3. The manifest section, and the deps mapping with the last dependency.
	last := len(d.Project.Config.Dependencies) == 1
	err = config.Edit(d.Project.ConfigPath, func(e *config.Editor) error {
		if err := e.RemoveDependency(name); err != nil {
			return err
		}
		if last {
			if err := e.RemoveCollection(project.DepsCollection); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := d.reload(); err != nil {
		return err
	}

	// 4. The now empty vendor directory.
	if last {
		if err := d.cleanStaging(); err != nil {
			return err
		}
		if err := d.removeEmptyVendorDir(); err != nil {
			return err
		}
	}

	// 5. ols.json.
	fmt.Fprintf(d.out(), "removed dependency %s\n", name)
	if _, err := d.updateOLS(); err != nil {
		return err
	}
	return nil
}

// verifyOwnership checks that the worktree at vendorPath is the clone Woden
// created for dep and can safely be deleted.
func (d *Deps) verifyOwnership(ctx context.Context, dep config.Dependency, locked lock.Package, hasLocked bool, vendorPath string) error {
	rel := vendorRel(dep.Name)
	worktree, err := d.isWorktree(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !worktree {
		return errs.Userf("refusing to remove dependency %q: %s is not a Git worktree", dep.Name, rel)
	}
	origin, hasOrigin, err := d.originURL(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !hasOrigin {
		return errs.Userf("refusing to remove dependency %q: %s has no origin remote and its ownership cannot be verified", dep.Name, rel)
	}
	if origin != dep.Git {
		return errs.Userf("refusing to remove dependency %q: %s has origin %q but woden.toml declares %q", dep.Name, rel, origin, dep.Git)
	}
	if hasLocked && locked.Git != dep.Git {
		return errs.Userf("refusing to remove dependency %q: woden.lock records git %q but woden.toml declares %q", dep.Name, locked.Git, dep.Git)
	}
	clean, err := d.isClean(ctx, vendorPath)
	if err != nil {
		return err
	}
	if !clean {
		return errs.Userf("refusing to remove dependency %q: %s has local modifications; commit or discard them first", dep.Name, rel)
	}
	if !hasLocked {
		return errs.Userf("refusing to remove dependency %q: woden.lock has no entry for it, so %s cannot be confirmed as the untouched worktree Woden created", dep.Name, rel)
	}
	head, err := d.head(ctx, vendorPath)
	if err != nil {
		return err
	}
	if head != locked.Commit {
		return errs.Userf("refusing to remove dependency %q: %s is checked out at %s but woden.lock pins %s; it has local commits", dep.Name, rel, shortCommit(head), shortCommit(locked.Commit))
	}
	return nil
}
