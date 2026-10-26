/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// updateCmd advances the dependencies that track a moving reference.
var updateCmd = &cobra.Command{
	Use:   "update [NAME...]",
	Short: "Update Git dependencies to their remote commits",
	Long: `Advance the Git dependencies that track a branch, a tag or the remote default
branch to the commit the remote resolves to now, and restore missing vendor
worktrees at the new commit.

Without NAME every dependency that is not pinned to a revision is updated.
Dependencies pinned with --rev are never advanced automatically: they are
reported as skipped, and naming one explicitly is an error.

A vendor worktree with local modifications is never overwritten; it is reported
as skipped, the remaining dependencies are still processed, and the command
fails at the end.`,
	Example: `  woden update
  woden update odin-json`,
	Args:    cobra.ArbitraryArgs,
	GroupID: GroupDependency,
	// Completion runs before PersistentPreRunE loads the project.
	ValidArgsFunction: completeDependencyNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		deps, err := newGitDeps(cmd, p)
		if err != nil {
			return err
		}
		return deps.Update(commandContext(cmd), args)
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
