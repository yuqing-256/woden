package cmd

import (
	"github.com/spf13/cobra"

	"github.com/yuqing-256/woden/internal/scaffold"
)

// newCmd creates a new project.
var newCmd = &cobra.Command{
	Use:   "new <path>",
	Short: "Create a new Woden project",
	Long: `Create a new Woden project in the directory at <path>.

The directory must not exist yet or must be empty. Missing parent directories
are created. The project name is taken from the directory name, so the directory
must be a valid Woden project name.

This command does not run git init.`,
	Example: `  woden new my-game`,
	GroupID: GroupProject,
	Annotations: map[string]string{
		NoProjectAnnotation: "true",
	},
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeDirectoryArgument,
	RunE: func(cmd *cobra.Command, args []string) error {
		return scaffold.New(args[0])
	},
}

// completeDirectoryArgument completes a single directory argument. It is shared
// by the commands that take a project directory.
func completeDirectoryArgument(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func init() {
	rootCmd.AddCommand(newCmd)
}
