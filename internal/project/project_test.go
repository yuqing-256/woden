package project_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"woden/internal/errs"
	"woden/internal/project"
	"woden/internal/testutil"
)

func newProject(t *testing.T, fixture string) string {
	t.Helper()
	root := t.TempDir()
	data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("toml", fixture)))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	if err := os.WriteFile(filepath.Join(root, "woden.toml"), data, 0o644); err != nil {
		t.Fatalf("write woden.toml: %v", err)
	}
	return root
}

func TestFindRootWalksUp(t *testing.T) {
	root := newProject(t, "vaild-minimal.toml")
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := project.FindRoot(nested)
	if err != nil {
		t.Fatalf("FindRoot: %v", err)
	}
	if filepath.Clean(got) != filepath.Clean(root) {
		t.Fatalf("FindRoot = %q, want %q", got, root)
	}

	if _, err := project.FindRoot(t.TempDir()); err == nil {
		t.Fatal("FindRoot outside a project should fail")
	} else if !errs.IsUser(err) || !strings.Contains(err.Error(), "no woden.toml") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadMinimalProject(t *testing.T) {
	root := newProject(t, "vaild-minimal.toml")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatalf("mkdir tests: %v", err)
	}

	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Root != filepath.Clean(root) {
		t.Fatalf("root = %q, want %q", p.Root, filepath.Clean(root))
	}
	if p.Config.Package.Name != "my-game" {
		t.Fatalf("package name = %q", p.Config.Package.Name)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings = %v, want none", p.Warnings)
	}

	got := p.CollectionArgs()
	want := []string{"-collection:app=" + filepath.Join(root, "src")}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("CollectionArgs = %v, want %v", got, want)
	}
	wantExe := "my-game"
	if runtime.GOOS == "windows" {
		wantExe += ".exe"
	}
	if got := p.ExecutableName(); got != wantExe {
		t.Fatalf("ExecutableName = %q, want %q", got, wantExe)
	}
	if err := p.CheckVendor(); err != nil {
		t.Fatalf("CheckVendor without dependencies = %v, want nil", err)
	}
	if dirs := p.SourceAndTestDirs(); len(dirs) != 2 || dirs[0] != "src" || dirs[1] != "tests" {
		t.Fatalf("SourceAndTestDirs = %v, want [src tests]", dirs)
	}
	if _, err := p.Path("../escape"); err == nil {
		t.Fatal("Path must reject escapes")
	}
}

func TestLoadFullProjectWarningsAndVendor(t *testing.T) {
	root := newProject(t, "vaild-full.toml")
	p, err := project.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !p.HasDependencies() {
		t.Fatal("HasDependencies = false")
	}
	if got := len(p.Dependencies()); got != 4 {
		t.Fatalf("dependency count = %d, want 4", got)
	}
	err = p.CheckVendor()
	if err == nil {
		t.Fatal("CheckVendor should fail when vendor/ is missing")
	}
	if !strings.Contains(err.Error(), "woden sync") {
		t.Fatalf("CheckVendor error = %v, want it to mention woden sync", err)
	}

	warnRoot := newProject(t, "unknown-key.toml")
	wp, err := project.Load(warnRoot)
	if err != nil {
		t.Fatalf("Load with unknown keys: %v", err)
	}
	if got := len(wp.Warnings); got != 2 {
		t.Fatalf("warnings = %v, want 2", wp.Warnings)
	}
}

func TestLoadInvalidProject(t *testing.T) {
	root := newProject(t, "bad-syntax.toml")
	if _, err := project.Load(root); err == nil {
		t.Fatal("Load succeeded on an invalid woden.toml")
	} else if !errs.IsUser(err) {
		t.Fatalf("error is not a user error: %v", err)
	}
}
