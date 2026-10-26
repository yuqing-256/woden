// Package config loads, validates and edits woden.toml.
//
// The TOML implementation is github.com/BurntSushi/toml: it supports strict
// decoding via MetaData.Undecoded (used to warn about unknown keys) and reports
// syntax errors with a line position. Writing is never done by re-encoding the
// decoded struct: the original file is edited textually so comments and
// formatting survive (see Editor).
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/validate"
)

// Supported profile names.
const (
	ProfileDev     = "dev"
	ProfileRelease = "release"
)

// FileName is the name of the project manifest.
const FileName = "woden.toml"

// OptLevels lists the values accepted for "opt-level" (they are the values
// Odin accepts for -o:).
var OptLevels = []string{"none", "minimal", "size", "speed", "aggressive"}

// Package is the [package] table.
type Package struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

// Dependency is one entry of the [dependencies] table.
type Dependency struct {
	// Name is the key of the table; it is filled in by Decode.
	Name   string `toml:"-"`
	Git    string `toml:"git"`
	Branch string `toml:"branch"`
	Tag    string `toml:"tag"`
	Rev    string `toml:"rev"`
}

// Ref returns the requested reference kind ("branch", "tag", "rev") and its
// value. An empty kind means the dependency tracks the remote default branch.
func (d Dependency) Ref() (kind, value string) {
	switch {
	case d.Branch != "":
		return "branch", d.Branch
	case d.Tag != "":
		return "tag", d.Tag
	case d.Rev != "":
		return "rev", d.Rev
	default:
		return "", ""
	}
}

// ProfileOverride is one entry of the [profile] table.
type ProfileOverride struct {
	Debug    *bool   `toml:"debug"`
	OptLevel *string `toml:"opt-level"`
}

// Config is the decoded woden.toml.
type Config struct {
	Package      Package                    `toml:"package"`
	Collections  map[string]string          `toml:"collections"`
	Profiles     map[string]ProfileOverride `toml:"profile"`
	Dependencies map[string]Dependency      `toml:"dependencies"`
}

// Profile is one of the two build profiles supported by Woden.
type Profile struct {
	Name      string
	Debug     bool
	OptLevel  string
	OutputDir string
}

// Load reads and validates the manifest at path.
//
// The returned warnings are non-fatal configuration problems (unknown keys).
func Load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, errs.WrapUser(err, "cannot read %s", path)
	}
	cfg, warnings, err := Decode(data)
	if err != nil {
		return nil, warnings, errs.WrapUser(err, "invalid %s", path)
	}
	return cfg, warnings, nil
}

// Decode parses and validates the manifest.
func Decode(data []byte) (*Config, []string, error) {
	var cfg Config
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, nil, wrapDecodeError(err)
	}
	warnings := unknownKeyWarnings(md)

	if !md.IsDefined("package") {
		return nil, warnings, errs.Userf("missing [package] section")
	}
	if cfg.Package.Name == "" {
		return nil, warnings, errs.Userf("package.name is required")
	}
	if err := validate.ProjectName(cfg.Package.Name); err != nil {
		return nil, warnings, errs.WrapUser(err, "invalid package.name")
	}
	if cfg.Package.Version == "" {
		return nil, warnings, errs.Userf("package.version is required")
	}

	for _, name := range SortedKeys(cfg.Collections) {
		if err := validate.CollectionName(name); err != nil {
			return nil, warnings, errs.WrapUser(err, "invalid collection name")
		}
		if err := validate.CollectionPath(cfg.Collections[name]); err != nil {
			return nil, warnings, errs.WrapUser(err, "invalid path for collection %q", name)
		}
	}

	for _, name := range SortedKeys(cfg.Profiles) {
		if name != ProfileDev && name != ProfileRelease {
			return nil, warnings, errs.Userf("unknown profile %q: only %q and %q may be overridden", name, ProfileDev, ProfileRelease)
		}
		ov := cfg.Profiles[name]
		if ov.OptLevel != nil && !slices.Contains(OptLevels, *ov.OptLevel) {
			return nil, warnings, errs.Userf("invalid opt-level %q in profile %q (allowed values: %s)", *ov.OptLevel, name, strings.Join(OptLevels, ", "))
		}
	}

	for _, name := range SortedKeys(cfg.Dependencies) {
		dep := cfg.Dependencies[name]
		dep.Name = name
		cfg.Dependencies[name] = dep
		if err := validate.DependencyName(name); err != nil {
			return nil, warnings, errs.WrapUser(err, "invalid dependency name")
		}
		if dep.Git == "" {
			return nil, warnings, errs.Userf("dependency %q must declare a git source", name)
		}
		if err := validate.GitURL(dep.Git); err != nil {
			return nil, warnings, errs.WrapUser(err, "invalid git source for dependency %q", name)
		}
		refs := 0
		if dep.Branch != "" {
			refs++
			if err := validate.RefName(dep.Branch); err != nil {
				return nil, warnings, errs.WrapUser(err, "invalid branch for dependency %q", name)
			}
		}
		if dep.Tag != "" {
			refs++
			if err := validate.RefName(dep.Tag); err != nil {
				return nil, warnings, errs.WrapUser(err, "invalid tag for dependency %q", name)
			}
		}
		if dep.Rev != "" {
			refs++
			if err := validate.Rev(dep.Rev); err != nil {
				return nil, warnings, errs.WrapUser(err, "invalid revision for dependency %q", name)
			}
		}
		if refs > 1 {
			return nil, warnings, errs.Userf("dependency %q may specify only one of branch, tag and rev", name)
		}
	}

	return &cfg, warnings, nil
}

// wrapDecodeError turns a TOML parse or type error into a user error.
func wrapDecodeError(err error) error {
	var parseErr toml.ParseError
	if errors.As(err, &parseErr) {
		return errs.WrapUser(err, "cannot parse TOML")
	}
	return errs.WrapUser(err, "invalid TOML")
}

// unknownKeyWarnings returns one warning per key that was not consumed during
// decoding, in document order.
func unknownKeyWarnings(md toml.MetaData) []string {
	var warnings []string
	for _, key := range md.Undecoded() {
		warnings = append(warnings, fmt.Sprintf("unknown key %q is ignored", key.String()))
	}
	return warnings
}

// Profile returns the effective settings of name, applying the overrides from
// the [profile] table.
func (c *Config) Profile(name string) (Profile, error) {
	var p Profile
	switch name {
	case ProfileDev:
		p = Profile{Name: ProfileDev, Debug: true, OptLevel: "minimal", OutputDir: filepath.Join("target", "debug")}
	case ProfileRelease:
		p = Profile{Name: ProfileRelease, Debug: false, OptLevel: "speed", OutputDir: filepath.Join("target", "release")}
	default:
		return Profile{}, errs.Userf("unknown build profile %q", name)
	}
	if ov, ok := c.Profiles[name]; ok {
		if ov.Debug != nil {
			p.Debug = *ov.Debug
		}
		if ov.OptLevel != nil {
			p.OptLevel = *ov.OptLevel
		}
	}
	return p, nil
}

// SortedCollectionNames returns the collection names in a deterministic order.
func (c *Config) SortedCollectionNames() []string {
	return SortedKeys(c.Collections)
}

// SortedDependencyNames returns the dependency names in a deterministic order.
func (c *Config) SortedDependencyNames() []string {
	return SortedKeys(c.Dependencies)
}

// SortedDependencies returns the dependencies sorted by name.
func (c *Config) SortedDependencies() []Dependency {
	names := c.SortedDependencyNames()
	deps := make([]Dependency, 0, len(names))
	for _, name := range names {
		deps = append(deps, c.Dependencies[name])
	}
	return deps
}

// DependencyByName looks up a dependency by name.
func (c *Config) DependencyByName(name string) (Dependency, bool) {
	dep, ok := c.Dependencies[name]
	return dep, ok
}

// DepsCollection returns the path that the "deps" collection maps to, if any.
func (c *Config) DepsCollection() (string, bool) {
	path, ok := c.Collections["deps"]
	return path, ok
}

// SortedKeys returns the keys of m sorted alphabetically.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
