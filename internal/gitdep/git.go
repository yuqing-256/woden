package gitdep

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/lock"
	"woden/internal/proc"
	"woden/internal/validate"
)

// gitProgram is the only program gitdep runs.
const gitProgram = "git"

// gitEnv is the environment of every Git invocation: Git must never prompt and
// its output must be stable and machine readable.
func gitEnv() []string {
	return []string{"LC_ALL=C", "GIT_TERMINAL_PROMPT=0"}
}

// gitArguments prepends the global options required on every invocation.
// Invalid dependencies cannot use the ext:: transport, and options such as
// --upload-pack cannot be injected from configuration.
func gitArguments(args ...string) []string {
	out := make([]string, 0, len(args)+2)
	out = append(out, "-c", "protocol.ext.allow=never")
	return append(out, args...)
}

// gitOutput is the captured output of one Git invocation.
type gitOutput struct {
	stdout string
	stderr string
}

// execGit runs Git in dir and returns the captured output and its exit code. A
// non-zero exit code is not an error: callers that probe for an expected
// failure use it directly.
func (d *Deps) execGit(ctx context.Context, dir string, args ...string) (gitOutput, int, error) {
	var stdout, stderr bytes.Buffer
	res, err := d.Runner.Run(ctx, gitProgram, gitArguments(args...), proc.Options{
		Dir:     dir,
		Env:     gitEnv(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Timeout: d.timeout(),
	})
	if err != nil {
		return gitOutput{}, 0, err
	}
	return gitOutput{stdout: stdout.String(), stderr: stderr.String()}, res.ExitCode, nil
}

// runGit runs Git and converts a non-zero exit code into a user error that
// includes Git's own diagnostics. On success stderr is ignored because Git
// prints informational messages there (for example the detached HEAD notice of
// checkout).
func (d *Deps) runGit(ctx context.Context, dir string, args ...string) (gitOutput, error) {
	out, code, err := d.execGit(ctx, dir, args...)
	if err != nil {
		return gitOutput{}, err
	}
	if code != 0 {
		return gitOutput{}, errs.Userf("git %s failed: %s", gitSubcommand(args), gitErrorText(out, code))
	}
	return out, nil
}

// gitSubcommand returns the subcommand of args, skipping global options.
func gitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "-C" || arg == "--git-dir" || arg == "--work-tree" || arg == "--namespace" || arg == "--exec-path":
			i++
		case strings.HasPrefix(arg, "-"):
			continue
		default:
			return arg
		}
	}
	return "?"
}

// gitErrorText returns the most useful diagnostic of a failed invocation.
func gitErrorText(out gitOutput, code int) string {
	for _, text := range []string{out.stderr, out.stdout} {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		lines := strings.Split(text, "\n")
		if len(lines) > 5 {
			lines = lines[:5]
		}
		return strings.Join(lines, "\n")
	}
	return fmt.Sprintf("exit code %d", code)
}

// refSpec is the requested source reference of a dependency: a branch, a tag,
// a fixed revision, or the remote default branch when kind is empty.
type refSpec struct {
	kind  string
	value string
}

// refFromConfig returns the refSpec declared in woden.toml.
func refFromConfig(dep config.Dependency) refSpec {
	kind, value := dep.Ref()
	return refSpec{kind: kind, value: value}
}

// matches reports whether the lock package records the same requested ref.
func (r refSpec) matches(pkg lock.Package) bool {
	return r.kind == pkg.RefKind() && r.value == pkg.RefValue()
}

// describe returns a human readable description of the requested ref.
func (r refSpec) describe() string {
	switch r.kind {
	case "branch":
		return "branch " + r.value
	case "tag":
		return "tag " + r.value
	case "rev":
		return "revision " + r.value
	default:
		return "the remote default branch"
	}
}

// cloneRefExpression returns the revision expression that resolves the
// requested ref inside a fresh clone.
func (r refSpec) cloneRefExpression() string {
	switch r.kind {
	case "branch":
		return "refs/remotes/origin/" + r.value + "^{commit}"
	case "tag":
		return "refs/tags/" + r.value + "^{commit}"
	case "rev":
		return r.value + "^{commit}"
	default:
		return "HEAD^{commit}"
	}
}

// fetchTarget returns the refspec passed to "git fetch" and the revision
// expression that resolves the fetched commit. Tags are fetched into
// refs/tags so a moved tag is observed; branches are forced into the
// remote-tracking namespace.
func (r refSpec) fetchTarget() (string, string) {
	switch r.kind {
	case "branch":
		return "+refs/heads/" + r.value + ":refs/remotes/origin/" + r.value,
			"refs/remotes/origin/" + r.value + "^{commit}"
	case "tag":
		return "+refs/tags/" + r.value + ":refs/tags/" + r.value,
			"refs/tags/" + r.value + "^{commit}"
	default:
		return "HEAD", "FETCH_HEAD^{commit}"
	}
}

// resolveCommit resolves rev to a full commit ID in the repository at dir. The
// revision is passed as one positional argument without "--": rev-parse treats
// arguments after "--" as paths, which would never resolve to a commit.
func (d *Deps) resolveCommit(ctx context.Context, dir, rev string) (string, error) {
	out, err := d.runGit(ctx, dir, "rev-parse", "--verify", rev)
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(out.stdout)
	if err := validate.CommitID(commit); err != nil {
		return "", errs.Userf("git resolved %q to %q, which is not a full commit ID", rev, commit)
	}
	return commit, nil
}

// clone clones the repository at url into dir without checking anything out.
// The URL and the destination are separated from the options with "--" so a
// value can never be mistaken for a Git option.
func (d *Deps) clone(ctx context.Context, url, dir string) error {
	_, err := d.runGit(ctx, d.Project.Root, "clone", "--quiet", "--no-checkout", "--", url, dir)
	return err
}

// checkout detaches the worktree at dir at commit.
func (d *Deps) checkout(ctx context.Context, dir, commit string) error {
	// "--detach" makes checkout unambiguous: commit is a commit, never a path.
	_, err := d.runGit(ctx, dir, "checkout", "--quiet", "--detach", commit)
	return err
}

// isClean reports whether the worktree at dir has no local modifications and no
// untracked files.
func (d *Deps) isClean(ctx context.Context, dir string) (bool, error) {
	out, err := d.runGit(ctx, dir, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out.stdout) == "", nil
}

// head returns the full commit ID checked out at dir.
func (d *Deps) head(ctx context.Context, dir string) (string, error) {
	return d.resolveCommit(ctx, dir, "HEAD^{commit}")
}

// isWorktree reports whether dir is a Git worktree.
func (d *Deps) isWorktree(ctx context.Context, dir string) (bool, error) {
	_, code, err := d.execGit(ctx, dir, "rev-parse", "--git-dir")
	if err != nil {
		return false, err
	}
	return code == 0, nil
}

// originURL returns the origin URL of the worktree at dir. ok is false when no
// origin remote is configured.
func (d *Deps) originURL(ctx context.Context, dir string) (url string, ok bool, err error) {
	out, code, err := d.execGit(ctx, dir, "remote", "get-url", "--", "origin")
	if err != nil {
		return "", false, err
	}
	if code != 0 {
		return "", false, nil
	}
	return strings.TrimSpace(out.stdout), true, nil
}

// fetch fetches the requested ref of url into the repository at dir and returns
// the resolved full commit ID.
func (d *Deps) fetch(ctx context.Context, dir, url string, spec refSpec) (string, error) {
	refspec, rev := spec.fetchTarget()
	if _, err := d.runGit(ctx, dir, "fetch", "--quiet", "--no-tags", "--", url, refspec); err != nil {
		return "", err
	}
	return d.resolveCommit(ctx, dir, rev)
}
