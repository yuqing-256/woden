package cmd

import (
	"github.com/spf13/cobra"

	"github.com/yuqing-256/woden/internal/scaffold"
)

// initCmd initializes a project in an existing directory.
var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a project in an existing directory",
	Long: `Initialize the directory at [path] as a Woden project.

When [path] is omitted, the current directory is used. The directory must
already exist.

Existing files are never overwritten: only missing files are created, and an
existing .gitignore is completed with the entries it does not have yet.`,
	Example: `  woden init
  woden init path/to/my-game`,
	GroupID: GroupProject,
	Annotations: map[string]string{
		NoProjectAnnotation: "true",
	},
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeDirectoryArgument,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return scaffold.Init("")
		}
		return scaffold.Init(args[0])
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
