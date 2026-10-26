// Package gitdep implements Woden's Git dependency management: the add, remove,
// sync and update operations, the woden.lock synchronization with woden.toml,
// and the vendor worktree management.
//
// All Git access goes through proc.Runner: internal/gitdep never invokes
// anything itself, so the operations are testable with a fake Runner and the
// command layer decides on the concrete environment (timeouts, PATH, stdout).
//
// Every Git invocation is non-interactive: git is started with
// "-c protocol.ext.allow=never", LC_ALL=C and GIT_TERMINAL_PROMPT=0, without a
// shell, with arguments passed as a slice, and with a timeout. Remote URLs and
// refs are validated with internal/validate before use.
//
// Vendor worktrees are full Git clones that are placed atomically: a clone is
// first created in a staging directory below vendor/ and then renamed to its
// final name. A crash never leaves a half-cloned worktree in place, and every
// interrupted operation can be repaired by running "woden sync", which treats
// woden.toml as the source of truth.
package gitdep

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/ols"
	"github.com/yuqing-256/woden/internal/proc"
	"github.com/yuqing-256/woden/internal/project"
)

const (
	// DefaultTimeout bounds a single Git invocation.
	DefaultTimeout = 2 * time.Minute

	// stagingPrefix is the name prefix of the temporary clone directories below
	// vendor/.
	stagingPrefix = ".staging-"
)

// Deps runs the Git dependency operations for one project.
type Deps struct {
	// Project is the loaded project. The operations that edit woden.toml
	// refresh Project.Config before they use it again.
	Project *project.Project
	// Runner runs Git. The command layer passes proc.ExecRunner.
	Runner proc.Runner
	// Stdout and Stderr receive progress output. Nil means os.Stdout/os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
	// Timeout bounds each Git invocation. Zero means DefaultTimeout.
	Timeout time.Duration
}

// New returns a Deps for the project.
func New(p *project.Project, runner proc.Runner, stdout, stderr io.Writer) *Deps {
	return &Deps{Project: p, Runner: runner, Stdout: stdout, Stderr: stderr}
}

// out returns the progress output writer.
func (d *Deps) out() io.Writer {
	if d.Stdout != nil {
		return d.Stdout
	}
	return os.Stdout
}

// errOut returns the diagnostic output writer.
func (d *Deps) errOut() io.Writer {
	if d.Stderr != nil {
		return d.Stderr
	}
	return os.Stderr
}

// timeout returns the timeout for one Git invocation.
func (d *Deps) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return DefaultTimeout
}

// reload re-reads woden.toml into the project after an edit.
func (d *Deps) reload() error {
	cfg, warnings, err := config.Load(d.Project.ConfigPath)
	if err != nil {
		return err
	}
	d.Project.Config = cfg
	d.Project.Warnings = warnings
	return nil
}

// updateOLS reconciles ols.json with the collections configured in woden.toml.
// The deps mapping is managed entirely by gitdep: it is added with the first
// dependency and removed with the last one, and ols.Sync only ever removes a
// declared mapping once woden.toml no longer contains it.
func (d *Deps) updateOLS() (bool, error) {
	if d.Project.Config == nil {
		return false, errs.Userf("project configuration is not loaded")
	}
	changed, _, err := ols.Sync(d.Project.Root, d.Project.Config.Collections)
	return changed, err
}

// problem collects non-fatal problems. Operations report every problem and
// fail at the end, so one dirty worktree does not hide the other ones.
type problem struct {
	w    io.Writer
	n    int
	what string
}

// addf records a problem on its own line.
func (p *problem) addf(format string, args ...any) {
	p.n++
	fmt.Fprintf(p.w, format+"\n", args...)
}

// err returns the failure for the recorded problems, or nil.
func (p *problem) err() error {
	if p.n == 0 {
		return nil
	}
	return errs.Userf("%s: %d problem(s) found", p.what, p.n)
}
