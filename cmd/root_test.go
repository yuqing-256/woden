package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// commandNames lists the commands Woden provides. The README is the only source
// of commands, so this list is intentionally explicit.
var commandNames = []string{
	"add", "build", "check", "clean", "fmt", "init", "lint", "new",
	"remove", "run", "sync", "test", "update",
}

// runRoot executes the root command with args and returns combined output.
func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	}()
	rootCmd.SetArgs(args)
	// Cobra keeps flag values on the command between Execute calls; reset the
	// built-in bool flags so tests are independent of each other.
	for _, name := range []string{"help", "version"} {
		if f := rootCmd.Flags().Lookup(name); f != nil {
			_ = f.Value.Set("false")
		}
	}
	err := rootCmd.ExecuteContext(context.Background())
	return buf.String(), err
}

func TestRootHelpListsEveryCommand(t *testing.T) {
	out, err := runRoot(t, "--help")
	if err != nil {
		t.Fatalf("--help failed: %v\n%s", err, out)
	}
	for _, name := range commandNames {
		if !strings.Contains(out, "\n  "+name+" ") {
			t.Errorf("help output does not list command %q:\n%s", name, out)
		}
	}
	for _, group := range []string{"Project commands:", "Build commands:", "Dependency commands:"} {
		if !strings.Contains(out, group) {
			t.Errorf("help output does not contain command group %q:\n%s", group, out)
		}
	}
}

func TestRootVersion(t *testing.T) {
	out, err := runRoot(t, "--version")
	if err != nil {
		t.Fatalf("--version failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dev") {
		t.Errorf("version output %q does not contain the version", out)
	}
}

func TestRootUnknownCommand(t *testing.T) {
	if _, err := runRoot(t, "frobnicate"); err == nil {
		t.Fatal("unknown command must fail")
	}
}

func TestCommandGroupAssignment(t *testing.T) {
	want := map[string]string{
		"new": GroupProject, "init": GroupProject, "clean": GroupProject,
		"fmt": GroupBuild, "check": GroupBuild, "lint": GroupBuild,
		"test": GroupBuild, "build": GroupBuild, "run": GroupBuild,
		"add": GroupDependency, "remove": GroupDependency,
		"sync": GroupDependency, "update": GroupDependency,
	}
	for _, name := range commandNames {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		if got := cmd.GroupID; got != want[name] {
			t.Errorf("%s GroupID = %q, want %q", name, got, want[name])
		}
		if strings.TrimSpace(cmd.Short) == "" {
			t.Errorf("%s has an empty Short description", name)
		}
		if cmd.Use == "" || !strings.HasPrefix(cmd.Use, name) {
			t.Errorf("%s Use = %q, want it to start with the command name", name, cmd.Use)
		}
	}
}

// TestCommandsRequiringProject verifies that only new and init skip the
// woden.toml preflight.
func TestCommandsRequiringProject(t *testing.T) {
	noProject := map[string]bool{"new": true, "init": true}
	for _, name := range commandNames {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		isNoProject := cmd.Annotations[NoProjectAnnotation] == "true"
		if isNoProject != noProject[name] {
			t.Errorf("%s %s annotation = %v, want %v", name, NoProjectAnnotation, isNoProject, noProject[name])
		}
	}
}

func TestPreRunRequiresProject(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for _, name := range []string{"check", "lint", "build", "sync"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		cmd.SetContext(context.Background())
		if err := rootCmd.PersistentPreRunE(cmd, nil); err == nil {
			t.Errorf("%s outside a project: pre-run did not fail", name)
		}
	}

	for _, name := range []string{"new", "init"} {
		cmd, _, err := rootCmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %q: %v", name, err)
		}
		if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
			t.Errorf("%s must not require a project: %v", name, err)
		}
	}
}

func TestPreRunLoadsProjectAndPrintsWarnings(t *testing.T) {
	dir := t.TempDir()
	content := "[package]\nname = \"probe\"\nversion = \"0.1.0\"\n\n[collections]\napp = \"src\"\n\n[unknown]\nkey = 1\n"
	if err := os.WriteFile(filepath.Join(dir, "woden.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	cmd, _, err := rootCmd.Find([]string{"check"})
	if err != nil {
		t.Fatalf("find check: %v", err)
	}
	cmd.SetContext(context.Background())
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)
	cmd.SetOut(&stderr)
	if err := rootCmd.PersistentPreRunE(cmd, nil); err != nil {
		t.Fatalf("pre-run failed: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning:") {
		t.Errorf("unknown key warning was not printed: %q", stderr.String())
	}
}
