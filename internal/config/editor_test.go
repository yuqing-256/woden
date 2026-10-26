package config_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/testutil"
)

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(testutil.Testdata(t, rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return data
}

func TestNewEditorRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"bom", append([]byte("\xef\xbb\xbf"), []byte("[package]\nname = \"x\"\n")...), "BOM"},
		{"mixed endings", []byte("[package]\r\nname = \"x\"\n"), "mixes CRLF and LF"},
		{"inline collections", readFixture(t, filepath.Join("toml", "edit", "bad-structure.toml")), "inline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.NewEditor(tt.data)
			if err == nil {
				t.Fatal("NewEditor succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestAddFirstDependency(t *testing.T) {
	base := readFixture(t, filepath.Join("toml", "edit", "base.toml"))
	want := readFixture(t, filepath.Join("toml", "edit", "add-first-dep.want.toml"))

	ed, err := config.NewEditor(base)
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.SetCollection("deps", "vendor"); err != nil {
		t.Fatalf("SetCollection: %v", err)
	}
	if err := ed.AddDependency(config.Dependency{
		Name:   "example-library",
		Git:    "https://github.com/example/example-library.git",
		Branch: "main",
	}); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if got := ed.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("edited file = %q, want %q", got, want)
	}
}

func TestAddFirstDependencyCRLF(t *testing.T) {
	base := testutil.ToCRLF(string(readFixture(t, filepath.Join("toml", "edit", "base.toml"))))
	want := testutil.ToCRLF(string(readFixture(t, filepath.Join("toml", "edit", "add-first-dep.want.toml"))))

	ed, err := config.NewEditor([]byte(base))
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.SetCollection("deps", "vendor"); err != nil {
		t.Fatalf("SetCollection: %v", err)
	}
	if err := ed.AddDependency(config.Dependency{
		Name:   "example-library",
		Git:    "https://github.com/example/example-library.git",
		Branch: "main",
	}); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if got := string(ed.Bytes()); got != want {
		t.Fatalf("CRLF edited file = %q, want %q", got, want)
	}
}

func TestRemoveDependencyDropsCommentBlock(t *testing.T) {
	two := readFixture(t, filepath.Join("toml", "edit", "two-deps.toml"))
	want := readFixture(t, filepath.Join("toml", "edit", "remove-alpha.want.toml"))

	ed, err := config.NewEditor(two)
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.RemoveDependency("alpha"); err != nil {
		t.Fatalf("RemoveDependency: %v", err)
	}
	if got := ed.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("edited file = %q, want %q", got, want)
	}
	if err := ed.RemoveDependency("does-not-exist"); err != nil {
		t.Fatalf("removing an absent dependency should be a no-op, got %v", err)
	}
}

func TestSetCollectionKeepsInlineComment(t *testing.T) {
	input := "[package]\nname = \"my-game\"\nversion = \"0.1.0\"\n\n[collections]\napp = \"src\" # keep me\n"
	want := "[package]\nname = \"my-game\"\nversion = \"0.1.0\"\n\n[collections]\napp = \"libs\" # keep me\n"

	ed, err := config.NewEditor([]byte(input))
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.SetCollection("app", "libs"); err != nil {
		t.Fatalf("SetCollection: %v", err)
	}
	if got := string(ed.Bytes()); got != want {
		t.Fatalf("edited file = %q, want %q", got, want)
	}
	if !ed.Changed() {
		t.Fatal("Changed = false after an edit")
	}
}

func TestSetCollectionCreatesTable(t *testing.T) {
	input := "[package]\nname = \"my-game\"\nversion = \"0.1.0\"\n"
	ed, err := config.NewEditor([]byte(input))
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.SetCollection("app", "src"); err != nil {
		t.Fatalf("SetCollection: %v", err)
	}
	got := string(ed.Bytes())
	if !strings.Contains(got, "[collections]") || !strings.Contains(got, `app = "src"`) {
		t.Fatalf("edited file = %q, want a [collections] table", got)
	}
	cfg, _, err := config.Decode(ed.Bytes())
	if err != nil {
		t.Fatalf("edited file no longer decodes: %v", err)
	}
	if cfg.Collections["app"] != "src" {
		t.Fatalf("collections[app] = %q", cfg.Collections["app"])
	}
}

func TestRemoveCollection(t *testing.T) {
	input := "[package]\nname = \"my-game\"\nversion = \"0.1.0\"\n\n[collections]\napp = \"src\"\nlibs = \"libs\"\n"
	ed, err := config.NewEditor([]byte(input))
	if err != nil {
		t.Fatalf("NewEditor: %v", err)
	}
	if err := ed.RemoveCollection("libs"); err != nil {
		t.Fatalf("RemoveCollection: %v", err)
	}
	got := string(ed.Bytes())
	if strings.Contains(got, "libs = ") {
		t.Fatalf("edited file = %q, want libs removed", got)
	}
	if !strings.Contains(got, `app = "src"`) {
		t.Fatalf("edited file = %q, want app kept", got)
	}
	if err := ed.RemoveCollection("absent"); err != nil {
		t.Fatalf("removing an absent collection should be a no-op, got %v", err)
	}
}

func TestEditWritesFileAndSkipsUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "woden.toml")
	want := readFixture(t, filepath.Join("toml", "edit", "add-first-dep.want.toml"))
	if err := os.WriteFile(path, readFixture(t, filepath.Join("toml", "edit", "base.toml")), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if err := config.Edit(path, func(ed *config.Editor) error {
		if err := ed.SetCollection("deps", "vendor"); err != nil {
			return err
		}
		return ed.AddDependency(config.Dependency{
			Name:   "example-library",
			Git:    "https://github.com/example/example-library.git",
			Branch: "main",
		})
	}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("edited file = %q, want %q", got, want)
	}

	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := config.Edit(path, func(ed *config.Editor) error { return nil }); err != nil {
		t.Fatalf("no-op Edit: %v", err)
	}
	if err := config.Edit(path, func(ed *config.Editor) error { return ed.RemoveDependency("example-library") }); err != nil {
		t.Fatalf("Edit remove: %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("permissions changed from %o to %o", before.Mode().Perm(), after.Mode().Perm())
	}
}
