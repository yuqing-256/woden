package gitdep

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/lock"
	"github.com/yuqing-256/woden/internal/project"
	"github.com/yuqing-256/woden/internal/validate"
)

// AddOptions carries the flags of "woden add".
type AddOptions struct {
	Name   string
	Git    string
	Branch string
	Tag    string
	Rev    string
}

// dependency returns the manifest entry for the request.
func (o AddOptions) dependency() config.Dependency {
	return config.Dependency{Name: o.Name, Git: o.Git, Branch: o.Branch, Tag: o.Tag, Rev: o.Rev}
}

// ref returns the requested source reference.
func (o AddOptions) ref() refSpec {
	return refFromConfig(o.dependency())
}

// Add declares a new Git dependency, clones it into vendor/ and pins the
// resolved commit in woden.lock.
//
// Write order (every step is reentrant, and "woden sync" repairs the states a
// crash can leave behind):
//
//  1. woden.toml is edited first: the dependency becomes declared, and sync
//     then either restores the missing vendor worktree from the lock or, when
//     the lock write was interrupted too, resolves and pins the requested ref.
//  2. woden.lock pins the full commit, so a sync after this point restores the
//     worktree without touching the network.
//  3. The staged clone is moved to vendor/<name>.
//  4. ols.json is reconciled. ols.Sync finishes the deps mapping update.
func (d *Deps) Add(ctx context.Context, opts AddOptions) error {
	if d.Project.Config == nil {
		return errs.Userf("project configuration is not loaded")
	}
	if err := validateAddOptions(opts); err != nil {
		return err
	}
	name := opts.Name
	for _, declared := range d.Project.Config.SortedDependencyNames() {
		if strings.EqualFold(declared, name) {
			return errs.Userf("dependency %q is already declared in woden.toml", declared)
		}
	}
	if path, ok := d.Project.Config.DepsCollection(); ok && path != project.VendorDir {
		return errs.Userf("collection %q maps to %q, but Git dependencies must be collected in %q; edit woden.toml first",
			project.DepsCollection, path, project.VendorDir)
	}
	vendorPath := d.vendorPath(name)
	if ok, err := exists(vendorPath); err != nil {
		return err
	} else if ok {
		return errs.Userf("%s already exists; remove it before adding dependency %q", vendorRel(name), name)
	}
	if entries, err := d.vendorEntries(); err != nil {
		return err
	} else {
		for _, entry := range entries {
			if strings.EqualFold(entry, name) {
				return errs.Userf("%s already exists; remove it before adding dependency %q", vendorRel(entry), name)
			}
		}
	}

	spec := opts.ref()
	// 1. Clone the requested commit into a staging directory. The checkout
	// happens before the lock is written, so a failed clone leaves no trace.
	stage, err := d.stagingDir()
	if err != nil {
		return err
	}
	stageMoved := false
	defer func() {
		if !stageMoved {
			removeStaging(stage)
		}
	}()
	if err := d.clone(ctx, opts.Git, stage); err != nil {
		return err
	}
	commit, err := d.resolveCommit(ctx, stage, spec.cloneRefExpression())
	if err != nil {
		return errs.WrapUser(err, "cannot resolve %s of %s", spec.describe(), opts.Git)
	}
	if err := d.checkout(ctx, stage, commit); err != nil {
		return err
	}

	// 2. Declare the dependency, adding the deps collection on demand.
	dep := opts.dependency()
	addCollection := false
	if _, ok := d.Project.Config.DepsCollection(); !ok {
		addCollection = true
	}
	err = config.Edit(d.Project.ConfigPath, func(e *config.Editor) error {
		if err := e.AddDependency(dep); err != nil {
			return err
		}
		if addCollection {
			if err := e.SetCollection(project.DepsCollection, project.VendorDir); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// 3. Pin the full commit in woden.lock.
	locked := lock.Package{
		Name:   name,
		Git:    opts.Git,
		Branch: opts.Branch,
		Tag:    opts.Tag,
		Rev:    opts.Rev,
		Commit: commit,
	}
	lk, err := d.loadLock()
	if err != nil {
		return err
	}
	lk.Upsert(locked)
	if err := lk.Save(d.Project.LockPath()); err != nil {
		return err
	}

	// 4. Move the finished clone into place atomically.
	if err := os.Rename(stage, vendorPath); err != nil {
		return errs.WrapUser(err, "cannot move the staged clone to %s", vendorRel(name))
	}
	stageMoved = true

	// 5. Reconcile ols.json with the new collections.
	if err := d.reload(); err != nil {
		return err
	}
	fmt.Fprintf(d.out(), "added dependency %s (%s) at %s\n", name, spec.describe(), shortCommit(commit))
	if _, err := d.updateOLS(); err != nil {
		return err
	}
	return nil
}

// validateAddOptions validates the add request before any Git access.
func validateAddOptions(opts AddOptions) error {
	if err := validate.DependencyName(opts.Name); err != nil {
		return errs.WrapUser(err, "invalid dependency name")
	}
	if err := validate.GitURL(opts.Git); err != nil {
		return errs.WrapUser(err, "invalid git URL")
	}
	refs := 0
	if opts.Branch != "" {
		refs++
		if err := validate.RefName(opts.Branch); err != nil {
			return errs.WrapUser(err, "invalid branch")
		}
	}
	if opts.Tag != "" {
		refs++
		if err := validate.RefName(opts.Tag); err != nil {
			return errs.WrapUser(err, "invalid tag")
		}
	}
	if opts.Rev != "" {
		refs++
		if err := validate.Rev(opts.Rev); err != nil {
			return errs.WrapUser(err, "invalid revision")
		}
	}
	if refs > 1 {
		return errs.Userf("at most one of --branch, --tag and --rev may be given")
	}
	return nil
}

// loadLock loads woden.lock, treating a missing file as an empty lock.
func (d *Deps) loadLock() (*lock.Lock, error) {
	lk, err := lock.Load(d.Project.LockPath())
	if err != nil {
		if lock.IsNotFound(err) {
			return lock.New(), nil
		}
		return nil, err
	}
	return lk, nil
}

// shortCommit abbreviates a full commit ID for messages.
func shortCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}
