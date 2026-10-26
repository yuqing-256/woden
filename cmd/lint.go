package cmd

import (
	"github.com/spf13/cobra"

	"github.com/yuqing-256/woden/internal/odin"
	"github.com/yuqing-256/woden/internal/project"
)

// lintCmd checks src/ with "odin check -vet".
var lintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Check src/ with odin check -vet",
	Long: `Check the Odin sources in src/ with odin check -vet.

The -vet flag enables Odin's built-in vet checks. The collections from
woden.toml are passed to Odin as -collection arguments.`,
	Example: "  woden lint",
	Args:    cobra.NoArgs,
	GroupID: GroupBuild,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		if err := p.CheckVendor(); err != nil {
			return err
		}
		code, err := runExternalTool(cmd, "odin", p.Root, lintInvocation(p))
		if err != nil {
			return err
		}
		if code != 0 {
			return childExitError("odin", code)
		}
		return nil
	},
}

// lintInvocation returns the "odin check -vet" arguments for p.
func lintInvocation(p *project.Project) []string {
	return odin.LintArgs(p.CollectionArgs())
}

func init() {
	rootCmd.AddCommand(lintCmd)
}
