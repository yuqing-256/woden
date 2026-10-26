package proc_test

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"woden/internal/proc"
)

func TestLook(t *testing.T) {
	goTool := "go"
	if runtime.GOOS == "windows" {
		goTool = "go.exe"
	}
	path, err := proc.Look(goTool)
	if err != nil {
		t.Fatalf("Look(%q): %v", goTool, err)
	}
	if path == "" {
		t.Fatalf("Look(%q) returned an empty path", goTool)
	}
	if _, err := proc.Look("woden-no-such-tool-xyz"); err == nil {
		t.Fatal("Look of a missing tool should fail")
	} else if !strings.Contains(err.Error(), "not found in PATH") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecRunnerSuccessAndNonZeroExit(t *testing.T) {
	runner := proc.ExecRunner{}
	var stdout, stderr bytes.Buffer

	res, err := runner.Run(context.Background(), "go", []string{"version"}, proc.Options{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("go version: %v (stderr: %s)", err, stderr.String())
	}
	if res.ExitCode != 0 {
		t.Fatalf("go version exit code = %d, want 0", res.ExitCode)
	}
	if !strings.Contains(stdout.String(), "go version") {
		t.Fatalf("stdout = %q, want it to contain %q", stdout.String(), "go version")
	}

	stderr.Reset()
	res, err = runner.Run(context.Background(), "go", []string{"woden-definitely-not-a-command"}, proc.Options{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("non-zero exit should be reported through Result, got error: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("exit code = 0, want non-zero")
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := (proc.ExecRunner{}).Run(ctx, "go", []string{"version"}, proc.Options{})
	if err == nil {
		t.Fatal("a timed out command should return an error")
	}
}
