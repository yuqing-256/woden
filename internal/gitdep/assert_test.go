package gitdep

import (
	"testing"
)

// mustHaveArgs asserts the exact Git command line of a recorded call.
func mustHaveArgs(t *testing.T, call fakeCall, want ...string) {
	t.Helper()
	if call.name != gitProgram {
		t.Fatalf("gitdep: expected program %q, got %q", gitProgram, call.name)
	}
	if len(call.args) != len(want) {
		t.Fatalf("gitdep: expected arguments %v, got %v", want, call.args)
	}
	for i := range want {
		if call.args[i] != want[i] {
			t.Fatalf("gitdep: expected arguments %v, got %v", want, call.args)
		}
	}
}

// mustHaveGitEnv asserts the non-interactive environment of a recorded call.
func mustHaveGitEnv(t *testing.T, call fakeCall) {
	t.Helper()
	want := map[string]bool{"LC_ALL=C": true, "GIT_TERMINAL_PROMPT=0": true}
	for _, entry := range call.opts.Env {
		delete(want, entry)
	}
	for missing := range want {
		t.Errorf("gitdep: Git invocation %v is missing the environment entry %q (env: %v)", call.args, missing, call.opts.Env)
	}
	if call.opts.Timeout != DefaultTimeout {
		t.Errorf("gitdep: expected timeout %v, got %v", DefaultTimeout, call.opts.Timeout)
	}
}

// mustHavePrefix asserts that args start with the required global options.
func mustHavePrefix(t *testing.T, call fakeCall) {
	t.Helper()
	if len(call.args) < 2 || call.args[0] != "-c" || call.args[1] != "protocol.ext.allow=never" {
		t.Fatalf("gitdep: Git invocation does not start with -c protocol.ext.allow=never: %v", call.args)
	}
}

// cloneDestination returns the destination directory of a recorded clone.
func cloneDestination(t *testing.T, call fakeCall) string {
	t.Helper()
	_, dest, ok := positionalAfterDoubleDash(call.args, 2)
	if !ok {
		t.Fatalf("gitdep: malformed clone call: %v", call.args)
	}
	return dest
}
