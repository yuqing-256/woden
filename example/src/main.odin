package main

import "core:fmt"
import "core:os"
import "core:strconv"

import "app:greet"
import "libs:mathx"

main :: proc() {
	fmt.println(greet.hello("woden-example"))
	fmt.printfln("2 + 3 = %d", mathx.add(2, 3))

	args := os.args[1:]
	exit_code := 0
	for arg, i in args {
		fmt.printfln("arg[%d]=%s", i, arg)
		if arg == "--exit" && i + 1 < len(args) {
			if code, ok := strconv.parse_int(args[i + 1], 10); ok {
				exit_code = code
			}
		}
	}
	if exit_code != 0 {
		os.exit(exit_code)
	}
}
