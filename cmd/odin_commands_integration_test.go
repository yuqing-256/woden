package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/testutil"
	"github.com/yuqing-256/woden/internal/testutil/faketool"
)

// The tests in this file run a freshly built woden binary against fake odin and
// odinfmt executables. They are integration tests: each command contract is
// verified by asserting the exact process invocation Woden makes, where it runs
// it and which exit code it reports.

// odinCommandTestEnv is a temporary environment with a freshly built woden
// binary and fake odin/odinfmt executables on PATH.
type odinCommandTestEnv struct {
	woden   string
	toolDir string
	logPath string
}

// odinCommandResult is the outcome of one woden invocation.
type odinCommandResult struct {
	stdout string
	stderr string
	code   int
}

// newOdinCommandTestEnv builds the woden binary, installs the fake tools and
// prepares a fresh invocation log.
func newOdinCommandTestEnv(t *testing.T) *odinCommandTestEnv {
	t.Helper()
	root := testutil.RepoRoot(t)
	toolDir := t.TempDir()
	woden := odinTestBuildWoden(t, root, toolDir)
	faketool.Install(t, root, toolDir)
	return &odinCommandTestEnv{
		woden:   woden,
		toolDir: toolDir,
		logPath: filepath.Join(t.TempDir(), "invocations.jsonl"),
	}
}

// env returns the environment for running woden with the fake tools exiting
// with exitCode.
func (e *odinCommandTestEnv) env(exitCode int) []string {
	return faketool.Env(e.toolDir, e.logPath, exitCode)
}

// project returns a temporary copy of example/.
func (e *odinCommandTestEnv) project(t *testing.T) string {
	t.Helper()
	return testutil.CopyExample(t)
}

// run executes woden in dir with args and returns its output and exit code.
func (e *odinCommandTestEnv) run(t *testing.T, dir string, exitCode int, args ...string) odinCommandResult {
	t.Helper()
	return e.runWithEnv(t, dir, e.env(exitCode), args...)
}

// runWithEnv executes woden in dir with an explicit environment.
func (e *odinCommandTestEnv) runWithEnv(t *testing.T, dir string, environment []string, args ...string) odinCommandResult {
	t.Helper()
	cmd := exec.Command(e.woden, args...)
	cmd.Dir = dir
	cmd.Env = environment
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("cannot run woden: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return odinCommandResult{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

// invocations returns the fake tool invocations logged so far.
func (e *odinCommandTestEnv) invocations(t *testing.T) []faketool.Record {
	t.Helper()
	return faketool.Invocations(t, e.logPath)
}

// odinTestBuildWoden builds the woden command into dir and returns its path.
func odinTestBuildWoden(t *testing.T, moduleRoot, dir string) string {
	t.Helper()
	out := filepath.Join(dir, "woden"+odinTestExeSuffix())
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = moduleRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cannot build woden: %v\n%s", err, output)
	}
	return out
}

// odinTestExeSuffix returns the executable extension of the host platform.
func odinTestExeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// odinTestCollectionArgs returns the expected -collection arguments for the
// given name/path pairs, resolved against root.
func odinTestCollectionArgs(root string, pairs ...string) []string {
	args := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		args = append(args, "-collection:"+pairs[i]+"="+filepath.Join(root, pairs[i+1]))
	}
	return args
}

// odinTestJoin returns prefix followed by extra.
func odinTestJoin(prefix []string, extra ...string) []string {
	out := make([]string, 0, len(prefix)+len(extra))
	out = append(out, prefix...)
	out = append(out, extra...)
	return out
}

// odinTestSamePath compares two paths, ignoring case on Windows.
func odinTestSamePath(got, want string) bool {
	got = odinTestCanonicalPath(got)
	want = odinTestCanonicalPath(want)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(got), filepath.Clean(want))
	}
	return filepath.Clean(got) == filepath.Clean(want)
}

// odinTestSameArg compares two command line arguments, ignoring case on
// Windows. Windows paths are case-insensitive, and the child process may report
// a differently-cased spelling of the directory it runs in.
func odinTestSameArg(got, want string) bool {
	got = odinTestCanonicalTempPath(got)
	want = odinTestCanonicalTempPath(want)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(got, want)
	}
	return got == want
}

// macOS exposes temporary directories through /var and its /private/var
// symlink. Canonicalize only that known alias inside arguments, preserving
// exact checks for argument order, flags and all other path components.
func odinTestCanonicalPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func odinTestCanonicalTempPath(arg string) string {
	if runtime.GOOS != "darwin" {
		return arg
	}
	return strings.ReplaceAll(arg, "/private/var/folders/", "/var/folders/")
}

// odinTestRequireInvocations asserts that exactly want fake tool invocations
// were recorded.
func odinTestRequireInvocations(t *testing.T, invocations []faketool.Record, want int) {
	t.Helper()
	if len(invocations) != want {
		t.Fatalf("recorded %d fake tool invocations, want %d: %+v", len(invocations), want, invocations)
	}
}

// odinTestRequireInvocation asserts one recorded fake tool invocation.
func odinTestRequireInvocation(t *testing.T, invocation faketool.Record, tool, dir string, exitCode int, args ...string) {
	t.Helper()
	if invocation.Tool != tool {
		t.Errorf("invocation tool = %q, want %q", invocation.Tool, tool)
	}
	if !odinTestSamePath(invocation.Dir, dir) {
		t.Errorf("invocation directory = %q, want %q", invocation.Dir, dir)
	}
	if invocation.Exit != exitCode {
		t.Errorf("invocation exit code = %d, want %d", invocation.Exit, exitCode)
	}
	if len(invocation.Args) != len(args) {
		t.Fatalf("arguments = %q, want %q", invocation.Args, args)
	}
	for i := range args {
		if !odinTestSameArg(invocation.Args[i], args[i]) {
			t.Fatalf("arguments = %q, want %q", invocation.Args, args)
		}
	}
}

// odinTestRequireFailure asserts a Woden failure (exit code 101) whose message
// contains want.
func odinTestRequireFailure(t *testing.T, result odinCommandResult, want string) {
	t.Helper()
	if result.code != 101 {
		t.Fatalf("exit code = %d, want 101; stdout: %s, stderr: %s", result.code, result.stdout, result.stderr)
	}
	if !strings.Contains(result.stderr, want) {
		t.Fatalf("stderr = %q, want it to contain %q", result.stderr, want)
	}
}

// odinTestMinimalProject creates a temporary project with only woden.toml.
func odinTestMinimalProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "woden.toml"), testutil.ReadFile(t, testutil.Testdata(t, "golden/new/woden.toml")))
	return dir
}

// TestFmtCommand checks that "woden fmt" formats src/ then tests/, stops on the
// first odinfmt failure and reports it as a Woden failure.
func TestFmtCommand(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)

	result := env.run(t, project, 0, "fmt")
	if result.code != 0 {
		t.Fatalf("woden fmt exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations := env.invocations(t)
	odinTestRequireInvocations(t, invocations, 2)
	odinTestRequireInvocation(t, invocations[0], "odinfmt", project, 0, "src", "-w")
	odinTestRequireInvocation(t, invocations[1], "odinfmt", project, 0, "tests", "-w")

	// An odinfmt failure on src/ stops before tests/ and exits with 101.
	result = env.run(t, project, 1, "fmt")
	odinTestRequireFailure(t, result, "odinfmt exited with code 1")
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 3)
	odinTestRequireInvocation(t, invocations[2], "odinfmt", project, 1, "src", "-w")

	// Neither src/ nor tests/ exists: a user error, and odinfmt is not run.
	bare := odinTestMinimalProject(t)
	result = env.run(t, bare, 0, "fmt")
	odinTestRequireFailure(t, result, "neither src/ nor tests/")
	odinTestRequireInvocations(t, env.invocations(t), 3)
}

// TestCheckAndLintCommand checks the exact "odin check" and "odin check -vet"
// invocations and the failure exit code.
func TestCheckAndLintCommand(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	collections := odinTestCollectionArgs(project, "app", "src", "libs", "libs")

	result := env.run(t, project, 0, "check")
	if result.code != 0 {
		t.Fatalf("woden check exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	result = env.run(t, project, 0, "lint")
	if result.code != 0 {
		t.Fatalf("woden lint exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations := env.invocations(t)
	odinTestRequireInvocations(t, invocations, 2)
	odinTestRequireInvocation(t, invocations[0], "odin", project, 0, odinTestJoin([]string{"check", "src"}, collections...)...)
	odinTestRequireInvocation(t, invocations[1], "odin", project, 0, odinTestJoin([]string{"check", "src", "-vet"}, collections...)...)

	result = env.run(t, project, 1, "check")
	odinTestRequireFailure(t, result, "odin exited with code 1")
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 3)
	odinTestRequireInvocation(t, invocations[2], "odin", project, 1, odinTestJoin([]string{"check", "src"}, collections...)...)
}

// TestTestCommand checks the "odin test" invocation, the created output
// directory, the artifact ownership and the failure exit code.
func TestTestCommand(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	collections := odinTestCollectionArgs(project, "app", "src", "libs", "libs")
	outPath := "target/debug/deps/woden-example" + odinTestExeSuffix()
	want := odinTestJoin([]string{"test", "tests", "-all-packages", "-out:" + outPath, "-debug", "-o:minimal"}, collections...)

	result := env.run(t, project, 0, "test")
	if result.code != 0 {
		t.Fatalf("woden test exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations := env.invocations(t)
	odinTestRequireInvocations(t, invocations, 1)
	odinTestRequireInvocation(t, invocations[0], "odin", project, 0, want...)

	depsDir := filepath.Join(project, "target", "debug", "deps")
	if info, err := os.Stat(depsDir); err != nil || !info.IsDir() {
		t.Fatalf("woden test did not create %s: %v", depsDir, err)
	}
	artifact := filepath.Join(project, "target", "debug", "deps", "woden-example"+odinTestExeSuffix())
	if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the test artifact must be left to Odin, but %s exists (err = %v)", artifact, err)
	}

	result = env.run(t, project, 1, "test")
	odinTestRequireFailure(t, result, "odin exited with code 1")
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 2)
	odinTestRequireInvocation(t, invocations[1], "odin", project, 1, want...)
}

// TestBuildCommand checks both profiles, the output directories, the profile
// override from woden.toml and the failure exit code.
func TestBuildCommand(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	collections := odinTestCollectionArgs(project, "app", "src", "libs", "libs")
	wantDev := odinTestJoin([]string{"build", "src", "-out:target/debug/woden-example" + odinTestExeSuffix(), "-debug", "-o:minimal"}, collections...)
	wantRelease := odinTestJoin([]string{"build", "src", "-out:target/release/woden-example" + odinTestExeSuffix(), "-o:speed"}, collections...)

	result := env.run(t, project, 0, "build")
	if result.code != 0 {
		t.Fatalf("woden build exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	result = env.run(t, project, 0, "build", "--release")
	if result.code != 0 {
		t.Fatalf("woden build --release exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations := env.invocations(t)
	odinTestRequireInvocations(t, invocations, 2)
	odinTestRequireInvocation(t, invocations[0], "odin", project, 0, wantDev...)
	odinTestRequireInvocation(t, invocations[1], "odin", project, 0, wantRelease...)

	for _, dir := range []string{"debug", "release"} {
		path := filepath.Join(project, "target", dir)
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Fatalf("woden build did not create %s: %v", path, err)
		}
	}

	result = env.run(t, project, 1, "build")
	odinTestRequireFailure(t, result, "odin exited with code 1")
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 3)

	// A [profile.dev] opt-level override changes the -o: argument.
	tomlPath := filepath.Join(project, "woden.toml")
	testutil.WriteFile(t, tomlPath, testutil.ReadFile(t, tomlPath)+"\n[profile.dev]\nopt-level = \"none\"\n")
	result = env.run(t, project, 0, "build")
	if result.code != 0 {
		t.Fatalf("woden build with a profile override exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	wantOverride := odinTestJoin([]string{"build", "src", "-out:target/debug/woden-example" + odinTestExeSuffix(), "-debug", "-o:none"}, collections...)
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 4)
	odinTestRequireInvocation(t, invocations[3], "odin", project, 0, wantOverride...)
}

// TestRunCommand checks the "odin run" invocation (absolute paths, the
// invocation directory, program arguments), and that the program's exit code is
// forwarded verbatim.
func TestRunCommand(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	invocationDir := filepath.Join(project, "src")
	collections := odinTestCollectionArgs(project, "app", "src", "libs", "libs")
	outPath := filepath.Join(project, "target", "debug", "woden-example"+odinTestExeSuffix())
	want := odinTestJoin([]string{
		"run", filepath.Join(project, "src"), "-out:" + outPath, "-keep-executable",
		"-debug", "-o:minimal",
	}, collections...)

	result := env.run(t, invocationDir, 0, "run")
	if result.code != 0 {
		t.Fatalf("woden run exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations := env.invocations(t)
	odinTestRequireInvocations(t, invocations, 1)
	odinTestRequireInvocation(t, invocations[0], "odin", invocationDir, 0, want...)
	if info, err := os.Stat(filepath.Join(project, "target", "debug")); err != nil || !info.IsDir() {
		t.Fatalf("woden run did not create target/debug: %v", err)
	}

	// Program arguments after "--" are passed through unchanged.
	result = env.run(t, invocationDir, 0, "run", "--", "a", "b c")
	if result.code != 0 {
		t.Fatalf("woden run -- a \"b c\" exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	result = env.run(t, invocationDir, 0, "run", "--", "--exit", "3")
	if result.code != 0 {
		t.Fatalf("woden run -- --exit 3 exit code = %d, want 0; stderr: %s", result.code, result.stderr)
	}
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 3)
	odinTestRequireInvocation(t, invocations[1], "odin", invocationDir, 0, odinTestJoin(want, "--", "a", "b c")...)
	odinTestRequireInvocation(t, invocations[2], "odin", invocationDir, 0, odinTestJoin(want, "--", "--exit", "3")...)

	// The program's exit code is forwarded verbatim, without an error message.
	result = env.run(t, invocationDir, 7, "run")
	if result.code != 7 {
		t.Fatalf("woden run exit code = %d, want the program's exit code 7", result.code)
	}
	if strings.TrimSpace(result.stderr) != "" {
		t.Fatalf("woden run printed an error for a forwarded exit code: %q", result.stderr)
	}
	invocations = env.invocations(t)
	odinTestRequireInvocations(t, invocations, 4)
	odinTestRequireInvocation(t, invocations[3], "odin", invocationDir, 7, want...)

	// Positional arguments must not appear before "--".
	result = env.run(t, project, 0, "run", "a")
	odinTestRequireFailure(t, result, "pass program arguments after --")
	result = env.run(t, project, 0, "run", "a", "--", "b")
	odinTestRequireFailure(t, result, "pass program arguments after --")
	odinTestRequireInvocations(t, env.invocations(t), 4)
}

// TestVendorPreflight checks that every command that can need the deps
// collection refuses to run, and points at "woden sync", when vendor/ is
// missing.
func TestVendorPreflight(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	tomlPath := filepath.Join(project, "woden.toml")
	testutil.WriteFile(t, tomlPath, testutil.ReadFile(t, tomlPath)+"deps = \"vendor\"\n")

	commands := [][]string{
		{"check"},
		{"lint"},
		{"test"},
		{"build"},
		{"build", "--release"},
		{"run"},
	}
	for _, command := range commands {
		t.Run(strings.Join(command, " "), func(t *testing.T) {
			result := env.run(t, project, 0, command...)
			odinTestRequireFailure(t, result, "woden sync")
		})
	}
	odinTestRequireInvocations(t, env.invocations(t), 0)
}

// TestMissingTools checks the error messages for odin and odinfmt not being on
// PATH.
func TestMissingTools(t *testing.T) {
	env := newOdinCommandTestEnv(t)
	project := env.project(t)
	emptyDir := t.TempDir()
	environment := faketool.WithPath(os.Environ(), emptyDir)

	result := env.runWithEnv(t, project, environment, "check")
	odinTestRequireFailure(t, result, "odin was not found in PATH")

	result = env.runWithEnv(t, project, environment, "fmt")
	odinTestRequireFailure(t, result, "odinfmt was not found in PATH")

	odinTestRequireInvocations(t, env.invocations(t), 0)
}
