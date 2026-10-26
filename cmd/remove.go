/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"
)

// removeCmd removes a Git dependency.
var removeCmd = &cobra.Command{
	Use:   "remove NAME",
	Short: "Remove a Git dependency",
	Long: `Remove the Git dependency NAME from woden.toml, woden.lock and vendor/.

The vendor worktree is only removed when it is safe: it must be a Git worktree
whose origin URL is the declared git URL, it must have no local modifications
and it must be checked out at the commit woden.lock pins. Otherwise the command
refuses and leaves everything untouched, so local work is never destroyed.

Removing the last dependency also removes the deps collection mapping from
woden.toml and the then empty vendor/ directory, and drops the deps entry from
ols.json.`,
	Example: `  woden remove odin-json`,
	Args:    cobra.ExactArgs(1),
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
		return deps.Remove(commandContext(cmd), args[0])
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
