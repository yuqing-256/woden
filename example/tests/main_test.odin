package tests

import "app:greet"
import "core:testing"
import "libs:mathx"

@(test)
test_hello_x :: proc(t: ^testing.T) {
	testing.expect_value(t, greet.hello("x"), "Hello, x!")
}

@(test)
test_add_2_3 :: proc(t: ^testing.T) {
	testing.expect_value(t, mathx.add(2, 3), 5)
}
