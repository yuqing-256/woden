package cmd

import (
	"github.com/spf13/cobra"

	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/odin"
	"github.com/yuqing-256/woden/internal/project"
)

// fmtCmd formats src/ and tests/ with odinfmt.
var fmtCmd = &cobra.Command{
	Use:   "fmt",
	Short: "Format src/ and tests/ with odinfmt",
	Long: `Format the Odin sources in src/ and tests/ with odinfmt.

The src/ directory is formatted before tests/. The libs/ and vendor/ directories
are never formatted.`,
	Example: "  woden fmt",
	Args:    cobra.NoArgs,
	GroupID: GroupBuild,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		dirs, err := formatDirectories(p)
		if err != nil {
			return err
		}
		for _, dir := range dirs {
			code, err := runExternalTool(cmd, "odinfmt", p.Root, odin.FmtArgs(dir))
			if err != nil {
				return err
			}
			if code != 0 {
				return childExitError("odinfmt", code)
			}
		}
		return nil
	},
}

// formatDirectories returns the directories "woden fmt" formats, in order:
// src/ and then tests/, skipping the ones that do not exist. Nothing to format
// is a user error.
func formatDirectories(p *project.Project) ([]string, error) {
	dirs := p.SourceAndTestDirs()
	if len(dirs) == 0 {
		return nil, errs.Userf("neither %s/ nor %s/ exists in %s; nothing to format", project.SourceDir, project.TestsDir, p.Root)
	}
	return dirs, nil
}

func init() {
	rootCmd.AddCommand(fmtCmd)
}
