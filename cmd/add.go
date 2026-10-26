package cmd

import (
	"github.com/spf13/cobra"

	"woden/internal/errs"
	"woden/internal/gitdep"
)

// addCmd declares a new Git dependency and vendors it.
var addCmd = &cobra.Command{
	Use:   "add NAME --git URL [--branch BRANCH | --tag TAG | --rev COMMIT]",
	Short: "Add a Git dependency",
	Long: `Add a Git dependency to woden.toml, clone it into vendor/ and pin its commit
in woden.lock.

NAME is the collection name the dependency is available under and must not
already be declared. --git is required. With --branch, --tag or --rev exactly
one source reference is selected; at most one of them may be given. --rev
accepts a full commit ID or an abbreviation of at least seven hexadecimal
digits, and records the value as written while woden.lock always pins the full
commit. Without a reference the dependency tracks the remote default branch.

The dependency is collected in the deps collection, which maps to vendor/. The
deps mapping is added to woden.toml with the first dependency.`,
	Example: `  woden add odin-json --git https://github.com/user/odin-json
  woden add odin-json --git https://github.com/user/odin-json --branch main
  woden add odin-json --git https://github.com/user/odin-json --tag v1.2.0
  woden add odin-json --git https://github.com/user/odin-json --rev 0123456789ab`,
	Args:    cobra.ExactArgs(1),
	GroupID: GroupDependency,
	RunE: func(cmd *cobra.Command, args []string) error {
		flags := cmd.Flags()
		gitURL, err := flags.GetString("git")
		if err != nil {
			return errs.WrapUser(err, "invalid arguments")
		}
		branch, err := flags.GetString("branch")
		if err != nil {
			return errs.WrapUser(err, "invalid arguments")
		}
		tag, err := flags.GetString("tag")
		if err != nil {
			return errs.WrapUser(err, "invalid arguments")
		}
		rev, err := flags.GetString("rev")
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
		return deps.Add(commandContext(cmd), gitdep.AddOptions{
			Name:   args[0],
			Git:    gitURL,
			Branch: branch,
			Tag:    tag,
			Rev:    rev,
		})
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().String("git", "", "Git URL of the dependency (required)")
	addCmd.Flags().String("branch", "", "Track a branch")
	addCmd.Flags().String("tag", "", "Track a tag")
	addCmd.Flags().String("rev", "", "Pin a commit ID")
	_ = addCmd.MarkFlagRequired("git")
	addCmd.MarkFlagsMutuallyExclusive("branch", "tag", "rev")
}
