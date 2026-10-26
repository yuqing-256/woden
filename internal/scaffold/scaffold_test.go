package scaffold

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"woden/internal/errs"
	"woden/internal/testutil"
)

// projectFiles is the sorted list of files a complete project contains,
// relative to the project root. The same relative paths are read from
// testdata/golden/new to compare the generated content.
var projectFiles = []string{
	".gitignore",
	"ols.json",
	"src/main.odin",
	"tests/main_test.odin",
	"woden.toml",
}

// projectDirs is the sorted list of directories inside a complete project.
var projectDirs = []string{"src", "tests"}

// goldenContent returns the expected bytes of one generated file. The golden
// fixtures describe a project called "my-game", so the project name is the only
// value that needs substituting.
func goldenContent(t *testing.T, rel, project string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testutil.Testdata(t, "golden/new"), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read golden fixture %s: %v", rel, err)
	}
	if project != "my-game" {
		data = bytes.ReplaceAll(data, []byte("my-game"), []byte(project))
	}
	return data
}

// checkGolden compares every file of a project with the golden fixtures, checks
// that no other files exist and that all generated text uses LF line endings.
func checkGolden(t *testing.T, dir, project string) {
	t.Helper()
	for _, rel := range projectFiles {
		want := goldenContent(t, rel, project)
		got := mustRead(t, filepath.Join(dir, filepath.FromSlash(rel)))
		if !bytes.Equal(got, want) {
			t.Errorf("%s does not match testdata/golden/new/%s\n got: %q\nwant: %q", rel, rel, got, want)
		}
	}
	files, dirs := walkTree(t, dir)
	if !slices.Equal(files, projectFiles) {
		t.Errorf("project files = %v, want %v", files, projectFiles)
	}
	if !slices.Equal(dirs, projectDirs) {
		t.Errorf("project directories = %v, want %v", dirs, projectDirs)
	}
	assertNoCR(t, dir)
}

// mustRead reads path and fails the test when it cannot be read.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

// walkTree returns the files and directories below dir as sorted,
// slash-separated paths relative to dir.
func walkTree(t *testing.T, dir string) (files, dirs []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			dirs = append(dirs, rel)
		} else {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files, dirs
}

// assertNoCR asserts that every file below dir uses LF line endings. Generated
// files are text, so a carriage return anywhere is a bug.
func assertNoCR(t *testing.T, dir string) {
	t.Helper()
	files, _ := walkTree(t, dir)
	for _, rel := range files {
		data := mustRead(t, filepath.Join(dir, filepath.FromSlash(rel)))
		if bytes.ContainsRune(data, '\r') {
			t.Errorf("%s contains a carriage return; generated files must use LF", rel)
		}
	}
}

// newProjectDir returns the path of a project directory that does not exist yet.
func newProjectDir(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testutil.TempDir(t), name)
}

func TestNewCreatesGoldenProject(t *testing.T) {
	target := filepath.Join(testutil.TempDir(t), "nested", "my-game")
	if err := New(target); err != nil {
		t.Fatalf("New(%q) = %v, want nil", target, err)
	}
	checkGolden(t, target, "my-game")

	if info, err := os.Stat(filepath.Dir(target)); err != nil || !info.IsDir() {
		t.Errorf("parent directory %s was not created: %v", filepath.Dir(target), err)
	}
}

func TestNewIntoExistingEmptyDir(t *testing.T) {
	target := newProjectDir(t, "my-game")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create empty target: %v", err)
	}
	if err := New(target); err != nil {
		t.Fatalf("New(%q) = %v, want nil", target, err)
	}
	checkGolden(t, target, "my-game")
}

func TestNewRejectsNonEmptyDir(t *testing.T) {
	target := newProjectDir(t, "my-game")
	testutil.WriteFile(t, filepath.Join(target, "notes.txt"), "keep me")

	err := New(target)
	if err == nil {
		t.Fatal("New() = nil, want an error for a non-empty directory")
	}
	if !errs.IsUser(err) {
		t.Errorf("New() error = %v, want a user error", err)
	}
	if got := testutil.ReadFile(t, filepath.Join(target, "notes.txt")); got != "keep me" {
		t.Errorf("existing file changed to %q", got)
	}
	if files, dirs := walkTree(t, target); !slices.Equal(files, []string{"notes.txt"}) || len(dirs) != 0 {
		t.Errorf("target was modified: files %v, dirs %v", files, dirs)
	}
}

func TestNewRejectsInvalidProjectName(t *testing.T) {
	for _, name := range []string{"1bad", "bad name", "con"} {
		t.Run(name, func(t *testing.T) {
			target := newProjectDir(t, name)
			err := New(target)
			if err == nil {
				t.Fatalf("New(%q) = nil, want an error for an invalid project name", target)
			}
			if !errs.IsUser(err) {
				t.Errorf("New() error = %v, want a user error", err)
			}
			if !strings.Contains(err.Error(), "rename the directory") {
				t.Errorf("New() error = %q, want it to tell the user to rename the directory", err)
			}
			if _, statErr := os.Lstat(target); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("target %s exists, want nothing to be created (stat error %v)", target, statErr)
			}
		})
	}
}

func TestNewRejectsSymlinkTarget(t *testing.T) {
	realDir := filepath.Join(testutil.TempDir(t), "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatalf("create real directory: %v", err)
	}
	link := filepath.Join(testutil.TempDir(t), "my-game")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("cannot create a symbolic link on this system: %v", err)
	}

	err := New(link)
	if err == nil {
		t.Fatal("New(symlink) = nil, want an error")
	}
	if !errs.IsUser(err) {
		t.Errorf("New() error = %v, want a user error", err)
	}
	if files, dirs := walkTree(t, realDir); len(files) != 0 || len(dirs) != 0 {
		t.Errorf("files were created through the link: files %v, dirs %v", files, dirs)
	}
}

func TestNewUsesDirectoryNameAsProjectName(t *testing.T) {
	target := newProjectDir(t, "other-name")
	if err := New(target); err != nil {
		t.Fatalf("New(%q) = %v, want nil", target, err)
	}
	checkGolden(t, target, "other-name")

	manifest := string(mustRead(t, filepath.Join(target, "woden.toml")))
	if !strings.Contains(manifest, `name = "other-name"`) {
		t.Errorf("woden.toml = %q, want name = \"other-name\"", manifest)
	}
	if strings.Contains(manifest, "my-game") {
		t.Errorf("woden.toml still contains the template project name: %q", manifest)
	}
}

func TestNewRollsBackOnFailure(t *testing.T) {
	for _, failOn := range []string{".gitignore", "ols.json", "woden.toml", "main.odin", "main_test.odin"} {
		t.Run(failOn, func(t *testing.T) {
			parent := filepath.Join(testutil.TempDir(t), "nested")
			target := filepath.Join(parent, "my-game")

			original := writeNew
			t.Cleanup(func() { writeNew = original })
			writeNew = func(path string, data []byte) error {
				if filepath.Base(path) == failOn {
					return errs.Userf("injected failure creating %s", path)
				}
				return original(path, data)
			}

			err := New(target)
			if err == nil {
				t.Fatalf("New() = nil, want the injected failure")
			}
			if !errs.IsUser(err) {
				t.Errorf("New() error = %v, want a user error", err)
			}
			if _, statErr := os.Lstat(target); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("target %s still exists after rollback (stat error %v)", target, statErr)
			}
			if _, statErr := os.Lstat(parent); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("created parent %s still exists after rollback (stat error %v)", parent, statErr)
			}
		})
	}
}

func TestInitCreatesGoldenProject(t *testing.T) {
	target := newProjectDir(t, "my-game")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := Init(target); err != nil {
		t.Fatalf("Init(%q) = %v, want nil", target, err)
	}
	checkGolden(t, target, "my-game")
}

func TestInitDefaultsToCurrentDirectory(t *testing.T) {
	target := newProjectDir(t, "my-game")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}
	testutil.Chdir(t, target)
	if err := Init(""); err != nil {
		t.Fatalf("Init(\"\") = %v, want nil", err)
	}
	checkGolden(t, target, "my-game")
}

func TestInitRejectsInitializedDirectory(t *testing.T) {
	target := newProjectDir(t, "my-game")
	manifestPath := filepath.Join(target, manifestFileName)
	testutil.WriteFile(t, manifestPath, "[package]\nname = \"my-game\"\n")

	err := Init(target)
	if err == nil {
		t.Fatal("Init() = nil, want an error for an initialized directory")
	}
	if !errs.IsUser(err) {
		t.Errorf("Init() error = %v, want a user error", err)
	}
	if got := testutil.ReadFile(t, manifestPath); got != "[package]\nname = \"my-game\"\n" {
		t.Errorf("woden.toml was modified: %q", got)
	}
	if files, dirs := walkTree(t, target); !slices.Equal(files, []string{manifestFileName}) || len(dirs) != 0 {
		t.Errorf("initialized directory was modified: files %v, dirs %v", files, dirs)
	}
}

func TestInitOnlyCreatesMissingFiles(t *testing.T) {
	target := newProjectDir(t, "my-game")
	customOLS := `{"custom":true}`
	customSource := "package app\n"
	customTest := "package tests\n"
	testutil.WriteFile(t, filepath.Join(target, olsFileName), customOLS)
	testutil.WriteFile(t, filepath.Join(target, sourceDirName, "app.odin"), customSource)
	testutil.WriteFile(t, filepath.Join(target, testsDirName, "app_test.odin"), customTest)

	if err := Init(target); err != nil {
		t.Fatalf("Init(%q) = %v, want nil", target, err)
	}
	if got := testutil.ReadFile(t, filepath.Join(target, olsFileName)); got != customOLS {
		t.Errorf("existing ols.json changed to %q", got)
	}
	if got := testutil.ReadFile(t, filepath.Join(target, sourceDirName, "app.odin")); got != customSource {
		t.Errorf("existing source changed to %q", got)
	}
	if got := testutil.ReadFile(t, filepath.Join(target, testsDirName, "app_test.odin")); got != customTest {
		t.Errorf("existing test changed to %q", got)
	}
	for _, rel := range []string{"src/main.odin", "tests/main_test.odin"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("sample file %s was created although the directory has sources", rel)
		}
	}
	if got, want := mustRead(t, filepath.Join(target, manifestFileName)), goldenContent(t, manifestFileName, "my-game"); !bytes.Equal(got, want) {
		t.Errorf("woden.toml = %q, want %q", got, want)
	}
	if got, want := mustRead(t, filepath.Join(target, gitignoreName)), goldenContent(t, gitignoreName, "my-game"); !bytes.Equal(got, want) {
		t.Errorf(".gitignore = %q, want %q", got, want)
	}
}

func TestInitSuppressesSamplesWhenOdinFilesExist(t *testing.T) {
	target := newProjectDir(t, "my-game")
	testutil.WriteFile(t, filepath.Join(target, sourceDirName, "nested", "deep.odin"), "package nested\n")
	testutil.WriteFile(t, filepath.Join(target, testsDirName, "unit", "x_test.odin"), "package unit\n")

	if err := Init(target); err != nil {
		t.Fatalf("Init(%q) = %v, want nil", target, err)
	}
	for _, rel := range []string{"src/main.odin", "tests/main_test.odin"} {
		if _, err := os.Lstat(filepath.Join(target, filepath.FromSlash(rel))); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("sample file %s was created although the directory has sources", rel)
		}
	}
	if got := testutil.ReadFile(t, filepath.Join(target, sourceDirName, "nested", "deep.odin")); got != "package nested\n" {
		t.Errorf("existing nested source changed to %q", got)
	}
}

func TestInitGitignore(t *testing.T) {
	tests := []struct {
		name    string
		exists  bool
		initial string
		want    string
	}{
		{name: "missing file", exists: false, want: "/target/\n/vendor/"},
		{name: "empty file", exists: true, initial: "", want: "/target/\n/vendor/\n"},
		{name: "another entry", exists: true, initial: "build/\n", want: "build/\n/target/\n/vendor/\n"},
		{name: "entry without trailing newline", exists: true, initial: "build/", want: "build/\n/target/\n/vendor/\n"},
		{name: "target entry only", exists: true, initial: "/target/\n", want: "/target/\n/vendor/\n"},
		{name: "vendor entry only", exists: true, initial: "/vendor/", want: "/vendor/\n/target/\n"},
		{name: "complete file", exists: true, initial: "/target/\n/vendor/\n", want: "/target/\n/vendor/\n"},
		{name: "padded entries", exists: true, initial: "  /target/  \n/vendor/\n", want: "  /target/  \n/vendor/\n"},
		{name: "comment without trailing newline", exists: true, initial: "# my ignores", want: "# my ignores\n/target/\n/vendor/\n"},
		{name: "crlf target entry", exists: true, initial: "/target/\r\n", want: "/target/\r\n/vendor/\r\n"},
		{name: "crlf complete file", exists: true, initial: "/vendor/\r\n/target/\r\n", want: "/vendor/\r\n/target/\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := newProjectDir(t, "my-game")
			if err := os.Mkdir(target, 0o755); err != nil {
				t.Fatalf("create target: %v", err)
			}
			gitignorePath := filepath.Join(target, gitignoreName)
			if tt.exists {
				testutil.WriteFile(t, gitignorePath, tt.initial)
			}

			if err := Init(target); err != nil {
				t.Fatalf("Init(%q) = %v, want nil", target, err)
			}
			got := string(mustRead(t, gitignorePath))
			if got != tt.want {
				t.Errorf(".gitignore = %q, want %q", got, tt.want)
			}
			for _, entry := range gitignoreEntries {
				if count := strings.Count(got, entry); count != 1 {
					t.Errorf(".gitignore contains %q %d times, want exactly once: %q", entry, count, got)
				}
			}
		})
	}
}

func TestInitRollsBackOnFailure(t *testing.T) {
	t.Run("gitignore is a directory", func(t *testing.T) {
		target := newProjectDir(t, "my-game")
		if err := os.MkdirAll(filepath.Join(target, gitignoreName), 0o755); err != nil {
			t.Fatalf("create .gitignore directory: %v", err)
		}

		err := Init(target)
		if err == nil {
			t.Fatal("Init() = nil, want an error")
		}
		if !errs.IsUser(err) {
			t.Errorf("Init() error = %v, want a user error", err)
		}
		if files, dirs := walkTree(t, target); len(files) != 0 || !slices.Equal(dirs, []string{gitignoreName}) {
			t.Errorf("rollback left files %v, dirs %v; want only the .gitignore directory", files, dirs)
		}
	})

	t.Run("src is a file", func(t *testing.T) {
		target := newProjectDir(t, "my-game")
		testutil.WriteFile(t, filepath.Join(target, sourceDirName), "not a directory")

		err := Init(target)
		if err == nil {
			t.Fatal("Init() = nil, want an error")
		}
		if !errs.IsUser(err) {
			t.Errorf("Init() error = %v, want a user error", err)
		}
		if files, dirs := walkTree(t, target); !slices.Equal(files, []string{sourceDirName}) || len(dirs) != 0 {
			t.Errorf("rollback left files %v, dirs %v; want only the src file", files, dirs)
		}
		if got := testutil.ReadFile(t, filepath.Join(target, sourceDirName)); got != "not a directory" {
			t.Errorf("existing src file changed to %q", got)
		}
	})
}

func TestInitRejectsMissingDirectory(t *testing.T) {
	target := newProjectDir(t, "my-game")
	err := Init(target)
	if err == nil {
		t.Fatal("Init() = nil, want an error for a missing directory")
	}
	if !errs.IsUser(err) {
		t.Errorf("Init() error = %v, want a user error", err)
	}
	if _, statErr := os.Lstat(target); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("missing target %s was created (stat error %v)", target, statErr)
	}
}

func TestInitRejectsFile(t *testing.T) {
	path := newProjectDir(t, "my-game")
	testutil.WriteFile(t, path, "not a directory")

	err := Init(path)
	if err == nil {
		t.Fatal("Init() = nil, want an error for a file")
	}
	if !errs.IsUser(err) {
		t.Errorf("Init() error = %v, want a user error", err)
	}
}

func TestInitRejectsInvalidDirectoryName(t *testing.T) {
	target := filepath.Join(testutil.TempDir(t), "1bad")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("create target: %v", err)
	}

	err := Init(target)
	if err == nil {
		t.Fatal("Init() = nil, want an error for an invalid project name")
	}
	if !errs.IsUser(err) {
		t.Errorf("Init() error = %v, want a user error", err)
	}
	if !strings.Contains(err.Error(), "rename the directory") {
		t.Errorf("Init() error = %q, want it to tell the user to rename the directory", err)
	}
}
