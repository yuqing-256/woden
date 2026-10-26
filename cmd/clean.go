package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yuqing-256/woden/internal/clean"
)

// cleanCmd removes the project's target directory.
var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove the target directory and its build artifacts",
	Long: `Remove the project's target directory.

Only target/ is deleted; source files, woden.toml, woden.lock and vendor/ are
never touched. Symbolic links, junctions and other links found inside target/
are removed as links: their targets are never followed or deleted.

The paths that are deleted are printed one per line, relative to the project
root. With --dry-run the same list is printed and nothing is changed.`,
	Example: `  woden clean
  woden clean --dry-run`,
	GroupID: GroupProject,
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		plan, err := clean.BuildPlan(project.Root)
		if err != nil {
			return err
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		for _, rel := range plan.RelPaths() {
			if _, err := fmt.Fprintln(out, rel); err != nil {
				return err
			}
		}
		if dryRun {
			return nil
		}
		return plan.Remove()
	},
}

func init() {
	cleanCmd.Flags().Bool("dry-run", false, "list the paths that would be deleted without deleting anything")
	rootCmd.AddCommand(cleanCmd)
}
