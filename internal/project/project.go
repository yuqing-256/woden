// Package project locates the project root from the current directory and
// exposes the paths and collection arguments the commands need.
package project

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/fsx"
	"github.com/yuqing-256/woden/internal/lock"
)

// Names of the well-known project entries.
const (
	SourceDir      = "src"
	TestsDir       = "tests"
	LibsDir        = "libs"
	VendorDir      = "vendor"
	TargetDir      = "target"
	OLSFileName    = "ols.json"
	DepsCollection = "deps"
)

// Project is a loaded Odin project.
type Project struct {
	// Root is the absolute, cleaned project root (the directory containing
	// woden.toml).
	Root       string
	ConfigPath string
	Config     *config.Config
	// Warnings are non-fatal configuration problems (unknown keys).
	Warnings []string
}

type contextKey struct{}

// WithContext returns a context carrying p.
func WithContext(ctx context.Context, p *Project) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}

// FromContext returns the project stored in ctx.
func FromContext(ctx context.Context) (*Project, bool) {
	p, ok := ctx.Value(contextKey{}).(*Project)
	return p, ok
}

// FindRoot walks up from start until it finds a directory containing
// woden.toml and returns that directory.
func FindRoot(start string) (string, error) {
	abs, err := fsx.AbsClean(start)
	if err != nil {
		return "", err
	}
	dir := abs
	for {
		candidate := filepath.Join(dir, config.FileName)
		info, err := os.Stat(candidate)
		switch {
		case err == nil && !info.IsDir():
			return dir, nil
		case err == nil:
			return "", errs.Userf("%s is a directory, not a file", candidate)
		case !errors.Is(err, fs.ErrNotExist):
			return "", errs.WrapUser(err, "cannot inspect %s", candidate)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errs.Userf("no %s found in %s or any parent directory", config.FileName, abs)
		}
		dir = parent
	}
}

// Load finds the project root starting at start and reads its manifest.
func Load(start string) (*Project, error) {
	root, err := FindRoot(start)
	if err != nil {
		return nil, err
	}
	configPath := filepath.Join(root, config.FileName)
	cfg, warnings, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return &Project{Root: root, ConfigPath: configPath, Config: cfg, Warnings: warnings}, nil
}

// Path resolves a project-relative path and checks that it stays inside the
// project root.
func (p *Project) Path(rel string) (string, error) {
	return fsx.Within(p.Root, rel)
}

// Abs joins project-relative parts to the root. It does not perform the
// containment check: callers use it for paths that are already validated.
func (p *Project) Abs(parts ...string) string {
	all := append([]string{p.Root}, parts...)
	return filepath.Join(all...)
}

// SourceDir returns the absolute path of src/.
func (p *Project) SourceDir() string { return p.Abs(SourceDir) }

// TestsDir returns the absolute path of tests/.
func (p *Project) TestsDir() string { return p.Abs(TestsDir) }

// TargetDir returns the absolute path of target/.
func (p *Project) TargetDir() string { return p.Abs(TargetDir) }

// VendorDir returns the absolute path of vendor/.
func (p *Project) VendorDir() string { return p.Abs(VendorDir) }

// LockPath returns the absolute path of woden.lock.
func (p *Project) LockPath() string { return p.Abs(lock.FileName) }

// OLSJSONPath returns the absolute path of ols.json.
func (p *Project) OLSJSONPath() string { return p.Abs(OLSFileName) }

// CollectionArgs returns the -collection:name=path arguments for the [collections]
// table, sorted by collection name so command lines are deterministic.
func (p *Project) CollectionArgs() []string {
	names := p.Config.SortedCollectionNames()
	args := make([]string, 0, len(names))
	for _, name := range names {
		rel := p.Config.Collections[name]
		args = append(args, "-collection:"+name+"="+p.Abs(filepath.FromSlash(rel)))
	}
	return args
}

// Dependencies returns the declared dependencies sorted by name.
func (p *Project) Dependencies() []config.Dependency {
	return p.Config.SortedDependencies()
}

// HasDependencies reports whether any Git dependency is declared.
func (p *Project) HasDependencies() bool {
	return len(p.Config.Dependencies) > 0
}

// ExecutableName returns the output executable name for the project, including
// the platform extension.
func (p *Project) ExecutableName() string {
	name := p.Config.Package.Name
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// Profile returns the effective profile settings for name.
func (p *Project) Profile(name string) (config.Profile, error) {
	return p.Config.Profile(name)
}

// CheckVendor returns a user error when the project needs its dependencies but
// their vendor directory does not exist: the user has to run "woden sync".
//
// Dependencies are needed when woden.toml declares any [dependencies] entries or
// maps the "deps" collection. In both cases the vendor directory (the mapped
// path, or vendor/ when no mapping exists) has to be present before a command can
// compile or run the project.
func (p *Project) CheckVendor() error {
	depsPath, mapped := p.Config.DepsCollection()
	if !mapped && !p.HasDependencies() {
		return nil
	}
	if !mapped {
		depsPath = VendorDir
	}

	target := p.Abs(filepath.FromSlash(depsPath))
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if mapped {
				return errs.Userf("the %q collection is configured but %s does not exist; run `woden sync` to restore the dependencies", DepsCollection, depsPath)
			}
			return errs.Userf("dependencies are declared in woden.toml but %s does not exist; run `woden sync` to restore them", depsPath)
		}
		return errs.WrapUser(err, "cannot inspect %s", depsPath)
	}
	if !info.IsDir() {
		return errs.Userf("%s is not a directory; run `woden sync` to restore the dependencies", depsPath)
	}
	return nil
}

// SourceAndTestDirs returns the source directories that exist, in the order
// they are formatted (src, then tests).
func (p *Project) SourceAndTestDirs() []string {
	var dirs []string
	for _, name := range []string{SourceDir, TestsDir} {
		info, err := os.Stat(p.Abs(name))
		if err == nil && info.IsDir() {
			dirs = append(dirs, name)
		}
	}
	return dirs
}

// SortedPackageNames returns the package names of the lock file, for
// completion.
func (p *Project) SortedPackageNames() []string {
	l, err := lock.Load(p.LockPath())
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(l.Packages))
	for _, pkg := range l.Packages {
		names = append(names, pkg.Name)
	}
	slices.Sort(names)
	return names
}
