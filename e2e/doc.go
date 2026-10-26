// Package e2e contains Woden's end-to-end tests.
//
// The tests are guarded by the e2e build tag (dependency scenarios additionally
// by woden_testing) so that ordinary test runs never require a real Odin
// toolchain or Git remote transport. They build the woden binary in TestMain
// and exercise it against temporary copies of example/ only; the repository's
// example/ and testdata/ trees are never written to.
package e2e
