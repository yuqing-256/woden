// Package faketool builds fake odin and odinfmt executables for Woden's tests.
//
// The fake tools are copies of one small helper program (cmd/faketool). Every
// invocation appends a JSON record with the tool name, its arguments, its
// working directory and the exit code it returns. Tests can therefore assert the
// exact command line Woden builds, and the directory it runs in, without the
// real Odin toolchain being installed.
package faketool

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const (
	// EnvLog is the environment variable that names the invocation log file.
	EnvLog = "WODEN_FAKE_TOOL_LOG"
	// EnvExit is the environment variable that sets the exit code of the fake
	// tools.
	EnvExit = "WODEN_FAKE_TOOL_EXIT"

	// importPath is the import path of the fake tool program.
	importPath = "woden/internal/testutil/faketool/cmd/faketool"
)

// Record is one logged fake tool invocation.
type Record struct {
	// Tool is the name the fake tool was invoked as ("odin" or "odinfmt").
	Tool string `json:"tool"`
	// Args are the arguments after the program name.
	Args []string `json:"args"`
	// Dir is the working directory the tool ran in.
	Dir string `json:"dir"`
	// Exit is the exit code the tool returned.
	Exit int `json:"exit"`
}

// Install builds the fake tool program and installs two copies of it, named
// odin and odinfmt, in dir. moduleRoot must be the directory containing go.mod.
func Install(t testing.TB, moduleRoot, dir string) {
	t.Helper()
	odinPath := filepath.Join(dir, "odin"+exeSuffix())
	build(t, moduleRoot, odinPath)
	data, err := os.ReadFile(odinPath)
	if err != nil {
		t.Fatalf("faketool: cannot read %s: %v", odinPath, err)
	}
	odinfmtPath := filepath.Join(dir, "odinfmt"+exeSuffix())
	if err := os.WriteFile(odinfmtPath, data, 0o755); err != nil {
		t.Fatalf("faketool: cannot write %s: %v", odinfmtPath, err)
	}
}

// build compiles the fake tool program to out.
func build(t testing.TB, moduleRoot, out string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", out, importPath)
	cmd.Dir = moduleRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("faketool: cannot build the fake tools: %v\n%s", err, output)
	}
}

// Env returns an environment for running Woden with dir as its whole PATH, the
// fake tools logging to logPath and exiting with exitCode.
func Env(dir, logPath string, exitCode int) []string {
	env := WithPath(os.Environ(), dir)
	env = Set(env, EnvLog, logPath)
	env = Set(env, EnvExit, strconv.Itoa(exitCode))
	return env
}

// WithPath returns env with PATH replaced by dirs.
func WithPath(env []string, dirs ...string) []string {
	return Set(env, "PATH", strings.Join(dirs, string(os.PathListSeparator)))
}

// Set returns a copy of env with key set to value. An existing entry for key is
// replaced; as Windows environments are case-insensitive, so is the lookup.
func Set(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(name, key) {
			continue
		}
		out = append(out, entry)
	}
	return append(out, key+"="+value)
}

// Invocations returns the invocations logged at logPath. A missing log file
// means that no fake tool ran.
func Invocations(t testing.TB, logPath string) []Record {
	t.Helper()
	file, err := os.Open(logPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		t.Fatalf("faketool: cannot open %s: %v", logPath, err)
	}
	defer file.Close()
	var records []Record
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("faketool: invalid invocation record %q: %v", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("faketool: cannot read %s: %v", logPath, err)
	}
	return records
}

// exeSuffix returns the platform executable extension.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
