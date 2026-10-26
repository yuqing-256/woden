package gitdep

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/lock"
	"github.com/yuqing-256/woden/internal/proc"
	"github.com/yuqing-256/woden/internal/project"
	"github.com/yuqing-256/woden/internal/testutil"
	"github.com/yuqing-256/woden/internal/tomlutil"
)

// Test fixtures for the unit tests. The commits are arbitrary full commit IDs:
// the fake Runner resolves the revision expressions to them.
const (
	testGitURL  = "https://example.com/deplib.git"
	testGitURL2 = "https://example.com/otherlib.git"

	testCommitA = "1111111111111111111111111111111111111111"
	testCommitB = "2222222222222222222222222222222222222222"
	testCommitC = "3333333333333333333333333333333333333333"
)

// testProject is a temporary copy of example/ plus the captured output of an
// operation.
type testProject struct {
	t      *testing.T
	root   string
	proj   *project.Project
	stdout bytes.Buffer
	stderr bytes.Buffer
}

// newTestProject copies example/ into a temporary directory and loads it.
func newTestProject(t *testing.T) *testProject {
	t.Helper()
	root := testutil.CopyExample(t)
	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("gitdep: cannot load the test project: %v", err)
	}
	return &testProject{t: t, root: root, proj: p}
}

// deps returns the operations wired to runner and to the captured output.
func (tp *testProject) deps(runner proc.Runner) *Deps {
	return &Deps{Project: tp.proj, Runner: runner, Stdout: &tp.stdout, Stderr: &tp.stderr}
}

// reload re-reads woden.toml after a test edited it or after an operation.
func (tp *testProject) reload() {
	tp.t.Helper()
	cfg, warnings, err := config.Load(tp.proj.ConfigPath)
	if err != nil {
		tp.t.Fatalf("gitdep: cannot reload woden.toml: %v", err)
	}
	tp.proj.Config = cfg
	tp.proj.Warnings = warnings
}

// manifest returns the current text of woden.toml.
func (tp *testProject) manifest() string {
	tp.t.Helper()
	return testutil.ReadFile(tp.t, tp.proj.ConfigPath)
}

// olsJSON returns the current text of ols.json, or "" when it is absent.
func (tp *testProject) olsJSON() string {
	tp.t.Helper()
	data, err := os.ReadFile(tp.proj.OLSJSONPath())
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		tp.t.Fatalf("gitdep: cannot read ols.json: %v", err)
	}
	return string(data)
}

// olsCollections returns the collections array of ols.json as name to path.
func (tp *testProject) olsCollections() map[string]string {
	tp.t.Helper()
	var doc struct {
		Collections []struct {
			Name string `json:"name"`
			Path string `json:"path"`
		} `json:"collections"`
	}
	data := tp.olsJSON()
	if data == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		tp.t.Fatalf("gitdep: cannot parse ols.json: %v\n%s", err, data)
	}
	collections := make(map[string]string, len(doc.Collections))
	for _, entry := range doc.Collections {
		collections[entry.Name] = entry.Path
	}
	return collections
}

// lock returns the loaded woden.lock and whether it exists.
func (tp *testProject) lock() (*lock.Lock, bool) {
	tp.t.Helper()
	lk, err := lock.Load(tp.proj.LockPath())
	if err != nil {
		if lock.IsNotFound(err) {
			return nil, false
		}
		tp.t.Fatalf("gitdep: cannot load woden.lock: %v", err)
	}
	return lk, true
}

// lockPackages returns woden.lock entries keyed by name.
func (tp *testProject) lockPackages() map[string]lock.Package {
	tp.t.Helper()
	lk, ok := tp.lock()
	if !ok {
		return map[string]lock.Package{}
	}
	packages := make(map[string]lock.Package, len(lk.Packages))
	for _, pkg := range lk.Packages {
		packages[pkg.Name] = pkg
	}
	return packages
}

// declare adds dep to woden.toml, optionally with the deps collection mapping.
func (tp *testProject) declare(dep config.Dependency, withDepsCollection bool) {
	tp.t.Helper()
	err := config.Edit(tp.proj.ConfigPath, func(e *config.Editor) error {
		if err := e.AddDependency(dep); err != nil {
			return err
		}
		if withDepsCollection {
			return e.SetCollection(project.DepsCollection, project.VendorDir)
		}
		return nil
	})
	if err != nil {
		tp.t.Fatalf("gitdep: cannot declare %s: %v", dep.Name, err)
	}
	tp.reload()
}

// writeLock writes a lock containing the given packages.
func (tp *testProject) writeLock(packages ...lock.Package) {
	tp.t.Helper()
	lk := lock.New()
	for _, pkg := range packages {
		lk.Upsert(pkg)
	}
	if err := lk.Save(tp.proj.LockPath()); err != nil {
		tp.t.Fatalf("gitdep: cannot write woden.lock: %v", err)
	}
}

// makeVendor creates a directory (and contents) for the worktree of name.
func (tp *testProject) makeVendor(name, content string) string {
	tp.t.Helper()
	dir := filepath.Join(tp.proj.VendorDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		tp.t.Fatalf("gitdep: cannot create %s: %v", dir, err)
	}
	if content != "" {
		testutil.WriteFile(tp.t, filepath.Join(dir, "deplib.odin"), content)
	}
	return dir
}

// vendorExists reports whether vendor/<name> exists.
func (tp *testProject) vendorExists(name string) bool {
	tp.t.Helper()
	_, err := os.Stat(filepath.Join(tp.proj.VendorDir(), name))
	return err == nil
}

// vendorDirExists reports whether vendor/ exists.
func (tp *testProject) vendorDirExists() bool {
	tp.t.Helper()
	_, err := os.Stat(tp.proj.VendorDir())
	return err == nil
}

// mustContain fails when got does not contain want.
func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("gitdep: expected the output to contain %q\noutput:\n%s", want, got)
	}
}

// mustNotContain fails when got contains unwanted.
func mustNotContain(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Errorf("gitdep: expected the output not to contain %q\noutput:\n%s", unwanted, got)
	}
}

// tomlutilQuote returns the quoted TOML representation of s, for manifest
// assertions.
func tomlutilQuote(s string) string {
	return tomlutil.Quote(s)
}

// snapshot captures the files and vendor/ entries a read-only operation must
// not modify. It is compared before and after "woden sync --check".
func (tp *testProject) snapshot() string {
	tp.t.Helper()
	var b strings.Builder
	for _, path := range []string{tp.proj.ConfigPath, tp.proj.LockPath(), tp.proj.OLSJSONPath()} {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				b.WriteString(path + ":absent\n")
				continue
			}
			tp.t.Fatalf("gitdep: cannot snapshot %s: %v", path, err)
		}
		b.WriteString(path + ":" + string(data) + "\n")
	}
	entries, err := os.ReadDir(tp.proj.VendorDir())
	if err != nil {
		if os.IsNotExist(err) {
			b.WriteString("vendor:absent\n")
		} else {
			tp.t.Fatalf("gitdep: cannot snapshot vendor/: %v", err)
		}
	} else {
		for _, entry := range entries {
			b.WriteString("vendor/entry:" + entry.Name() + "\n")
		}
	}
	return b.String()
}
