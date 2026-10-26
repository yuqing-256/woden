// Package clean builds and executes a safe deletion plan for a project's
// target directory.
//
// The plan is computed with os.Lstat only. Symbolic links, junctions and other
// Windows reparse points are recorded as entries and never followed, so their
// targets are never traversed. Every entry is checked to be lexically inside
// the target directory, both while the plan is built and again right before
// anything is removed. The same plan is used for --dry-run and for the real
// deletion, so what is listed is exactly what is removed.
package clean

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"woden/internal/errs"
	"woden/internal/fsx"
)

// TargetDir is the project directory removed by clean.
const TargetDir = "target"

// Entry is one filesystem entry scheduled for deletion.
type Entry struct {
	// Path is the absolute, cleaned path of the entry.
	Path string
	// Rel is the slash-separated path relative to the project root.
	Rel string
}

// Plan is a deterministic deletion plan for a project's target directory.
type Plan struct {
	// Root is the absolute path of the project root.
	Root string
	// Target is the absolute path of the target directory.
	Target string
	// Entries lists every path to delete. Children always come before their
	// parent directory, so deleting the entries in order works. The target
	// directory itself is not part of Entries; Remove deletes it afterwards.
	Entries []Entry
}

// BuildPlan computes the deletion plan for the project rooted at projectRoot.
//
// BuildPlan never writes to the filesystem. A missing target directory yields
// an empty plan and no error; a target that is not a directory (and not a
// link) is rejected. The returned plan is deterministic: entries are collected
// in lexical order, children before parents.
func BuildPlan(projectRoot string) (*Plan, error) {
	root, err := fsx.AbsClean(projectRoot)
	if err != nil {
		return nil, errs.WrapUser(err, "cannot resolve the project root")
	}
	plan := &Plan{Root: root, Target: filepath.Join(root, TargetDir)}

	info, err := os.Lstat(plan.Target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return plan, nil
		}
		return nil, errs.WrapUser(err, "cannot inspect %s", plan.Target)
	}
	if isLink(info) {
		// target itself is a link: remove the link, never its target.
		plan.Entries = append(plan.Entries, plan.entry(plan.Target))
		return plan, nil
	}
	if !info.IsDir() {
		return nil, errs.Userf("%s is not a directory; refusing to clean it", plan.Target)
	}
	if err := plan.walk(plan.Target); err != nil {
		return nil, err
	}
	return plan, nil
}

// walk collects the entries inside dir, children before parents. os.ReadDir
// sorts by filename, which makes the plan deterministic.
func (p *Plan) walk(dir string) error {
	dirents, err := os.ReadDir(dir)
	if err != nil {
		return errs.WrapUser(err, "cannot read %s", dir)
	}
	for _, dirent := range dirents {
		path := filepath.Join(dir, dirent.Name())
		if err := p.checkInside(path); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return errs.WrapUser(err, "cannot inspect %s", path)
		}
		if isLink(info) {
			// Never traverse links: remove the link itself.
			p.Entries = append(p.Entries, p.entry(path))
			continue
		}
		if info.IsDir() {
			if err := p.walk(path); err != nil {
				return err
			}
		}
		p.Entries = append(p.Entries, p.entry(path))
	}
	return nil
}

// checkInside rejects any path that is not lexically inside the target
// directory. Walked paths are built from directory entries, so ".." fragments
// cannot occur; this check is defense in depth against future changes and
// against odd platform behaviour.
func (p *Plan) checkInside(path string) error {
	if p.Target == "" || !fsx.IsWithin(p.Target, path) {
		return errs.Userf("refusing to delete %s: it is outside %s", path, p.Target)
	}
	return nil
}

// isLink reports whether info describes a symbolic link, junction or other
// reparse point. Such entries are removed as links; their contents are never
// traversed.
func isLink(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0 || isReparsePoint(info)
}

// entry builds an Entry for path.
func (p *Plan) entry(path string) Entry {
	rel, err := filepath.Rel(p.Root, path)
	if err != nil {
		rel = path
	}
	return Entry{Path: path, Rel: filepath.ToSlash(rel)}
}

// RelPaths returns the project-relative paths of the plan in deletion order.
func (p *Plan) RelPaths() []string {
	if p == nil {
		return nil
	}
	paths := make([]string, len(p.Entries))
	for i, e := range p.Entries {
		paths[i] = e.Rel
	}
	return paths
}

// Remove deletes every entry of the plan and then the target directory itself.
//
// The whole plan is validated again before anything is removed: an entry
// outside the target directory aborts the deletion before a single file is
// touched. Entries that vanished since the plan was built are ignored, so a
// concurrent clean is not an error.
func (p *Plan) Remove() error {
	if p == nil || p.Target == "" {
		return errs.Userf("internal error: clean plan has no target directory")
	}
	for _, e := range p.Entries {
		if err := p.checkInside(e.Path); err != nil {
			return err
		}
	}
	for _, e := range p.Entries {
		if err := removePath(e.Path); err != nil {
			return errs.WrapUser(err, "cannot delete %s", e.Path)
		}
	}
	if err := removePath(p.Target); err != nil {
		return errs.WrapUser(err, "cannot delete %s", p.Target)
	}
	return nil
}

// removePath deletes path without following links. A path that is already gone
// is not an error.
func removePath(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
