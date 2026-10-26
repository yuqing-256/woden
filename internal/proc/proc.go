// Package proc wraps external process execution.
//
// Every external program (odin, odinfmt, git) is started through a Runner, so
// commands can be tested with a fake runner instead of relying on the tools
// being installed. Real execution always uses exec.CommandContext (never a
// shell) and supports a timeout.
package proc

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"woden/internal/errs"
)

// Options configures one external command.
type Options struct {
	// Dir is the working directory; empty means the current directory.
	Dir string
	// Env lists additional environment variables ("KEY=value").
	Env []string
	// Stdout and Stderr default to os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
	Stdin  io.Reader
	// Timeout bounds the execution; zero means no timeout.
	Timeout time.Duration
}

// Result is the outcome of an external command. A non-zero ExitCode is not an
// error: it is the caller's decision how to report it.
type Result struct {
	ExitCode int
}

// Runner runs external commands.
type Runner interface {
	Run(ctx context.Context, name string, args []string, opts Options) (Result, error)
}

// ExecRunner runs real commands.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, name string, args []string, opts Options) (Result, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = opts.Dir
	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}
	cmd.Stdin = opts.Stdin
	if opts.Stdout != nil {
		cmd.Stdout = opts.Stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if opts.Stderr != nil {
		cmd.Stderr = opts.Stderr
	} else {
		cmd.Stderr = os.Stderr
	}
	err := cmd.Run()
	if err == nil {
		return Result{ExitCode: 0}, nil
	}
	// A cancelled context kills the child; report that instead of the
	// artificial exit code the operating system produces.
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return Result{}, errs.WrapUser(err, "%s timed out after %s", name, opts.Timeout)
		}
		return Result{}, errs.WrapUser(err, "%s was interrupted", name)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{ExitCode: exitErr.ExitCode()}, nil
	}
	return Result{}, errs.WrapUser(err, "cannot run %s", name)
}

// Look finds an executable in PATH, returning a user error when it is missing
// or only found in the current directory.
func Look(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err == nil {
		return path, nil
	}
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return "", errs.Userf("%s was not found in PATH", name)
	case errors.Is(err, exec.ErrDot):
		return "", errs.Userf("%s was found in the current directory, which is not allowed; add it to PATH", name)
	default:
		return "", errs.WrapUser(err, "cannot find %s in PATH", name)
	}
}

// Check reports whether name can be executed.
func Check(name string) error {
	_, err := Look(name)
	return err
}
