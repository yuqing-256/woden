package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yuqing-256/woden/internal/config"
	"github.com/yuqing-256/woden/internal/errs"
	"github.com/yuqing-256/woden/internal/testutil"
)

func decodeFixture(t *testing.T, rel string) (*config.Config, []string) {
	t.Helper()
	data, err := os.ReadFile(testutil.Testdata(t, rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	cfg, warnings, err := config.Decode(data)
	if err != nil {
		t.Fatalf("Decode(%s): %v", rel, err)
	}
	return cfg, warnings
}

func TestDecodeMinimal(t *testing.T) {
	cfg, warnings := decodeFixture(t, filepath.Join("toml", "vaild-minimal.toml"))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if got, want := cfg.Package.Name, "my-game"; got != want {
		t.Fatalf("package name = %q, want %q", got, want)
	}
	if got, want := cfg.Package.Version, "0.1.0"; got != want {
		t.Fatalf("package version = %q, want %q", got, want)
	}
	if got, want := cfg.Collections["app"], "src"; got != want {
		t.Fatalf("collections[app] = %q, want %q", got, want)
	}
	if len(cfg.Dependencies) != 0 {
		t.Fatalf("dependencies = %v, want none", cfg.Dependencies)
	}

	dev, err := cfg.Profile(config.ProfileDev)
	if err != nil {
		t.Fatalf("dev profile: %v", err)
	}
	if !dev.Debug || dev.OptLevel != "minimal" || dev.OutputDir != filepath.Join("target", "debug") {
		t.Fatalf("dev profile = %+v, want built-in dev defaults", dev)
	}
	release, err := cfg.Profile(config.ProfileRelease)
	if err != nil {
		t.Fatalf("release profile: %v", err)
	}
	if release.Debug || release.OptLevel != "speed" || release.OutputDir != filepath.Join("target", "release") {
		t.Fatalf("release profile = %+v, want built-in release defaults", release)
	}
}

func TestDecodeFull(t *testing.T) {
	cfg, warnings := decodeFixture(t, filepath.Join("toml", "vaild-full.toml"))
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}

	dev, err := cfg.Profile(config.ProfileDev)
	if err != nil {
		t.Fatalf("dev profile: %v", err)
	}
	if !dev.Debug || dev.OptLevel != "none" {
		t.Fatalf("dev profile = %+v, want debug with opt-level none", dev)
	}
	release, err := cfg.Profile(config.ProfileRelease)
	if err != nil {
		t.Fatalf("release profile: %v", err)
	}
	if !release.Debug || release.OptLevel != "speed" {
		t.Fatalf("release profile = %+v, want debug=true and opt-level speed", release)
	}

	if got, want := len(cfg.Dependencies), 4; got != want {
		t.Fatalf("dependency count = %d, want %d", got, want)
	}
	tests := []struct {
		name     string
		kind     string
		value    string
		git      string
		refKind  string
		refValue string
	}{
		{"alpha", "branch", "main", "https://github.com/example/alpha.git", "branch", "main"},
		{"beta", "tag", "v1.0.0", "https://github.com/example/beta.git", "tag", "v1.0.0"},
		{"gamma", "rev", "0123456789abcdef0123456789abcdef01234567", "https://github.com/example/gamma.git", "rev", "0123456789abcdef0123456789abcdef01234567"},
	}
	for _, tt := range tests {
		dep, ok := cfg.Dependencies[tt.name]
		if !ok {
			t.Fatalf("missing dependency %q", tt.name)
		}
		kind, value := dep.Ref()
		if kind != tt.refKind || value != tt.refValue {
			t.Fatalf("dependency %q ref = (%q, %q), want (%q, %q)", tt.name, kind, value, tt.refKind, tt.refValue)
		}
		if dep.Git != tt.git {
			t.Fatalf("dependency %q git = %q, want %q", tt.name, dep.Git, tt.git)
		}
	}
	delta, ok := cfg.Dependencies["delta"]
	if !ok {
		t.Fatal("missing dependency delta")
	}
	if kind, value := delta.Ref(); kind != "" || value != "" {
		t.Fatalf("delta ref = (%q, %q), want no ref (default branch)", kind, value)
	}

	names := cfg.SortedCollectionNames()
	want := []string{"app", "deps", "libs"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("sorted collections = %v, want %v", names, want)
	}
	if _, ok := cfg.DepsCollection(); !ok {
		t.Fatal("DepsCollection reports no deps mapping")
	}
	if got := cfg.SortedDependencyNames(); strings.Join(got, ",") != "alpha,beta,delta,gamma" {
		t.Fatalf("sorted dependencies = %v", got)
	}
}

func TestDecodeUnknownKeysWarn(t *testing.T) {
	cfg, warnings := decodeFixture(t, filepath.Join("toml", "unknown-key.toml"))
	if cfg == nil {
		t.Fatal("Decode returned no config")
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %v, want exactly 2", warnings)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "package.edition") || !strings.Contains(joined, "foo") {
		t.Fatalf("warnings = %v, want package.edition and foo", warnings)
	}
}

func TestDecodeRejectsInvalidFixtures(t *testing.T) {
	fixtures := []string{
		"bad-syntax.toml", "bad-profile-name.toml", "bad-opt-level.toml", "bad-debug-type.toml",
		"bad-multi-ref.toml", "bad-dep-no-git.toml", "bad-dep-name.toml", "bad-collection-reserved.toml",
		"bad-collection-escape.toml", "bad-collection-abs.toml", "bad-name.toml", "missing-package.toml",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("toml", name)))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if _, _, err := config.Decode(data); err == nil {
				t.Fatal("Decode succeeded, want an error")
			} else if !errs.IsUser(err) {
				t.Fatalf("error is not a user error: %v", err)
			}
		})
	}

	// The syntax error must point at the offending line.
	data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("toml", "bad-syntax.toml")))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	_, _, err = config.Decode(data)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("syntax error = %v, want a line 2 location", err)
	}
}

func TestDecodeMissingPackage(t *testing.T) {
	data, err := os.ReadFile(testutil.Testdata(t, filepath.Join("toml", "missing-package.toml")))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	_, _, err = config.Decode(data)
	if err == nil || !strings.Contains(err.Error(), "missing [package]") {
		t.Fatalf("error = %v, want missing [package]", err)
	}
}

func TestProfileOverridesAndUnknownProfile(t *testing.T) {
	cfg := &config.Config{Profiles: map[string]config.ProfileOverride{
		"dev": {OptLevel: stringPtr("none")},
	}}
	dev, err := cfg.Profile("dev")
	if err != nil {
		t.Fatalf("dev profile: %v", err)
	}
	if dev.OptLevel != "none" || !dev.Debug {
		t.Fatalf("dev profile = %+v, want opt-level none and debug default true", dev)
	}
	if _, err := cfg.Profile("custom"); err == nil {
		t.Fatal("unknown profile should fail")
	}

	cfg.Profiles["release"] = config.ProfileOverride{Debug: boolPtr(true)}
	release, err := cfg.Profile("release")
	if err != nil {
		t.Fatalf("release profile: %v", err)
	}
	if !release.Debug || release.OptLevel != "speed" {
		t.Fatalf("release profile = %+v, want debug=true and speed default", release)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, _, err := config.Load(filepath.Join(t.TempDir(), "woden.toml")); err == nil {
		t.Fatal("Load of a missing file should fail")
	}
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }
