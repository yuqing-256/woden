//go:build !woden_testing

package validate_test

import (
	"testing"

	"woden/internal/validate"
)

func TestGitURLRejectsFileURLByDefault(t *testing.T) {
	if err := validate.GitURL("file:///tmp/repo"); err == nil {
		t.Fatal("file:// URLs must be rejected in release builds")
	}
}
