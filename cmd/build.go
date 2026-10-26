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

// buildCmd builds the project with "odin build".
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build the project",
	Long: `Build the Odin program in src/.

The dev profile is used by default and writes target/debug/<package.name>. With
--release the release profile writes target/release/<package.name>.`,
	Example: `  woden build
  woden build --release`,
	Args:    cobra.NoArgs,
	GroupID: GroupBuild,
	RunE: func(cmd *cobra.Command, args []string) error {
		release, err := cmd.Flags().GetBool("release")
		if err != nil {
			return errs.WrapUser(err, "invalid arguments")
		}
		p, err := projectFromCommand(cmd)
		if err != nil {
			return err
		}
		if err := p.CheckVendor(); err != nil {
			return err
		}
		profile, _, buildArgs, err := buildInvocation(p, release)
		if err != nil {
			return err
		}
		// Odin does not create the parents of -out: itself.
		if err := os.MkdirAll(p.Abs(profile.OutputDir), 0o755); err != nil {
			return errs.WrapUser(err, "cannot create %s", profile.OutputDir)
		}
		code, err := runExternalTool(cmd, "odin", p.Root, buildArgs)
		if err != nil {
			return err
		}
		if code != 0 {
			return childExitError("odin", code)
		}
		return nil
	},
}

// buildInvocation returns the profile, the executable path and the "odin build"
// arguments: the dev profile by default, the release profile with release.
func buildInvocation(p *project.Project, release bool) (config.Profile, string, []string, error) {
	name := config.ProfileDev
	if release {
		name = config.ProfileRelease
	}
	profile, err := p.Profile(name)
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	// The output path is written with forward slashes, as documented, even on
	// Windows where profile.OutputDir uses the native separator.
	outPath := filepath.ToSlash(filepath.Join(profile.OutputDir, p.ExecutableName()))
	return profile, outPath, odin.BuildArgs(profile, p.CollectionArgs(), outPath), nil
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().Bool("release", false, "Build with the release profile")
}
