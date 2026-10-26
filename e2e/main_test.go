//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	// repoRoot is the absolute path of the repository root (the directory
	// containing go.mod), discovered once in TestMain.
	repoRoot string
	// wodenBin is built without extra tags and is used by every command that
	// does not need file:// Git URLs.
	wodenBin string
	// wodenTestingBin is built with -tags woden_testing, the only build that
	// accepts file:// Git URLs; dependency tests use it.
	wodenTestingBin string
)

// exeName appends the platform executable suffix to a bare name.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// TestMain builds the woden binaries once and runs the tests.
func TestMain(m *testing.M) {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot locate the repository root: %v\n", err)
		os.Exit(1)
	}
	repoRoot = root

	binDir, err := os.MkdirTemp("", "woden-e2e-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot create a temporary bin directory: %v\n", err)
		os.Exit(1)
	}

	wodenBin = filepath.Join(binDir, exeName("woden"))
	if err := buildWoden(wodenBin, nil); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot build woden: %v\n", err)
		_ = os.RemoveAll(binDir)
		os.Exit(1)
	}
	wodenTestingBin = filepath.Join(binDir, exeName("woden-testing"))
	if err := buildWoden(wodenTestingBin, []string{"woden_testing"}); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: cannot build woden with the woden_testing tag: %v\n", err)
		_ = os.RemoveAll(binDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(binDir)
	os.Exit(code)
}

// findRepoRoot walks up from the current directory to the go.mod file.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// buildWoden builds the module root into out with the given build tags.
func buildWoden(out string, tags []string) error {
	args := []string{"build", "-o", out}
	if len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, outBytes)
	}
	return nil
}

// commandResult is the outcome of one woden invocation.
type commandResult struct {
	stdout string
	stderr string
	code   int
}

// combined returns stdout and stderr concatenated for case-insensitive checks.
func (r commandResult) combined() string {
	return r.stdout + r.stderr
}

// runWoden runs bin with args in dir and returns its streams and exit code.
func runWoden(t *testing.T, bin, dir string, args ...string) commandResult {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := commandResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return res
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.code = exitErr.ExitCode()
		return res
	}
	t.Fatalf("e2e: running %s %s in %s failed: %v", bin, strings.Join(args, " "), dir, err)
	return res
}

// requireExit fails the test when res.code is not want.
func requireExit(t *testing.T, res commandResult, want int, what string) {
	t.Helper()
	if res.code != want {
		t.Fatalf("%s: exit code %d, want %d\nstdout:\n%s\nstderr:\n%s",
			what, res.code, want, res.stdout, res.stderr)
	}
}

// requireRealTools skips the test when the real Odin toolchain is missing.
func requireRealTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"odin", "odinfmt"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("real odin/odinfmt not found in PATH (missing %s)", tool)
		}
	}
}

// requireGit skips the test when git is missing.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("real git not found in PATH")
	}
}

// odinfmtLFConfig forces LF output on odinfmt (see docs/DECISIONS.md D-005,
// AUD-014). odinfmt defaults to CRLF on Windows, which would rewrite every
// formatted file even when the formatting is unchanged.
const odinfmtLFConfig = "{\n  \"newline_style\": \"LF\"\n}\n"

// writeODINFmtLFConfig writes the LF-forcing odinfmt.json into a temporary
// project copy. It is only used by the fmt tests; Woden itself never manages
// odinfmt.json.
func writeODINFmtLFConfig(t *testing.T, project string) {
	t.Helper()
	path := filepath.Join(project, "odinfmt.json")
	if err := os.WriteFile(path, []byte(odinfmtLFConfig), 0o644); err != nil {
		t.Fatalf("e2e: cannot write %s: %v", path, err)
	}
}

// targetDirName is the directory Woden owns; snapshots skip it.
const targetDirName = "target"

// snapshotFiles returns SHA-256 hashes of every regular file under dir,
// keyed by slash-separated relative path, skipping the target/ tree.
func snapshotFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != dir && entry.Name() == targetDirName {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		snapshot[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("e2e: cannot snapshot %s: %v", dir, err)
	}
	return snapshot
}

// requireSnapshotEqual fails when after differs from before (changed, added or
// removed files).
func requireSnapshotEqual(t *testing.T, before, after map[string]string, what string) {
	t.Helper()
	for name, want := range before {
		got, ok := after[name]
		if !ok {
			t.Errorf("%s: file %s disappeared", what, name)
			continue
		}
		if got != want {
			t.Errorf("%s: file %s changed", what, name)
		}
	}
	for name := range after {
		if _, ok := before[name]; !ok {
			t.Errorf("%s: unexpected file %s", what, name)
		}
	}
}

// goldenNewFiles lists the files generated by woden new/init, relative to the
// project root. The golden bytes are authoritative (docs/DECISIONS.md D-003).
var goldenNewFiles = []string{
	".gitignore",
	"woden.toml",
	"ols.json",
	filepath.FromSlash("src/main.odin"),
	filepath.FromSlash("tests/main_test.odin"),
}

// requireGoldenNew byte-compares the generated project against
// testdata/golden/new/ and fails on unexpected files.
func requireGoldenNew(t *testing.T, project string) {
	t.Helper()
	goldenDir := filepath.Join(repoRoot, "testdata", "golden", "new")
	for _, rel := range goldenNewFiles {
		want, err := os.ReadFile(filepath.Join(goldenDir, rel))
		if err != nil {
			t.Fatalf("e2e: cannot read golden file %s: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(project, rel))
		if err != nil {
			t.Errorf("e2e: generated file %s is missing: %v", rel, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("e2e: generated %s differs from testdata/golden/new/%s\n got %d bytes: %q\nwant %d bytes: %q",
				rel, rel, len(got), got, len(want), want)
		}
	}

	known := make(map[string]bool, len(goldenNewFiles))
	for _, rel := range goldenNewFiles {
		known[filepath.ToSlash(rel)] = true
	}
	var extra []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(project, path)
		if err != nil {
			return err
		}
		if !known[filepath.ToSlash(rel)] {
			extra = append(extra, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("e2e: cannot walk generated project: %v", err)
	}
	if len(extra) > 0 {
		t.Errorf("e2e: generated project contains unexpected files: %v", extra)
	}
}

// artifactPath returns the expected build artifact path for a package name.
func artifactPath(project, profileDir, name string) string {
	return filepath.Join(project, targetDirName, profileDir, exeName(name))
}

// readProjectFile reads rel (slash separated) inside project.
func readProjectFile(t *testing.T, project, rel string) string {
	t.Helper()
	path := filepath.Join(project, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("e2e: cannot read %s: %v", rel, err)
	}
	return string(data)
}

// writeProjectFile writes content to rel (slash separated) inside project.
func writeProjectFile(t *testing.T, project, rel, content string) {
	t.Helper()
	path := filepath.Join(project, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("e2e: cannot create directory for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("e2e: cannot write %s: %v", rel, err)
	}
}

// firstLines normalises CRLF and returns the first n non-empty-tail lines.
func firstLines(s string, n int) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return lines
}
