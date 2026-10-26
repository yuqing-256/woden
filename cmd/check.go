package cmd

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"woden/internal/odin"
	"woden/internal/proc"
	"woden/internal/project"
)

// odinToolTimeout bounds every odinfmt and odin invocation. The tools have no
// timeout of their own, so a hung compiler must not hang Woden forever.
const odinToolTimeout = 15 * time.Minute

// checkCmd checks src/ with "odin check".
var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check src/ with odin check",
	Long: `Check the Odin sources in src/ with odin check.

The collections from woden.toml are passed to Odin as -collection arguments.`,
	Example: "  woden check",
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
		code, err := runExternalTool(cmd, "odin", p.Root, checkInvocation(p))
		if err != nil {
			return err
		}
		if code != 0 {
			return childExitError("odin", code)
		}
		return nil
	},
}

// checkInvocation returns the "odin check" arguments for p.
func checkInvocation(p *project.Project) []string {
	return odin.CheckArgs(p.CollectionArgs())
}

// runExternalTool looks tool up in PATH, runs it in dir and returns its exit
// code. A missing tool is a user error; a non-zero exit code is reported to the
// caller rather than as an error, because "woden run" forwards it verbatim.
func runExternalTool(cmd *cobra.Command, tool, dir string, args []string) (int, error) {
	if _, err := proc.Look(tool); err != nil {
		return 0, err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := (proc.ExecRunner{}).Run(ctx, tool, args, proc.Options{
		Dir:     dir,
		Stdout:  cmd.OutOrStdout(),
		Stderr:  cmd.ErrOrStderr(),
		Timeout: odinToolTimeout,
	})
	if err != nil {
		return 0, err
	}
	return result.ExitCode, nil
}

func init() {
	rootCmd.AddCommand(checkCmd)
}
