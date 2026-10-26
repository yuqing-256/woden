package validate_test

import (
	"errors"
	"testing"

	"github.com/yuqing-256/woden/internal/validate"
)

func checkTable(t *testing.T, name string, fn func(string) error, valid, invalid []string) {
	t.Helper()
	for _, value := range valid {
		if err := fn(value); err != nil {
			t.Errorf("%s(%q) = %v, want nil", name, value, err)
		}
	}
	for _, value := range invalid {
		if err := fn(value); err == nil {
			t.Errorf("%s(%q) = nil, want an error", name, value)
		}
	}
}

func TestProjectName(t *testing.T) {
	checkTable(t, "ProjectName", validate.ProjectName,
		[]string{"my-game", "a", "_private", "game2", "A-B_c", "woden-example"},
		[]string{"", "1game", "-game", "a b", "con", "NUL", "com1", "a.b", "..", "a/b", "a\\b", "a:b", "game."},
	)
}

func TestDependencyName(t *testing.T) {
	checkTable(t, "DependencyName", validate.DependencyName,
		[]string{"alpha", "example-library", "a_b", "lib.v2"},
		[]string{"", ".", "..", "a/b", "a\\b", "a:b", "-alpha", ".git", "CON", "a\x00b", "a b", "trailing.", "trailing "},
	)
}

func TestCollectionName(t *testing.T) {
	checkTable(t, "CollectionName", validate.CollectionName,
		[]string{"app", "libs", "deps", "my-lib", "_x"},
		[]string{"", "core", "base", "vendor", "a/b", "a:b", "a=b", "a b", "a\x01b", "-x"},
	)
}

func TestCollectionPath(t *testing.T) {
	checkTable(t, "CollectionPath", validate.CollectionPath,
		[]string{"src", "libs/sub", "a", "./src", "src/vendor-ish"},
		[]string{"", "../outside", "a/../b", "/etc", "C:\\libs", `\\server\share`, "a:b", "a\x00b"},
	)
}

func TestGitURL(t *testing.T) {
	checkTable(t, "GitURL", validate.GitURL,
		[]string{
			"https://github.com/example/alpha.git",
			"ssh://git@github.com/example/alpha.git",
			"git://github.com/example/alpha.git",
			"git@github.com:example/alpha.git",
		},
		[]string{
			"",
			"-https://github.com/example/alpha.git",
			"https://github.com/a b",
			"ext::sh -c 'touch /tmp/pwned'",
			"C:\\repos\\alpha",
			`\\server\share`,
			"plain-path",
			"https://",
			"https://github.com/a\tb",
		},
	)
}

func TestRevAndCommitID(t *testing.T) {
	valid := []string{"0123456", "0123456789abcdef0123456789abcdef01234567", "ABCDEF0"}
	invalid := []string{"", "012345", "0123456789abcdef0123456789abcdef012345678", "zzzzzzz", "abc def"}
	checkTable(t, "Rev", validate.Rev, valid, invalid)

	if err := validate.CommitID("0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatalf("CommitID: %v", err)
	}
	for _, value := range []string{"", "fedcba9", "0123456789abcdef0123456789abcdef0123456g"} {
		if err := validate.CommitID(value); err == nil {
			t.Fatalf("CommitID(%q) = nil, want an error", value)
		}
	}
}

func TestRefName(t *testing.T) {
	checkTable(t, "RefName", validate.RefName,
		[]string{"main", "v1.0.0", "feature/x", "release-2026"},
		[]string{"", "-main", "a..b", "a b", "a~b", "a^b", "a:b", "a?b", "a*b", "a[b", "a\\b", "a@{b}", "@", "/main", ".main", "main/", "main.", "main.lock", "a//b"},
	)
}

func TestErrorsArePlain(t *testing.T) {
	if err := validate.ProjectName("1game"); err == nil || errors.Is(err, errors.New("")) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}
