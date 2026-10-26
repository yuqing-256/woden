//go:build woden_testing

package gitdep

import (
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yuqing-256/woden/internal/proc"
	"github.com/yuqing-256/woden/internal/project"
	"github.com/yuqing-256/woden/internal/testutil"
)

// The integration tests use real Git against a repository created with
// testutil.NewDepRepo. The repository is reached through a file:// URL when the
// local transport works; some sandboxes forbid the shell that Git spawns for
// file:// URLs, so the helper falls back to a loopback-only git daemon. No
// external network access ever happens.
const integrationTimeout = 60 * time.Second

// fileTransportProbe and fileTransportNote remember the (slow) transport probe
// so only the first integration test pays for it.
var (
	fileTransportProbe sync.Once
	fileTransportNote  sync.Once
	fileTransportOK    bool
)

// integrationRepo is a real Git repository plus a URL that can reach it.
type integrationRepo struct {
	*testutil.DepRepo
	URL string
}

// newIntegrationRepo creates a real Git dependency repository and picks a URL
// that actually works in the current environment.
func newIntegrationRepo(t *testing.T) *integrationRepo {
	t.Helper()
	disableGitPrompts(t)

	repo := testutil.NewDepRepo(t)
	url := repo.URL
	if !fileTransportWorks(repo.URL) {
		fileTransportNote.Do(func() {
			t.Logf("file:// transport is unavailable in this environment: serving repositories over loopback git daemons instead")
		})
		url = startLoopbackDaemon(t, repo.Dir)
	}
	return &integrationRepo{DepRepo: repo, URL: url}
}

// fileTransportWorks probes the file:// transport once: whether Git can use it
// is a property of the installed Git build, not of an individual URL, and the
// probe is slow when the transport is broken.
func fileTransportWorks(url string) bool {
	fileTransportProbe.Do(func() { fileTransportOK = urlReachable(url) })
	return fileTransportOK
}

// disableGitPrompts makes every Git invocation deterministic and offline.
func disableGitPrompts(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_CONFIG_GLOBAL":   os.DevNull,
		"GIT_CONFIG_SYSTEM":   os.DevNull,
		"LC_ALL":              "C",
	} {
		t.Setenv(name, value)
	}
}

// urlReachable reports whether the current Git build can talk to url.
func urlReachable(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	runner := proc.ExecRunner{}
	result, err := runner.Run(ctx, "git", []string{"ls-remote", "--", url, "HEAD"}, proc.Options{
		Stdout:  io.Discard,
		Stderr:  io.Discard,
		Timeout: 20 * time.Second,
	})
	return err == nil && result.ExitCode == 0
}

// startLoopbackDaemon serves dir over git://127.0.0.1:<port>/<name> and returns
// the URL. The daemon is stopped when the test finishes.
func startLoopbackDaemon(t *testing.T, dir string) string {
	t.Helper()

	port := freeLocalPort(t)
	name := filepath.Base(dir)
	args := []string{
		"daemon",
		"--reuseaddr",
		"--export-all",
		"--base-path=" + filepath.Dir(dir),
		"--port=" + strconv.Itoa(port),
		"--listen=127.0.0.1",
	}
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
	// Run from os.TempDir, never from a test directory: Windows cannot delete a
	// directory that is still some process's working directory.
	cmd.Dir = os.TempDir()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("gitdep integration: cannot start the loopback git daemon: %v", err)
	}
	t.Cleanup(func() { stopDaemon(t, cmd.Process.Pid, port) })

	url := "git://127.0.0.1:" + strconv.Itoa(port) + "/" + name
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if urlReachable(url) {
			return url
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("gitdep integration: the loopback git daemon did not become reachable at %s: %s", url, strings.TrimSpace(stderr.String()))
	return ""
}

// freeLocalPort asks the kernel for an unused loopback port.
func freeLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("gitdep integration: cannot allocate a loopback port: %v", err)
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("gitdep integration: unexpected listener address %v", listener.Addr())
	}
	return addr.Port
}

// stopDaemon terminates the daemon and every child it spawned. Git for Windows
// re-executes itself as git-daemon.exe, so the process that holds the listening
// socket can differ from the one that was started; that owner is found by port
// and terminated as well. The function returns once nothing accepts connections
// on the port any more, so callers can safely delete the served repository.
func stopDaemon(t *testing.T, startedPID, port int) {
	t.Helper()
	terminatePID(startedPID)
	if runtime.GOOS != "windows" {
		return
	}
	for attempt := 0; attempt < 50; attempt++ {
		pid := listenerPID(port)
		if pid == 0 {
			return
		}
		terminatePID(pid)
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("warning: the loopback git daemon on port %d did not stop", port)
}

// listenerPID returns the process ID listening on the loopback port, or 0.
func listenerPID(port int) int {
	output, err := exec.Command("netstat", "-ano", "-p", "tcp").Output()
	if err != nil {
		return 0
	}
	suffix := ":" + strconv.Itoa(port)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.EqualFold(fields[0], "TCP") {
			continue
		}
		if !strings.HasSuffix(fields[1], suffix) || !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err == nil {
			return pid
		}
	}
	return 0
}

// integrationProject copies the example project, disables Git prompts and
// returns the loaded project plus a Deps wired to the real Git executable.
func integrationProject(t *testing.T) (*project.Project, *Deps, *strings.Builder, *strings.Builder) {
	t.Helper()
	disableGitPrompts(t)

	root := testutil.CopyExample(t)
	proj, err := project.Load(root)
	if err != nil {
		t.Fatalf("gitdep integration: cannot load the example project: %v", err)
	}
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	deps := New(proj, proc.ExecRunner{}, stdout, stderr)
	deps.Timeout = integrationTimeout
	return proj, deps, stdout, stderr
}

// integrationCommit returns the full commit ID currently checked out in the
// vendor worktree of name.
func integrationCommit(t *testing.T, proj *project.Project, name string) string {
	t.Helper()
	dir := filepath.Join(proj.VendorDir(), name)
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("gitdep integration: cannot read the checked out commit of %s: %v", dir, err)
	}
	return strings.TrimSpace(string(out))
}
