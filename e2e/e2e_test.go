//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"woden/internal/testutil"
)

// TestNewCreatesGoldenProject checks that woden new produces exactly the
// golden files (byte comparison, docs/DECISIONS.md D-003).
func TestNewCreatesGoldenProject(t *testing.T) {
	parent := t.TempDir()
	requireExit(t, runWoden(t, wodenBin, parent, "new", "my-game"), 0, "woden new my-game")
	requireGoldenNew(t, filepath.Join(parent, "my-game"))
}

// TestNewRejectsNonEmptyTarget checks the README rule that the target must not
// exist or must be empty.
func TestNewRejectsNonEmptyTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "my-game")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("e2e: cannot create %s: %v", target, err)
	}
	if err := os.WriteFile(filepath.Join(target, "existing.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("e2e: cannot write marker file: %v", err)
	}
	requireExit(t, runWoden(t, wodenBin, filepath.Dir(target), "new", "my-game"), 101, "woden new into a non-empty directory")
	if got, err := os.ReadFile(filepath.Join(target, "existing.txt")); err != nil || string(got) != "keep" {
		t.Errorf("e2e: existing file was modified: content=%q err=%v", got, err)
	}
}

// TestNewRejectsInvalidProjectName checks the README naming rule.
func TestNewRejectsInvalidProjectName(t *testing.T) {
	requireExit(t, runWoden(t, wodenBin, t.TempDir(), "new", "1bad"), 101, "woden new 1bad")
}

// TestInitCreatesGoldenProject checks woden init on an existing empty directory
// and that a second init refuses because woden.toml already exists.
func TestInitCreatesGoldenProject(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "my-game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("e2e: cannot create %s: %v", dir, err)
	}
	requireExit(t, runWoden(t, wodenBin, parent, "init", "my-game"), 0, "woden init my-game")
	requireGoldenNew(t, dir)
	requireExit(t, runWoden(t, wodenBin, parent, "init", "my-game"), 101, "second woden init")
}

// TestCheckAndLint runs the real compiler on the example project.
func TestCheckAndLint(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	requireExit(t, runWoden(t, wodenBin, project, "check"), 0, "woden check")
	requireExit(t, runWoden(t, wodenBin, project, "lint"), 0, "woden lint")
}

// TestWodenTest runs the example project's three tests.
func TestWodenTest(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	res := runWoden(t, wodenBin, project, "test")
	requireExit(t, res, 0, "woden test")
	// The exact odin test output format is version-dependent (FIXTURES §4 marks
	// it "待实测"); pin only the information the suite is about.
	if !regexp.MustCompile(`(?i)3 tests?`).MatchString(res.combined()) {
		t.Errorf("e2e: expected odin to report three tests, output:\n%s", res.combined())
	}
}

// TestBuildAndRelease checks both profiles and the platform artifact names.
func TestBuildAndRelease(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)

	requireExit(t, runWoden(t, wodenBin, project, "build"), 0, "woden build")
	if _, err := os.Stat(artifactPath(project, "debug", "woden-example")); err != nil {
		t.Errorf("e2e: build artifact is missing: %v", err)
	}

	requireExit(t, runWoden(t, wodenBin, project, "build", "--release"), 0, "woden build --release")
	if _, err := os.Stat(artifactPath(project, "release", "woden-example")); err != nil {
		t.Errorf("e2e: release build artifact is missing: %v", err)
	}
}

// TestRunPrintsGreetingAndKeepsArtifact checks the first two output lines and
// that the shared dev artifact survives (README line 163).
func TestRunPrintsGreetingAndKeepsArtifact(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	res := runWoden(t, wodenBin, project, "run")
	requireExit(t, res, 0, "woden run")
	lines := firstLines(res.stdout, 2)
	if len(lines) < 2 || lines[0] != "Hello, woden-example!" || lines[1] != "2 + 3 = 5" {
		t.Fatalf("e2e: unexpected first lines, got %q\nfull stdout:\n%s\nstderr:\n%s", lines, res.stdout, res.stderr)
	}
	if _, err := os.Stat(artifactPath(project, "debug", "woden-example")); err != nil {
		t.Errorf("e2e: woden run did not keep the executable: %v", err)
	}
}

// TestRunPassesArguments checks `woden run -- a "b c"`.
func TestRunPassesArguments(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	res := runWoden(t, wodenBin, project, "run", "--", "a", "b c")
	requireExit(t, res, 0, `woden run -- a "b c"`)
	for _, want := range []string{"arg[0]=a", "arg[1]=b c"} {
		if !strings.Contains(res.combined(), want) {
			t.Errorf("e2e: output does not contain %q:\n%s", want, res.combined())
		}
	}
}

// TestRunPassesExitCode checks that the program's exit code is passed through.
func TestRunPassesExitCode(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	res := runWoden(t, wodenBin, project, "run", "--", "--exit", "3")
	if res.code != 3 {
		t.Fatalf("e2e: woden run -- --exit 3: exit code %d, want 3\nstdout:\n%s\nstderr:\n%s", res.code, res.stdout, res.stderr)
	}
}

// TestFmtKeepsHashes checks that woden fmt does not change already formatted
// files. The odinfmt.json workaround is required on Windows (AUD-014, D-005).
func TestFmtKeepsHashes(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	writeODINFmtLFConfig(t, project)
	before := snapshotFiles(t, project)
	requireExit(t, runWoden(t, wodenBin, project, "fmt"), 0, "woden fmt")
	after := snapshotFiles(t, project)
	requireSnapshotEqual(t, before, after, "woden fmt")
}

// TestFmtOnlyRewritesSrcSubtree reproduces the FIXTURES §4 mutation: garbling
// src/greet/greet.odin and libs/mathx/mathx.odin must end with only the src
// file formatted; libs/ is project-owned and never touched.
func TestFmtOnlyRewritesSrcSubtree(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	writeODINFmtLFConfig(t, project)

	rel := map[string]string{
		"greet": "src/greet/greet.odin",
		"mathx": "libs/mathx/mathx.odin",
		"main":  "src/main.odin",
		"tests": "tests/main_test.odin",
	}
	original := make(map[string]string, len(rel))
	for key, path := range rel {
		original[key] = readProjectFile(t, project, path)
	}

	garble := func(s string) string { return strings.ReplaceAll(s, "\t", "    ") }
	garbledGreet := garble(original["greet"])
	garbledMathx := garble(original["mathx"])
	if garbledGreet == original["greet"] || garbledMathx == original["mathx"] {
		t.Fatal("e2e: mutation did not change the files")
	}
	writeProjectFile(t, project, rel["greet"], garbledGreet)
	writeProjectFile(t, project, rel["mathx"], garbledMathx)

	requireExit(t, runWoden(t, wodenBin, project, "fmt"), 0, "woden fmt (mutation)")

	if got := readProjectFile(t, project, rel["greet"]); got != original["greet"] {
		t.Errorf("e2e: src/greet/greet.odin was not formatted back to its canonical form:\n%s", got)
	}
	if got := readProjectFile(t, project, rel["mathx"]); got != garbledMathx {
		t.Errorf("e2e: libs/mathx/mathx.odin was modified; woden fmt must not touch libs/")
	}
	if got := readProjectFile(t, project, rel["main"]); got != original["main"] {
		t.Errorf("e2e: src/main.odin changed unexpectedly")
	}
	if got := readProjectFile(t, project, rel["tests"]); got != original["tests"] {
		t.Errorf("e2e: tests/main_test.odin changed unexpectedly")
	}
}

// TestCheckFailsOnTypeErrorMutation reproduces the FIXTURES §4 type-error
// mutation.
func TestCheckFailsOnTypeErrorMutation(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	const path = "src/greet/greet.odin"
	original := readProjectFile(t, project, path)
	broken := strings.Replace(original, `return fmt.aprintf("Hello, %s!", name)`, "return 42", 1)
	if broken == original {
		t.Fatal("e2e: type-error mutation did not apply")
	}
	writeProjectFile(t, project, path, broken)
	requireExit(t, runWoden(t, wodenBin, project, "check"), 101, "woden check with a type error")
}

// TestLintFailsOnUnusedVariableMutation checks that -vet catches unused
// variables while check alone succeeds.
func TestLintFailsOnUnusedVariableMutation(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	const path = "src/main.odin"
	original := readProjectFile(t, project, path)
	mutated := strings.Replace(original, "main :: proc() {\n", "main :: proc() {\n\tunused_variable := 1\n", 1)
	if mutated == original {
		t.Fatal("e2e: unused-variable mutation did not apply")
	}
	writeProjectFile(t, project, path, mutated)
	requireExit(t, runWoden(t, wodenBin, project, "check"), 0, "woden check with an unused variable")
	requireExit(t, runWoden(t, wodenBin, project, "lint"), 101, "woden lint with an unused variable")
}

// TestTestFailsOnWrongExpectationMutation checks that a failing test exits 101.
func TestTestFailsOnWrongExpectationMutation(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	const path = "tests/main_test.odin"
	original := readProjectFile(t, project, path)
	broken := strings.Replace(original, "testing.expect_value(t, mathx.add(2, 3), 5)", "testing.expect_value(t, mathx.add(2, 3), 6)", 1)
	if broken == original {
		t.Fatal("e2e: test-failure mutation did not apply")
	}
	writeProjectFile(t, project, path, broken)
	requireExit(t, runWoden(t, wodenBin, project, "test"), 101, "woden test with a failing expectation")
}

// TestCheckFailsWhenLibsCollectionMissing checks that removing the libs
// mapping breaks the import of libs:mathx.
func TestCheckFailsWhenLibsCollectionMissing(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	const path = "woden.toml"
	original := readProjectFile(t, project, path)
	mutated := strings.Replace(original, "libs = \"libs\"\n", "", 1)
	if mutated == original {
		t.Fatal("e2e: collection mutation did not apply")
	}
	writeProjectFile(t, project, path, mutated)
	requireExit(t, runWoden(t, wodenBin, project, "check"), 101, "woden check without the libs collection")
}

// TestCheckPrefersSyncWhenVendorMissing checks the preflight error for a
// configured dependency without a vendor tree.
func TestCheckPrefersSyncWhenVendorMissing(t *testing.T) {
	requireRealTools(t)
	project := testutil.CopyExample(t)
	const path = "woden.toml"
	original := readProjectFile(t, project, path)
	mutated := original + "\n[dependencies.deplib]\ngit = \"https://example.invalid/deplib.git\"\n"
	writeProjectFile(t, project, path, mutated)
	res := runWoden(t, wodenBin, project, "check")
	requireExit(t, res, 101, "woden check with a dependency and no vendor/")
	if !strings.Contains(strings.ToLower(res.combined()), "sync") {
		t.Errorf("e2e: expected the error to suggest `woden sync`, got:\n%s", res.combined())
	}
}

// TestSyncCheckSucceeds checks the read-only consistency command on the
// unmodified example project.
func TestSyncCheckSucceeds(t *testing.T) {
	project := testutil.CopyExample(t)
	requireExit(t, runWoden(t, wodenBin, project, "sync", "--check"), 0, "woden sync --check")
}

// TestCleanDryRunKeepsTarget checks that --dry-run lists entries without
// changing anything.
func TestCleanDryRunKeepsTarget(t *testing.T) {
	project := testutil.CopyExample(t)
	writeProjectFile(t, project, "target/debug/deps/leftover.exe", "stale")
	writeProjectFile(t, project, "target/debug/"+exeName("woden-example"), "stale")
	before := snapshotFiles(t, project)

	res := runWoden(t, wodenBin, project, "clean", "--dry-run")
	requireExit(t, res, 0, "woden clean --dry-run")
	after := snapshotFiles(t, project)
	requireSnapshotEqual(t, before, after, "woden clean --dry-run")
	if _, err := os.Stat(filepath.Join(project, "target", "debug", "deps", "leftover.exe")); err != nil {
		t.Errorf("e2e: clean --dry-run deleted target content: %v", err)
	}
	lower := strings.ToLower(res.combined())
	if !strings.Contains(lower, "target") && !strings.Contains(lower, "debug") && !strings.Contains(lower, "leftover") {
		t.Errorf("e2e: clean --dry-run did not list the target entries, output:\n%s", res.combined())
	}
}

// TestCleanRemovesTarget checks that a real clean removes target/ and leaves
// every other file untouched.
func TestCleanRemovesTarget(t *testing.T) {
	project := testutil.CopyExample(t)
	writeProjectFile(t, project, "target/debug/deps/leftover.exe", "stale")
	written := snapshotFiles(t, project)
	// The sources must be byte-identical after clean; take a snapshot without
	// the newly created target files.
	requireExit(t, runWoden(t, wodenBin, project, "clean"), 0, "woden clean")
	if _, err := os.Stat(filepath.Join(project, "target")); !os.IsNotExist(err) {
		t.Errorf("e2e: target/ still exists after woden clean: %v", err)
	}
	after := snapshotFiles(t, project)
	requireSnapshotEqual(t, written, after, "woden clean")
}
