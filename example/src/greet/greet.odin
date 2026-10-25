package greet

import "core:fmt"

hello :: proc(name: string) -> string {
	return fmt.aprintf("Hello, %s!", name)
}
