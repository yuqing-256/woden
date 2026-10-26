/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/odin"
	"woden/internal/project"
)

// runCmd builds and runs the program with "odin run".
var runCmd = &cobra.Command{
	Use:   "run [-- ARGS...]",
	Short: "Build and run the program",
	Long: `Build and run the Odin program in src/ with odin run.

The dev profile is used and the executable is kept in target/debug/, so it is
shared with "woden build". The program runs in the directory where woden was
invoked. Arguments after -- are passed to the program unchanged.`,
	Example: `  woden run
  woden run -- --verbose`,
	Args:    runCommandArgs,
	GroupID: GroupBuild,
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		if err := p.CheckVendor(); err != nil {
			return err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return errs.WrapUser(err, "cannot determine the current directory")
		}
		profile, _, runArgs, err := runInvocation(p, args)
		if err != nil {
			return err
		}
		// Odin does not create the parents of -out: itself.
		if err := os.MkdirAll(p.Abs(profile.OutputDir), 0o755); err != nil {
			return errs.WrapUser(err, "cannot create %s", profile.OutputDir)
		}
		code, err := runExternalTool(cmd, "odin", cwd, runArgs)
		if err != nil {
			return err
		}
		if code != 0 {
			// Only "woden run" forwards the child's exit code; the message is
			// empty so nothing is printed.
			return errs.Exit(code, nil)
		}
		return nil
	},
}

// runCommandArgs accepts positional arguments only after "--": they belong to
// the program, not to Woden.
func runCommandArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 && cmd.ArgsLenAtDash() != 0 {
		return errs.Userf("unexpected argument %q: pass program arguments after --", args[0])
	}
	return nil
}

// runInvocation returns the dev profile, the executable path and the "odin run"
// arguments. Unlike the other commands, "odin run" runs in the directory where
// Woden was invoked, so its paths are absolute.
func runInvocation(p *project.Project, programArgs []string) (config.Profile, string, []string, error) {
	profile, err := p.Profile(config.ProfileDev)
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	outPath := filepath.Join(p.Abs(profile.OutputDir), p.ExecutableName())
	args := odin.RunArgs(profile, p.CollectionArgs(), p.SourceDir(), outPath, programArgs)
	return profile, outPath, args, nil
}

func init() {
	rootCmd.AddCommand(runCmd)
}
