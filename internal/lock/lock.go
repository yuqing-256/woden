// Package lock reads and writes woden.lock.
//
// The lock file records, for every Git dependency, the resolved full commit ID
// so that builds are reproducible. It is a small TOML document written by
// Woden itself; the writer is deterministic (packages sorted by name, fixed
// field order) so that no two machines produce different lock files for the
// same dependency state.
package lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/fsx"
	"github.com/yuqing-256/woden/internal/tomlutil"
	"github.com/yuqing-256/woden/internal/validate"
)

// FileName is the name of the lock file.
const FileName = "woden.lock"

// Version is the only lock format version Woden understands.
const Version = 1

// Package is one [[package]] entry.
type Package struct {
	Name string `toml:"name"`
	Git  string `toml:"git"`
	// At most one of Branch, Tag and Rev is set. Rev holds the value the user
	// asked for (possibly abbreviated); Commit always holds the full ID.
	Branch string `toml:"branch,omitempty"`
	Tag    string `toml:"tag,omitempty"`
	Rev    string `toml:"rev,omitempty"`
	Commit string `toml:"commit"`
}

// RefKind returns "branch", "tag", "rev" or "" for the remote default branch.
func (p Package) RefKind() string {
	switch {
	case p.Branch != "":
		return "branch"
	case p.Tag != "":
		return "tag"
	case p.Rev != "":
		return "rev"
	default:
		return ""
	}
}

// RefValue returns the reference value, or "" for the default branch.
func (p Package) RefValue() string {
	switch {
	case p.Branch != "":
		return p.Branch
	case p.Tag != "":
		return p.Tag
	case p.Rev != "":
		return p.Rev
	default:
		return ""
	}
}

// Lock is the decoded woden.lock file.
type Lock struct {
	Version  int       `toml:"version"`
	Packages []Package `toml:"package"`
}

// New returns an empty lock file at the current version.
func New() *Lock {
	return &Lock{Version: Version}
}

// Load reads and validates the lock file at path. A missing file is reported
// with an error wrapping fs.ErrNotExist.
func Load(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse decodes and validates a lock file.
func Parse(data []byte) (*Lock, error) {
	var l Lock
	md, err := toml.Decode(string(data), &l)
	if err != nil {
		return nil, errs.WrapUser(err, "invalid %s", FileName)
	}
	if !md.IsDefined("version") {
		return nil, errs.Userf("%s is missing its version", FileName)
	}
	if l.Version != Version {
		return nil, errs.Userf("unsupported %s version %d (expected %d)", FileName, l.Version, Version)
	}
	seen := make(map[string]bool, len(l.Packages))
	for i := range l.Packages {
		pkg := &l.Packages[i]
		if err := validatePackage(*pkg); err != nil {
			return nil, err
		}
		key := strings.ToLower(pkg.Name)
		if seen[key] {
			return nil, errs.Userf("duplicate package %q in %s", pkg.Name, FileName)
		}
		seen[key] = true
	}
	return &l, nil
}

func validatePackage(pkg Package) error {
	if err := validate.DependencyName(pkg.Name); err != nil {
		return errs.WrapUser(err, "invalid package name in %s", FileName)
	}
	if pkg.Git == "" {
		return errs.Userf("package %q in %s has no git source", pkg.Name, FileName)
	}
	if err := validate.GitURL(pkg.Git); err != nil {
		return errs.WrapUser(err, "invalid git source for package %q in %s", pkg.Name, FileName)
	}
	refs := 0
	if pkg.Branch != "" {
		refs++
		if err := validate.RefName(pkg.Branch); err != nil {
			return errs.WrapUser(err, "invalid branch for package %q in %s", pkg.Name, FileName)
		}
	}
	if pkg.Tag != "" {
		refs++
		if err := validate.RefName(pkg.Tag); err != nil {
			return errs.WrapUser(err, "invalid tag for package %q in %s", pkg.Name, FileName)
		}
	}
	if pkg.Rev != "" {
		refs++
		if err := validate.Rev(pkg.Rev); err != nil {
			return errs.WrapUser(err, "invalid revision for package %q in %s", pkg.Name, FileName)
		}
	}
	if refs > 1 {
		return errs.Userf("package %q in %s may specify only one of branch, tag and rev", pkg.Name, FileName)
	}
	if err := validate.CommitID(pkg.Commit); err != nil {
		return errs.WrapUser(err, "invalid commit for package %q in %s", pkg.Name, FileName)
	}
	return nil
}

// Bytes returns the serialized lock file. Packages are sorted by name so the
// output is stable across runs.
func (l *Lock) Bytes() []byte {
	packages := slices.Clone(l.Packages)
	slices.SortFunc(packages, func(a, b Package) int {
		return strings.Compare(a.Name, b.Name)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "version = %d\n", Version)
	for _, pkg := range packages {
		b.WriteString("\n[[package]]\n")
		fmt.Fprintf(&b, "name = %s\n", tomlutil.Quote(pkg.Name))
		fmt.Fprintf(&b, "git = %s\n", tomlutil.Quote(pkg.Git))
		switch {
		case pkg.Branch != "":
			fmt.Fprintf(&b, "branch = %s\n", tomlutil.Quote(pkg.Branch))
		case pkg.Tag != "":
			fmt.Fprintf(&b, "tag = %s\n", tomlutil.Quote(pkg.Tag))
		case pkg.Rev != "":
			fmt.Fprintf(&b, "rev = %s\n", tomlutil.Quote(pkg.Rev))
		}
		fmt.Fprintf(&b, "commit = %s\n", tomlutil.Quote(pkg.Commit))
	}
	return []byte(b.String())
}

// Save writes the lock file atomically.
func (l *Lock) Save(path string) error {
	return fsx.AtomicWrite(path, l.Bytes(), 0o644)
}

// Find returns the package with the given name.
func (l *Lock) Find(name string) (Package, bool) {
	for _, pkg := range l.Packages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return Package{}, false
}

// Sorted returns the packages sorted by name.
func (l *Lock) Sorted() []Package {
	packages := slices.Clone(l.Packages)
	slices.SortFunc(packages, func(a, b Package) int {
		return strings.Compare(a.Name, b.Name)
	})
	return packages
}

// Upsert inserts or replaces a package.
func (l *Lock) Upsert(pkg Package) {
	for i := range l.Packages {
		if l.Packages[i].Name == pkg.Name {
			l.Packages[i] = pkg
			return
		}
	}
	l.Packages = append(l.Packages, pkg)
}

// Remove deletes a package by name.
func (l *Lock) Remove(name string) {
	filtered := l.Packages[:0]
	for _, pkg := range l.Packages {
		if pkg.Name != name {
			filtered = append(filtered, pkg)
		}
	}
	l.Packages = filtered
}

// IsNotFound reports whether err means the lock file does not exist.
func IsNotFound(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
