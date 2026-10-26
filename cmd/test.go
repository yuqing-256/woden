package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"woden/internal/config"
	"woden/internal/errs"
	"woden/internal/odin"
	"woden/internal/project"
)

// testCmd runs the tests in tests/ with "odin test".
var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run the tests in tests/ with odin test",
	Long: `Run the tests in tests/ with odin test -all-packages.

The dev profile's compilation options are used. The test executable is written to
target/debug/deps/ and removed by Odin after the tests finish.`,
	Example: "  woden test",
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
		_, _, testArgs, err := testInvocation(p)
		if err != nil {
			return err
		}
		// Odin does not create the parents of -out: itself.
		if err := os.MkdirAll(p.Abs(odin.DepsDir), 0o755); err != nil {
			return errs.WrapUser(err, "cannot create %s", odin.DepsDir)
		}
		code, err := runExternalTool(cmd, "odin", p.Root, testArgs)
		if err != nil {
			return err
		}
		if code != 0 {
			return childExitError("odin", code)
		}
		return nil
	},
}

// testInvocation returns the dev profile, the test executable path and the
// "odin test" arguments.
func testInvocation(p *project.Project) (config.Profile, string, []string, error) {
	profile, err := p.Profile(config.ProfileDev)
	if err != nil {
		return config.Profile{}, "", nil, err
	}
	outPath := odin.DepsDir + "/" + p.ExecutableName()
	return profile, outPath, odin.TestArgs(profile, p.CollectionArgs(), outPath), nil
}

func init() {
	rootCmd.AddCommand(testCmd)
}
