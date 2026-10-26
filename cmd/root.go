/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"woden/internal/errs"
	"woden/internal/project"
)

// version is injected at build time with
//
//	go build -ldflags "-X woden/cmd.version=v1.2.3"
var version = "dev"

// Command group identifiers. Every command belongs to exactly one group so
// "woden help" can present the commands the way README.md documents them.
const (
	GroupProject    = "project"
	GroupBuild      = "build"
	GroupDependency = "dependency"
)

// NoProjectAnnotation marks commands that do not require a woden.toml (new and
// init, which create one).
const NoProjectAnnotation = "woden.io/no-project"

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "woden",
	Short: "A project manager for the Odin programming language",
	Long: `Woden is a project manager for the Odin programming language.

It creates new projects, formats and checks source code, runs tests, builds
and runs programs, and manages Git dependencies declared in woden.toml.`,
	Example: `  woden new my-game
  woden check
  woden build --release
  woden run -- --verbose`,
	Version:       version,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Annotations[NoProjectAnnotation] == "true" {
			return nil
		}
		// Cobra's own commands never need a project.
		switch cmd.Name() {
		case "help", "completion", "version":
			return nil
		}
		cwd, err := os.Getwd()
		if err != nil {
			return errs.WrapUser(err, "cannot determine the current directory")
		}
		p, err := project.Load(cwd)
		if err != nil {
			return err
		}
		cmd.SetContext(project.WithContext(cmd.Context(), p))
		for _, warning := range p.Warnings {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	rootCmd.AddGroup(
		&cobra.Group{ID: GroupProject, Title: "Project commands:"},
		&cobra.Group{ID: GroupBuild, Title: "Build commands:"},
		&cobra.Group{ID: GroupDependency, Title: "Dependency commands:"},
	)
	rootCmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return errs.WrapUser(err, "invalid arguments")
	})
}

// Execute runs the CLI with the given context. The context is cancelled on
// Ctrl+C by main, so long-running child processes are killed as well.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

// projectFromCommand returns the project loaded by the root command's
// PersistentPreRunE.
func projectFromCommand(cmd *cobra.Command) (*project.Project, error) {
	p, ok := project.FromContext(cmd.Context())
	if !ok {
		return nil, errs.Userf("internal error: no project in command context")
	}
	return p, nil
}

// childExitError reports a non-zero exit code of an external tool as a Woden
// failure.
func childExitError(tool string, code int) error {
	return errs.Userf("%s exited with code %d", tool, code)
}
