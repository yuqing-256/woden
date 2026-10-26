package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"woden/internal/errs"
	"woden/internal/gitdep"
	"woden/internal/proc"
	"woden/internal/project"
)

// syncCmd restores the vendor worktrees from woden.lock.
var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Restore Git dependencies from woden.lock",
	Long: `Restore the vendor worktrees of the Git dependencies declared in woden.toml.

woden.toml is the source of truth for the set of dependencies and woden.lock
pins their commits. sync clones missing or incorrect vendor worktrees from the
lock and never advances the pinned commits; use "woden update" for that. A
worktree with local modifications is reported and left untouched, the remaining
dependencies are still synchronized, and the command fails at the end.

With --check nothing is downloaded, checked out or written: sync verifies that
woden.toml, woden.lock, the vendor worktrees and ols.json agree, prints every
problem and fails if there is one.`,
	Example: `  woden sync
  woden sync --check`,
	Args:    cobra.NoArgs,
	GroupID: GroupDependency,
	RunE: func(cmd *cobra.Command, args []string) error {
		check, err := cmd.Flags().GetBool("check")
		if err != nil {
			return errs.WrapUser(err, "invalid arguments")
		}
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		deps, err := newGitDeps(cmd, p)
		if err != nil {
			return err
		}
		return deps.Sync(commandContext(cmd), check)
	},
}

// newGitDeps returns the Git dependency runner for cmd. Git must be installed;
// a missing Git is a user error.
func newGitDeps(cmd *cobra.Command, p *project.Project) (*gitdep.Deps, error) {
	if _, err := proc.Look("git"); err != nil {
		return nil, err
	}
	return gitdep.New(p, proc.ExecRunner{}, cmd.OutOrStdout(), cmd.ErrOrStderr()), nil
}

// commandContext returns the context of cmd, falling back to a background
// context when RunE is called directly (for example by a test).
func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

// completeDependencyNames completes the dependency names declared in the
// project. Cobra runs completion before PersistentPreRunE, so the project is
// loaded here.
func completeDependencyNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	p, err := project.Load(cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	used := make(map[string]bool, len(args))
	for _, arg := range args {
		used[arg] = true
	}
	var names []string
	for _, name := range p.Config.SortedDependencyNames() {
		if used[name] {
			continue
		}
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().Bool("check", false, "Verify the dependencies without changing anything")
}
