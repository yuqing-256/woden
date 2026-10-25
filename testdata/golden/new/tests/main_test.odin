package tests

import "core:testing"

@(test)
test_sanity :: proc(t: ^testing.T) {
	testing.expect_value(t, 1 + 1, 2)
}
