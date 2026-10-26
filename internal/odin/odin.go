// Package odin builds the Odin command lines Woden runs and checks.
//
// The builders are pure functions so their exact arguments can be asserted in
// unit tests without executing Odin.
package odin

import (
	"runtime"

	"woden/internal/config"
)

// Directories passed to Odin, relative to the project root.
const (
	SrcDir     = "src"
	TestsDir   = "tests"
	TargetDir  = "target"
	DebugDir   = "target/debug"
	ReleaseDir = "target/release"
	DepsDir    = "target/debug/deps"
)

// ExeName returns name with the platform executable extension.
func ExeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FmtArgs returns the arguments for odinfmt.
func FmtArgs(dir string) []string {
	return []string{dir, "-w"}
}

// CheckArgs returns the arguments for "odin check".
func CheckArgs(collections []string) []string {
	return concat([]string{"check", SrcDir}, collections)
}

// LintArgs returns the arguments for "odin check -vet".
func LintArgs(collections []string) []string {
	return concat([]string{"check", SrcDir, "-vet"}, collections)
}

// TestArgs returns the arguments for "odin test". outPath is the executable
// path (including the platform extension).
func TestArgs(profile config.Profile, collections []string, outPath string) []string {
	args := []string{"test", TestsDir, "-all-packages", "-out:" + outPath}
	args = append(args, profileFlags(profile)...)
	return concat(args, collections)
}

// BuildArgs returns the arguments for "odin build".
func BuildArgs(profile config.Profile, collections []string, outPath string) []string {
	args := []string{"build", SrcDir, "-out:" + outPath}
	args = append(args, profileFlags(profile)...)
	return concat(args, collections)
}

// RunArgs returns the arguments for "odin run". programArgs are passed to the
// program after "--".
func RunArgs(profile config.Profile, collections []string, srcPath, outPath string, programArgs []string) []string {
	args := []string{"run", srcPath, "-out:" + outPath, "-keep-executable"}
	args = append(args, profileFlags(profile)...)
	args = concat(args, collections)
	if len(programArgs) > 0 {
		args = append(args, "--")
		args = append(args, programArgs...)
	}
	return args
}

// profileFlags converts a profile into Odin flags.
func profileFlags(p config.Profile) []string {
	var flags []string
	if p.Debug {
		flags = append(flags, "-debug")
	}
	if p.OptLevel != "" {
		flags = append(flags, "-o:"+p.OptLevel)
	}
	return flags
}

// concat returns a new slice with a and b appended.
func concat(a, b []string) []string {
	out := make([]string, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}
