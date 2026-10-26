//go:build woden_testing

package validate_test

import (
	"testing"

	"github.com/yuqing-256/woden/internal/validate"
)

func TestGitURLAcceptsFileURLWithTestingTag(t *testing.T) {
	if err := validate.GitURL("file:///tmp/repo"); err != nil {
		t.Fatalf("file:// URLs must be accepted with the woden_testing tag: %v", err)
	}
}
