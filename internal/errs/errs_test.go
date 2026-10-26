package errs_test

import (
	"errors"
	"strings"
	"testing"

	"woden/internal/errs"
)

func TestUserf(t *testing.T) {
	err := errs.Userf("cannot open %q", "woden.toml")
	if err == nil {
		t.Fatal("Userf returned nil")
	}
	if got, want := err.Error(), `cannot open "woden.toml"`; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if !errs.IsUser(err) {
		t.Fatal("Userf error is not a user error")
	}
	if got := errs.Message(err); got != err.Error() {
		t.Fatalf("Message = %q, want %q", got, err.Error())
	}
}

func TestWrapUser(t *testing.T) {
	cause := errors.New("permission denied")
	err := errs.WrapUser(cause, "cannot write %s", "woden.toml")
	if !errors.Is(err, cause) {
		t.Fatal("WrapUser must preserve the wrapped error")
	}
	if !errs.IsUser(err) {
		t.Fatal("WrapUser error is not a user error")
	}
	if got, want := err.Error(), "cannot write woden.toml: permission denied"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestExit(t *testing.T) {
	err := errs.Exit(101, errs.Userf("build failed"))
	code, ok := errs.Code(err)
	if !ok || code != 101 {
		t.Fatalf("Code = %d, %v; want 101, true", code, ok)
	}
	if got := errs.Message(err); !strings.Contains(got, "build failed") {
		t.Fatalf("Message = %q, want it to contain %q", got, "build failed")
	}

	quiet := errs.Exit(3, nil)
	if got := errs.Message(quiet); got != "" {
		t.Fatalf("Message of a quiet exit = %q, want empty", got)
	}
	if code, ok := errs.Code(quiet); !ok || code != 3 {
		t.Fatalf("Code = %d, %v; want 3, true", code, ok)
	}
	if code, ok := errs.Code(errors.New("plain")); ok || code != 0 {
		t.Fatalf("Code of a plain error = %d, %v; want 0, false", code, ok)
	}
}
