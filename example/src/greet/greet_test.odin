package greet

import "core:testing"

@(test)
test_hello_empty :: proc(t: ^testing.T) {
	testing.expect_value(t, hello(""), "Hello, !")
}
