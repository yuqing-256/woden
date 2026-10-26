//go:build !woden_testing

package validate

// fileURLAllowed reports whether Git dependencies may use the file:// scheme.
// Released binaries must never accept it: a manifest is untrusted input, and
// file:// would let it read arbitrary local repositories.
func fileURLAllowed() bool {
	return false
}
