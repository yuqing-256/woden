// Package errs defines the error types shared by Woden.
//
// Woden distinguishes three kinds of failures:
//
//   - user errors, caused by a project, an argument or an environment that the
//     user can fix (reported with the "error: " prefix, exit code 101),
//   - internal errors, i.e. programming mistakes that are never expected,
//   - exit codes that must be forwarded verbatim to the caller (only used by
//     "woden run", which returns the program's exit code).
package errs

import (
	"errors"
	"fmt"
)

// userError marks an error as caused by the user.
type userError struct {
	err error
}

func (e *userError) Error() string { return e.err.Error() }
func (e *userError) Unwrap() error { return e.err }

// Userf returns a user error with a formatted message.
func Userf(format string, args ...any) error {
	return &userError{err: fmt.Errorf(format, args...)}
}

// WrapUser wraps err as a user error with additional context.
func WrapUser(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	return &userError{err: fmt.Errorf(format+": %w", append(args, err)...)}
}

// IsUser reports whether err describes a user error.
func IsUser(err error) bool {
	var u *userError
	return errors.As(err, &u)
}

// Message returns the message of err without any internal detail. It is the
// message printed by main; internal errors are shown verbatim as well because
// Woden does not hide its own bugs from its users.
func Message(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// exitError reports a process exit code that must be used instead of the
// default failure code. Only "woden run" returns one.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	// An empty message means "exit silently": main must not print anything.
	return ""
}

func (e *exitError) Unwrap() error { return e.err }

// Exit returns an error that asks main to terminate with code. If err is nil
// no message is printed.
func Exit(code int, err error) error {
	return &exitError{code: code, err: err}
}

// Code returns the exit code requested by err and whether err requests one.
func Code(err error) (int, bool) {
	var e *exitError
	if errors.As(err, &e) {
		return e.code, true
	}
	return 0, false
}
