package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"woden/cmd"
	"woden/internal/errs"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx))
}

// run executes the CLI and returns the exit code: 0 on success, 101 on a Woden
// failure, and the child's exit code when "woden run" reports one.
func run(ctx context.Context) int {
	err := cmd.Execute(ctx)
	if err == nil {
		return 0
	}
	if code, ok := errs.Code(err); ok {
		if message := errs.Message(err); message != "" {
			fmt.Fprintf(os.Stderr, "error: %s\n", message)
		}
		return code
	}
	if errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "error: interrupted")
		return 101
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", errs.Message(err))
	return 101
}
