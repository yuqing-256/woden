package proc_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yuqing-256/woden/internal/proc"
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
	started := filepath.Join(t.TempDir(), "child-started")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := (proc.ExecRunner{}).Run(ctx, os.Args[0], []string{"-test.run=TestProcessHelper", "--", "sleep", started}, proc.Options{})
	if err == nil {
		t.Fatal("a timed out command should return an error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout diagnostic", err)
	}
	if _, statErr := os.Stat(started); statErr != nil {
		t.Fatalf("child did not start before timeout: %v", statErr)
	}
}

func TestProcessHelper(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-2] != "sleep" {
		return
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("started"), 0o600); err != nil {
		t.Fatalf("mark child process start: %v", err)
	}
	time.Sleep(5 * time.Second)
}

func TestExecRunnerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := filepath.Join(t.TempDir(), "child-started")
	cmdDone := make(chan error, 1)
	go func() {
		_, err := (proc.ExecRunner{}).Run(ctx, os.Args[0], []string{"-test.run=TestProcessHelper", "--", "sleep", started}, proc.Options{})
		cmdDone <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("child process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-cmdDone:
		if err == nil || !strings.Contains(err.Error(), "interrupted") {
			t.Fatalf("error = %v, want cancellation diagnostic", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled process did not stop")
	}
}

func TestExecRunnerMissingCommand(t *testing.T) {
	_, err := (proc.ExecRunner{}).Run(context.Background(), "woden-no-such-executable-xyz", nil, proc.Options{})
	if err == nil {
		t.Fatal("missing command should return an error")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		t.Fatalf("missing command returned exit error: %v", err)
	}
}
