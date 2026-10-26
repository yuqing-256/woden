//go:build woden_testing

package validate

// fileURLAllowed reports whether Git dependencies may use the file:// scheme.
// Test builds accept file:// so the end-to-end tests can use real local Git
// repositories without network access.
func fileURLAllowed() bool {
	return true
}
