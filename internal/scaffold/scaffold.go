// Package scaffold implements the project layout created by "woden new" and
// "woden init".
//
// Both commands generate the same set of files. New requires a missing or empty
// target directory and creates every file; Init never overwrites anything and
// only fills in what is missing. Every write is recorded, so a failure leaves
// the target exactly as it was found: new removes the files and directories it
// created, and init additionally leaves pre-existing files untouched.
package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"woden/internal/errs"
	"woden/internal/fsx"
	"woden/internal/validate"
)

// writeNew creates a file that must not exist yet. It is a variable so tests
// can inject a failure in the middle of scaffolding to exercise the rollback
// path.
var writeNew = fsx.WriteNew

// New creates a new Woden project at path.
//
// The target directory must not exist or be an empty directory. A missing
// target, including any missing parents, is created. The project name is taken
// from the target directory name, so an invalid name is reported before
// anything is created and the user can rename the directory. Nothing is printed
// on success.
func New(path string) error {
	target, err := fsx.AbsClean(path)
	if err != nil {
		return err
	}
	name, err := projectName(target)
	if err != nil {
		return err
	}
	info, err := os.Lstat(target)
	switch {
	case err == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return errs.Userf("%s is a symbolic link; refusing to create a project through it", target)
		}
		if !info.IsDir() {
			return errs.Userf("%s already exists and is not a directory", target)
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return errs.WrapUser(err, "cannot read %s", target)
		}
		if len(entries) > 0 {
			return errs.Userf("%s already exists and is not empty", target)
		}
	case errors.Is(err, fs.ErrNotExist):
		// The target and possibly some of its parents are created below.
	default:
		return errs.WrapUser(err, "cannot inspect %s", target)
	}

	g := &generator{root: target, name: name}
	if err := g.mkdirAll(target); err != nil {
		g.rollback()
		return err
	}
	if err := g.createAll(); err != nil {
		g.rollback()
		return err
	}
	return nil
}

// Init initializes the existing directory path as a Woden project.
//
// An empty path means the current directory. The target must already exist and
// be a directory. A directory that already contains woden.toml is reported as
// already initialized. Missing default files are created, existing files are
// never overwritten, and an existing .gitignore is only completed with the
// entries it is missing. Nothing is printed on success.
func Init(path string) error {
	if path == "" {
		path = "."
	}
	root, err := fsx.AbsClean(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(root)
	switch {
	case err == nil:
		if !info.IsDir() {
			return errs.Userf("%s is not a directory", root)
		}
	case errors.Is(err, fs.ErrNotExist):
		return errs.Userf("%s does not exist", root)
	default:
		return errs.WrapUser(err, "cannot inspect %s", root)
	}
	name, err := projectName(root)
	if err != nil {
		return err
	}
	manifestPath := filepath.Join(root, manifestFileName)
	exists, err := fsx.Exists(manifestPath)
	if err != nil {
		return errs.WrapUser(err, "cannot inspect %s", manifestPath)
	}
	if exists {
		return errs.Userf("%s already exists; %s has already been initialized", manifestPath, root)
	}

	g := &generator{root: root, name: name}
	if err := g.initMissing(); err != nil {
		g.rollback()
		return err
	}
	return nil
}

// generator creates project files and records everything it created so a
// failure can be rolled back without touching anything that existed before.
type generator struct {
	root         string
	name         string
	createdFiles []string
	createdDirs  []string
}

// createAll creates every project file. The project root itself has already
// been created by New.
func (g *generator) createAll() error {
	files := []struct {
		path string
		data []byte
	}{
		{filepath.Join(g.root, manifestFileName), wodenTOML(g.name)},
		{filepath.Join(g.root, olsFileName), olsJSON()},
		{filepath.Join(g.root, sourceDirName, mainOdinName), mainOdin()},
		{filepath.Join(g.root, testsDirName, mainTestOdinName), mainTestOdin()},
		{filepath.Join(g.root, gitignoreName), gitignore()},
	}
	for _, file := range files {
		if err := g.mkdirAll(filepath.Dir(file.path)); err != nil {
			return err
		}
		if err := g.createFile(file.path, file.data); err != nil {
			return err
		}
	}
	return nil
}

// initMissing creates the default files that are missing and leaves everything
// else untouched. The existing .gitignore is completed last, after creating the
// new files has succeeded, because it is the only step that modifies a file that
// already existed.
func (g *generator) initMissing() error {
	if err := g.createFile(filepath.Join(g.root, manifestFileName), wodenTOML(g.name)); err != nil {
		return err
	}
	olsPath := filepath.Join(g.root, olsFileName)
	exists, err := fsx.Exists(olsPath)
	if err != nil {
		return errs.WrapUser(err, "cannot inspect %s", olsPath)
	}
	if !exists {
		if err := g.createFile(olsPath, olsJSON()); err != nil {
			return err
		}
	}
	if err := g.ensureSample(sourceDirName, mainOdinName, mainOdin()); err != nil {
		return err
	}
	if err := g.ensureSample(testsDirName, mainTestOdinName, mainTestOdin()); err != nil {
		return err
	}
	return g.ensureGitignore(filepath.Join(g.root, gitignoreName))
}

// ensureSample creates dir/fileName unless dir already contains an Odin source
// file at any depth, in which case the project's own sources are left alone.
func (g *generator) ensureSample(dirName, fileName string, data []byte) error {
	dir := filepath.Join(g.root, dirName)
	info, err := os.Lstat(dir)
	switch {
	case err == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return errs.Userf("%s is a symbolic link; refusing to create files through it", dir)
		}
		if !info.IsDir() {
			return errs.Userf("%s is not a directory", dir)
		}
		hasOdin, err := containsOdinFile(dir)
		if err != nil {
			return err
		}
		if hasOdin {
			return nil
		}
	case errors.Is(err, fs.ErrNotExist):
		// No .odin file is present in a directory that does not exist.
	default:
		return errs.WrapUser(err, "cannot inspect %s", dir)
	}
	if err := g.mkdirAll(dir); err != nil {
		return err
	}
	return g.createFile(filepath.Join(dir, fileName), data)
}

// ensureGitignore creates .gitignore when it is missing. An existing file keeps
// its content byte for byte: only the well-known entries that are missing are
// appended.
func (g *generator) ensureGitignore(path string) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return errs.Userf("%s is a symbolic link; refusing to modify it", path)
		}
		if info.IsDir() {
			return errs.Userf("%s is a directory, not a file", path)
		}
	case errors.Is(err, fs.ErrNotExist):
		return g.createFile(path, gitignore())
	default:
		return errs.WrapUser(err, "cannot inspect %s", path)
	}
	data, err := fsx.ReadFile(path)
	if err != nil {
		return err
	}
	updated, changed := appendMissingGitignoreLines(data)
	if !changed {
		return nil
	}
	// The file already existed, so it is updated atomically in place instead of
	// being recreated: the bytes before the appended lines are preserved. This
	// is the last write of Init, so no rollback can lose the original content.
	return fsx.AtomicWrite(path, updated, info.Mode().Perm())
}

// createFile writes data to path, which must not exist yet.
func (g *generator) createFile(path string, data []byte) error {
	if err := writeNew(path, data); err != nil {
		return err
	}
	g.createdFiles = append(g.createdFiles, path)
	return nil
}

// mkdirAll creates dir and any missing parents, recording the directories it
// created. A symbolic link where a directory is expected is rejected, because
// creating files through it could write outside the project.
func (g *generator) mkdirAll(dir string) error {
	info, err := os.Lstat(dir)
	switch {
	case err == nil:
		if info.Mode()&fs.ModeSymlink != 0 {
			return errs.Userf("%s is a symbolic link; refusing to create files through it", dir)
		}
		if !info.IsDir() {
			return errs.Userf("%s is not a directory", dir)
		}
		return nil
	case !errors.Is(err, fs.ErrNotExist):
		return errs.WrapUser(err, "cannot inspect %s", dir)
	}

	var missing []string
	for current := dir; ; current = filepath.Dir(current) {
		if _, err := os.Lstat(current); err == nil {
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return errs.WrapUser(err, "cannot inspect %s", current)
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], 0o755); err != nil {
			if errors.Is(err, fs.ErrExist) {
				continue
			}
			return errs.WrapUser(err, "cannot create directory %s", missing[i])
		}
		g.createdDirs = append(g.createdDirs, missing[i])
	}
	return nil
}

// rollback removes everything the generator created, newest first. Removal
// errors are ignored: rollback is best effort and must not hide the original
// failure. Directories are only removed when they are empty.
func (g *generator) rollback() {
	for i := len(g.createdFiles) - 1; i >= 0; i-- {
		_ = os.Remove(g.createdFiles[i])
	}
	for i := len(g.createdDirs) - 1; i >= 0; i-- {
		_ = os.Remove(g.createdDirs[i])
	}
	g.createdFiles = nil
	g.createdDirs = nil
}

// projectName derives the project name from the absolute project directory and
// validates it.
func projectName(target string) (string, error) {
	name := filepath.Base(target)
	if err := validate.ProjectName(name); err != nil {
		return "", errs.Userf("invalid project name %q: %v; rename the directory and try again", name, err)
	}
	return name, nil
}

// containsOdinFile reports whether dir contains an Odin source file at any
// depth. Symbolic links are not followed.
func containsOdinFile(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".odin") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, errs.WrapUser(err, "cannot scan %s", dir)
	}
	return found, nil
}
