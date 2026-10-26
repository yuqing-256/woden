package gitdep

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/proc"
)

// fakeCall records one Git invocation.
type fakeCall struct {
	name string
	args []string
	opts proc.Options
}

// fakeGit is a proc.Runner that simulates the small Git surface gitdep uses.
// It records every invocation so tests can pin the exact command line, the
// environment and the working directory, and it scripts the answers of the
// probe commands. Remote commands (clone, fetch) succeed by creating the
// destination directory; a test that must prove that no remote access happens
// simply asserts that no clone or fetch call was recorded.
type fakeGit struct {
	t     *testing.T
	calls []fakeCall

	// heads maps a working directory to the commit checked out there.
	heads map[string]string
	// dirty marks working directories with local modifications.
	dirty map[string]bool
	// origins maps a working directory to its origin URL.
	origins map[string]string
	// repos marks working directories that are Git worktrees.
	repos map[string]bool
	// resolve maps a revision expression (for example
	// "refs/heads/main^{commit}") to the commit it resolves to.
	resolve map[string]string
	// fail scripts a failing subcommand with its exit code and message.
	fail map[string]fakeFailure
}

// fakeFailure is a scripted Git failure.
type fakeFailure struct {
	code    int
	message string
}

// newFakeGit returns a healthy fake with no scripted state.
func newFakeGit(t *testing.T) *fakeGit {
	t.Helper()
	return &fakeGit{
		t:       t,
		heads:   map[string]string{},
		dirty:   map[string]bool{},
		origins: map[string]string{},
		repos:   map[string]bool{},
		resolve: map[string]string{},
		fail:    map[string]fakeFailure{},
	}
}

// setHead registers the commit checked out in dir.
func (f *fakeGit) setHead(dir, commit string) {
	f.heads[dir] = commit
	f.repos[dir] = true
}

// setOrigin registers the origin URL of the worktree at dir.
func (f *fakeGit) setOrigin(dir, url string) {
	f.origins[dir] = url
	f.repos[dir] = true
}

// setDirty marks the worktree at dir as modified.
func (f *fakeGit) setDirty(dir string) {
	f.dirty[dir] = true
	f.repos[dir] = true
}

// Run implements proc.Runner.
func (f *fakeGit) Run(ctx context.Context, name string, args []string, opts proc.Options) (proc.Result, error) {
	if err := ctx.Err(); err != nil {
		return proc.Result{}, err
	}
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...), opts: opts})
	sub := gitSubcommand(args)
	if failure, ok := f.fail[sub]; ok {
		f.stderr(opts, failure.message)
		return proc.Result{ExitCode: failure.code}, nil
	}
	switch sub {
	case "clone":
		return f.clone(args, opts)
	case "checkout":
		return f.checkout(args, opts)
	case "rev-parse":
		return f.revParse(args, opts)
	case "status":
		return f.status(opts)
	case "remote":
		return f.remote(opts)
	case "fetch":
		return f.fetch(opts)
	default:
		f.t.Fatalf("gitdep: unexpected Git subcommand %q in %v", sub, args)
		return proc.Result{}, nil
	}
}

// clone creates the destination directory like a successful clone would.
func (f *fakeGit) clone(args []string, opts proc.Options) (proc.Result, error) {
	url, dest, ok := positionalAfterDoubleDash(args, 2)
	if !ok {
		f.t.Fatalf("gitdep: malformed clone: %v", args)
	}
	_ = url
	if err := os.MkdirAll(dest, 0o755); err != nil {
		f.t.Fatalf("gitdep: fake clone cannot create %s: %v", dest, err)
	}
	f.repos[dest] = true
	return proc.Result{}, nil
}

// checkout records the detached HEAD of the working directory.
func (f *fakeGit) checkout(args []string, opts proc.Options) (proc.Result, error) {
	commit := args[len(args)-1]
	if !strings.HasPrefix(commit, "--") {
		f.heads[opts.Dir] = commit
	}
	return proc.Result{}, nil
}

// revParse answers the two forms gitdep uses: --git-dir probes and
// --verify <rev> resolutions.
func (f *fakeGit) revParse(args []string, opts proc.Options) (proc.Result, error) {
	for i, arg := range args {
		switch arg {
		case "--git-dir":
			if !f.repos[opts.Dir] {
				f.stderr(opts, "fatal: not a git repository (or any of the parent directories)")
				return proc.Result{ExitCode: 128}, nil
			}
			f.stdout(opts, ".git\n")
			return proc.Result{}, nil
		case "--verify":
			if i+1 >= len(args) {
				f.t.Fatalf("gitdep: malformed rev-parse: %v", args)
			}
			rev := args[i+1]
			commit, ok := f.commitFor(opts.Dir, rev)
			if !ok {
				f.stderr(opts, "fatal: Needed a single revision")
				return proc.Result{ExitCode: 128}, nil
			}
			f.stdout(opts, commit+"\n")
			return proc.Result{}, nil
		}
	}
	f.t.Fatalf("gitdep: unexpected rev-parse form: %v", args)
	return proc.Result{}, nil
}

// commitFor resolves a revision expression in dir.
func (f *fakeGit) commitFor(dir, rev string) (string, bool) {
	if commit, ok := f.resolve[rev]; ok {
		return commit, true
	}
	if rev == "HEAD^{commit}" {
		commit, ok := f.heads[dir]
		return commit, ok && commit != ""
	}
	return "", false
}

// status answers the cleanliness probe.
func (f *fakeGit) status(opts proc.Options) (proc.Result, error) {
	if f.dirty[opts.Dir] {
		f.stdout(opts, " M deplib.odin\n")
	}
	return proc.Result{}, nil
}

// remote answers "remote get-url -- origin".
func (f *fakeGit) remote(opts proc.Options) (proc.Result, error) {
	url, ok := f.origins[opts.Dir]
	if !ok {
		f.stderr(opts, "error: No such remote 'origin'")
		return proc.Result{ExitCode: 2}, nil
	}
	f.stdout(opts, url+"\n")
	return proc.Result{}, nil
}

// fetch synthesises a successful remote fetch. The subsequent rev-parse is
// answered from resolve.
func (f *fakeGit) fetch(opts proc.Options) (proc.Result, error) {
	return proc.Result{}, nil
}

// positionalAfterDoubleDash returns count positional arguments following "--".
func positionalAfterDoubleDash(args []string, count int) (string, string, bool) {
	for i, arg := range args {
		if arg != "--" {
			continue
		}
		if i+count >= len(args) {
			return "", "", false
		}
		if count == 2 {
			return args[i+1], args[i+2], true
		}
		return args[i+1], "", true
	}
	return "", "", false
}

// stdout writes to the captured stdout of the invocation.
func (f *fakeGit) stdout(opts proc.Options, text string) {
	if opts.Stdout != nil {
		fmt.Fprint(opts.Stdout, text)
	}
}

// stderr writes to the captured stderr of the invocation.
func (f *fakeGit) stderr(opts proc.Options, text string) {
	if opts.Stderr != nil {
		fmt.Fprint(opts.Stderr, text)
	}
}

// subcommands returns the subcommand of every recorded call, in order.
func (f *fakeGit) subcommands() []string {
	out := make([]string, len(f.calls))
	for i, call := range f.calls {
		out[i] = gitSubcommand(call.args)
	}
	return out
}

// callsFor returns the recorded calls of the given subcommand.
func (f *fakeGit) callsFor(sub string) []fakeCall {
	var out []fakeCall
	for _, call := range f.calls {
		if gitSubcommand(call.args) == sub {
			out = append(out, call)
		}
	}
	return out
}

// assertNoRemoteCommands fails when gitdep tried to clone or fetch: the
// read-only and offline code paths must never touch the network.
func (f *fakeGit) assertNoRemoteCommands(t *testing.T) {
	t.Helper()
	for _, call := range f.calls {
		switch gitSubcommand(call.args) {
		case "clone", "fetch":
			t.Errorf("gitdep: unexpected remote command: git %v", call.args)
		}
	}
}
