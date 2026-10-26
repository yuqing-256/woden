//go:build !woden_testing

package gitdep

import "testing"

// TestIntegrationTestsRequireBuildTag tells developers why the real-Git
// integration tests did not run. Git URL validation only accepts file:// URLs
// in the woden_testing build, and those URLs are how the tests reach
// testutil.NewDepRepo, so they are compiled only with that tag.
func TestIntegrationTestsRequireBuildTag(t *testing.T) {
	t.Skip("real-Git integration tests are skipped: run `go test -tags woden_testing ./internal/gitdep/`")
}
