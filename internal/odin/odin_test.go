package odin_test

import (
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"woden/internal/config"
	"woden/internal/odin"
)

func profiles(t *testing.T) (config.Profile, config.Profile) {
	t.Helper()
	cfg := &config.Config{Profiles: map[string]config.ProfileOverride{}}
	dev, err := cfg.Profile(config.ProfileDev)
	if err != nil {
		t.Fatalf("dev profile: %v", err)
	}
	release, err := cfg.Profile(config.ProfileRelease)
	if err != nil {
		t.Fatalf("release profile: %v", err)
	}
	return dev, release
}

func TestFmtArgs(t *testing.T) {
	if got, want := odin.FmtArgs("src"), []string{"src", "-w"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("FmtArgs = %v, want %v", got, want)
	}
}

func TestCheckAndLintArgs(t *testing.T) {
	colls := []string{"-collection:app=/p/src", "-collection:libs=/p/libs"}
	if got, want := odin.CheckArgs(colls), []string{"check", "src", colls[0], colls[1]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CheckArgs = %v, want %v", got, want)
	}
	if got, want := odin.LintArgs(colls), []string{"check", "src", "-vet", colls[0], colls[1]}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LintArgs = %v, want %v", got, want)
	}
}

func TestTestArgs(t *testing.T) {
	dev, _ := profiles(t)
	colls := []string{"-collection:app=/p/src"}
	out := filepath.Join("target", "debug", "deps", odin.ExeName("my-game"))
	got := odin.TestArgs(dev, colls, out)
	want := []string{"test", "tests", "-all-packages", "-out:" + out, "-debug", "-o:minimal", colls[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TestArgs = %v, want %v", got, want)
	}
}

func TestBuildArgs(t *testing.T) {
	dev, release := profiles(t)
	colls := []string{"-collection:app=/p/src"}

	debugOut := filepath.Join("target", "debug", odin.ExeName("my-game"))
	got := odin.BuildArgs(dev, colls, debugOut)
	want := []string{"build", "src", "-out:" + debugOut, "-debug", "-o:minimal", colls[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs(dev) = %v, want %v", got, want)
	}

	releaseOut := filepath.Join("target", "release", odin.ExeName("my-game"))
	got = odin.BuildArgs(release, colls, releaseOut)
	want = []string{"build", "src", "-out:" + releaseOut, "-o:speed", colls[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildArgs(release) = %v, want %v", got, want)
	}
}

func TestRunArgs(t *testing.T) {
	dev, _ := profiles(t)
	colls := []string{"-collection:app=/p/src"}
	src := filepath.Join("p", "src")
	out := filepath.Join("p", "target", "debug", odin.ExeName("my-game"))

	got := odin.RunArgs(dev, colls, src, out, nil)
	want := []string{"run", src, "-out:" + out, "-keep-executable", "-debug", "-o:minimal", colls[0]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunArgs = %v, want %v", got, want)
	}

	got = odin.RunArgs(dev, colls, src, out, []string{"a", "b c"})
	want = append(want, "--", "a", "b c")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunArgs(with args) = %v, want %v", got, want)
	}
}

func TestExeName(t *testing.T) {
	want := "my-game"
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got := odin.ExeName("my-game"); got != want {
		t.Fatalf("ExeName = %q, want %q", got, want)
	}
}
