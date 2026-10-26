package cmd

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"woden/internal/project"
	"woden/internal/testutil"
)

// odinTestWriteProject writes content as woden.toml in a fresh temporary
// directory and loads the project from it.
func odinTestWriteProject(t *testing.T, content string) *project.Project {
	t.Helper()
	dir := t.TempDir()
	testutil.WriteFile(t, filepath.Join(dir, "woden.toml"), content)
	p, err := project.Load(dir)
	if err != nil {
		t.Fatalf("cannot load the test project: %v", err)
	}
	return p
}

// odinTestTomlFixture returns the content of a testdata/toml fixture. The
// checked-in files spell "valid" as "vaild", so both spellings are accepted.
func odinTestTomlFixture(t *testing.T, base string) string {
	t.Helper()
	for _, name := range []string{"valid-" + base + ".toml", "vaild-" + base + ".toml"} {
		path := testutil.Testdata(t, filepath.Join("toml", name))
		if data, err := os.ReadFile(path); err == nil {
			return string(data)
		}
	}
	t.Fatalf("missing testdata/toml fixture for %q", base)
	return ""
}

// odinTestMkdir creates a directory below the project root.
func odinTestMkdir(t *testing.T, p *project.Project, rel string) *project.Project {
	t.Helper()
	if err := os.MkdirAll(p.Abs(rel), 0o755); err != nil {
		t.Fatalf("cannot create %s: %v", rel, err)
	}
	return p
}

// TestFormatDirectories checks which directories "woden fmt" formats.
func TestFormatDirectories(t *testing.T) {
	toml := odinTestTomlFixture(t, "minimal")
	cases := []struct {
		name  string
		mkdir []string
		want  []string
	}{
		{"nothing to format", nil, nil},
		{"src only", []string{"src"}, []string{"src"}},
		{"tests only", []string{"tests"}, []string{"tests"}},
		{"src and tests", []string{"src", "tests"}, []string{"src", "tests"}},
		{"libs is never formatted", []string{"libs", "src", "tests", "vendor"}, []string{"src", "tests"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := odinTestWriteProject(t, toml)
			for _, name := range tc.mkdir {
				odinTestMkdir(t, p, name)
			}
			got, err := formatDirectories(p)
			if len(tc.want) == 0 {
				if err == nil {
					t.Fatalf("formatDirectories(%v) succeeded, want an error", tc.mkdir)
				}
				if !strings.Contains(err.Error(), "neither src/ nor tests/") {
					t.Fatalf("error = %q, want it to mention neither src/ nor tests/", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("formatDirectories() error = %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("formatDirectories() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCheckAndLintInvocation checks the exact "odin check" and "odin check
// -vet" argument lists, including the collection arguments and their order.
func TestCheckAndLintInvocation(t *testing.T) {
	p := odinTestWriteProject(t, odinTestTomlFixture(t, "full"))
	collections := []string{
		"-collection:app=" + p.Abs("src"),
		"-collection:deps=" + p.Abs("vendor"),
		"-collection:libs=" + p.Abs("libs"),
	}
	wantCheck := append([]string{"check", "src"}, collections...)
	if got := checkInvocation(p); !slices.Equal(got, wantCheck) {
		t.Fatalf("checkInvocation() = %q, want %q", got, wantCheck)
	}
	wantLint := append([]string{"check", "src", "-vet"}, collections...)
	if got := lintInvocation(p); !slices.Equal(got, wantLint) {
		t.Fatalf("lintInvocation() = %q, want %q", got, wantLint)
	}
}

// TestTestInvocation checks the "odin test" arguments and output path.
func TestTestInvocation(t *testing.T) {
	t.Run("dev profile defaults", func(t *testing.T) {
		p := odinTestWriteProject(t, odinTestTomlFixture(t, "minimal"))
		profile, outPath, args, err := testInvocation(p)
		if err != nil {
			t.Fatalf("testInvocation() error = %v", err)
		}
		if profile.Name != "dev" || !profile.Debug || profile.OptLevel != "minimal" {
			t.Fatalf("profile = %+v, want the default dev profile", profile)
		}
		wantOut := "target/debug/deps/" + p.ExecutableName()
		if outPath != wantOut {
			t.Fatalf("output path = %q, want %q", outPath, wantOut)
		}
		want := []string{
			"test", "tests", "-all-packages", "-out:" + wantOut,
			"-debug", "-o:minimal",
			"-collection:app=" + p.Abs("src"),
		}
		if !slices.Equal(args, want) {
			t.Fatalf("testInvocation() args = %q, want %q", args, want)
		}
	})
	t.Run("profile overrides", func(t *testing.T) {
		p := odinTestWriteProject(t, odinTestTomlFixture(t, "full"))
		profile, outPath, args, err := testInvocation(p)
		if err != nil {
			t.Fatalf("testInvocation() error = %v", err)
		}
		if !profile.Debug || profile.OptLevel != "none" {
			t.Fatalf("profile = %+v, want debug with opt-level none", profile)
		}
		wantOut := "target/debug/deps/" + p.ExecutableName()
		if outPath != wantOut {
			t.Fatalf("output path = %q, want %q", outPath, wantOut)
		}
		want := []string{
			"test", "tests", "-all-packages", "-out:" + wantOut,
			"-debug", "-o:none",
			"-collection:app=" + p.Abs("src"),
			"-collection:deps=" + p.Abs("vendor"),
			"-collection:libs=" + p.Abs("libs"),
		}
		if !slices.Equal(args, want) {
			t.Fatalf("testInvocation() args = %q, want %q", args, want)
		}
	})
}

// TestBuildInvocation checks the "odin build" arguments for both profiles,
// including the profile overrides from woden.toml.
func TestBuildInvocation(t *testing.T) {
	cases := []struct {
		name     string
		fixture  string
		release  bool
		debug    bool
		optLevel string
		outDir   string
		full     bool
	}{
		{"dev defaults", "minimal", false, true, "minimal", "target/debug", false},
		{"release defaults", "minimal", true, false, "speed", "target/release", false},
		{"dev overrides", "full", false, true, "none", "target/debug", true},
		{"release overrides", "full", true, true, "speed", "target/release", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := odinTestWriteProject(t, odinTestTomlFixture(t, tc.fixture))
			profile, outPath, args, err := buildInvocation(p, tc.release)
			if err != nil {
				t.Fatalf("buildInvocation() error = %v", err)
			}
			if profile.Debug != tc.debug || profile.OptLevel != tc.optLevel {
				t.Fatalf("profile = %+v, want debug=%v opt-level=%q", profile, tc.debug, tc.optLevel)
			}
			wantOut := tc.outDir + "/" + p.ExecutableName()
			if outPath != wantOut {
				t.Fatalf("output path = %q, want %q", outPath, wantOut)
			}
			want := []string{"build", "src", "-out:" + wantOut}
			if tc.debug {
				want = append(want, "-debug")
			}
			want = append(want, "-o:"+tc.optLevel)
			want = append(want, "-collection:app="+p.Abs("src"))
			if tc.full {
				want = append(want,
					"-collection:deps="+p.Abs("vendor"),
					"-collection:libs="+p.Abs("libs"),
				)
			}
			if !slices.Equal(args, want) {
				t.Fatalf("buildInvocation() args = %q, want %q", args, want)
			}
		})
	}
}

// TestRunInvocation checks the "odin run" argument list: absolute paths, the
// kept executable and program arguments after "--".
func TestRunInvocation(t *testing.T) {
	p := odinTestWriteProject(t, odinTestTomlFixture(t, "minimal"))
	srcPath := p.SourceDir()
	outPath := filepath.Join(p.Abs("target/debug"), p.ExecutableName())
	base := []string{
		"run", srcPath, "-out:" + outPath, "-keep-executable",
		"-debug", "-o:minimal",
		"-collection:app=" + p.Abs("src"),
	}

	t.Run("no program arguments", func(t *testing.T) {
		profile, gotOut, args, err := runInvocation(p, nil)
		if err != nil {
			t.Fatalf("runInvocation() error = %v", err)
		}
		if profile.Name != "dev" {
			t.Fatalf("profile name = %q, want dev", profile.Name)
		}
		if gotOut != outPath {
			t.Fatalf("output path = %q, want %q", gotOut, outPath)
		}
		if !slices.Equal(args, base) {
			t.Fatalf("runInvocation() args = %q, want %q", args, base)
		}
	})
	t.Run("program arguments after --", func(t *testing.T) {
		_, _, args, err := runInvocation(p, []string{"a", "b c"})
		if err != nil {
			t.Fatalf("runInvocation() error = %v", err)
		}
		want := append(slices.Clone(base), "--", "a", "b c")
		if !slices.Equal(args, want) {
			t.Fatalf("runInvocation() args = %q, want %q", args, want)
		}
	})
}

// TestRunCommandArgs checks that "woden run" only accepts program arguments
// after "--".
func TestRunCommandArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no arguments", nil, ""},
		{"arguments after dashes", []string{"--", "a", "b c"}, ""},
		{"flag-like program arguments", []string{"--", "--exit", "3"}, ""},
		{"empty program arguments", []string{"--"}, ""},
		{"bare positional", []string{"a"}, `unexpected argument "a": pass program arguments after --`},
		{"positional before dashes", []string{"a", "--", "b"}, `unexpected argument "a": pass program arguments after --`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &cobra.Command{
				Use:           "run",
				Args:          runCommandArgs,
				RunE:          func(*cobra.Command, []string) error { return nil },
				SilenceUsage:  true,
				SilenceErrors: true,
			}
			c.SetArgs(tc.args)
			c.SetOut(io.Discard)
			c.SetErr(io.Discard)
			err := c.Execute()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Execute() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Execute() succeeded, want error %q", tc.wantErr)
			}
			if got := err.Error(); got != tc.wantErr {
				t.Fatalf("Execute() error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

// TestOdinCommandMetadata checks the Cobra metadata of the Odin-invoking
// commands: group, help fields, argument validation and flags.
func TestOdinCommandMetadata(t *testing.T) {
	commands := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"fmt", fmtCmd},
		{"check", checkCmd},
		{"lint", lintCmd},
		{"test", testCmd},
		{"build", buildCmd},
		{"run", runCmd},
	}
	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cmd.GroupID != GroupBuild {
				t.Errorf("GroupID = %q, want %q", tc.cmd.GroupID, GroupBuild)
			}
			if tc.cmd.RunE == nil {
				t.Error("RunE is nil")
			}
			if tc.cmd.Args == nil {
				t.Error("Args is nil")
			}
			if tc.cmd.PreRunE != nil || tc.cmd.PostRunE != nil {
				t.Error("the command must not define PreRunE or PostRunE")
			}
			for _, field := range []struct {
				name  string
				value string
			}{
				{"Use", tc.cmd.Use},
				{"Short", tc.cmd.Short},
				{"Long", tc.cmd.Long},
				{"Example", tc.cmd.Example},
			} {
				if strings.TrimSpace(field.value) == "" {
					t.Errorf("%s is empty", field.name)
				}
			}
			registered := false
			for _, c := range rootCmd.Commands() {
				if c == tc.cmd {
					registered = true
					break
				}
			}
			if !registered {
				t.Errorf("%s is not registered with the root command", tc.cmd.Name())
			}
		})
	}
	if buildCmd.Flags().Lookup("release") == nil {
		t.Fatal("build --release flag is missing")
	}
	if got := odinTestLocalFlags(buildCmd); !slices.Equal(got, []string{"release"}) {
		t.Fatalf("build flags = %q, want only --release", got)
	}
	for _, c := range []*cobra.Command{fmtCmd, checkCmd, lintCmd, testCmd, runCmd} {
		if got := odinTestLocalFlags(c); len(got) != 0 {
			t.Fatalf("%s flags = %q, want none", c.Name(), got)
		}
	}
}

// odinTestLocalFlags returns the names of the flags defined locally on c,
// excluding the help flag cobra adds when the command executes.
func odinTestLocalFlags(c *cobra.Command) []string {
	var names []string
	for _, line := range strings.Split(c.Flags().FlagUsages(), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		for _, part := range strings.Split(strings.Fields(line)[0], ", ") {
			if !strings.HasPrefix(part, "--") {
				continue
			}
			if name := strings.TrimPrefix(part, "--"); name != "help" {
				names = append(names, name)
			}
		}
	}
	slices.Sort(names)
	return names
}
