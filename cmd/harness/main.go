package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// main is the harness CLI entry point. It:
// 1. Sets up signal-based context cancellation (SIGINT/SIGTERM).
// 2. Creates the root command with all persistent flags.
// 3. Executes the command tree via Cobra.
// 4. Maps the returned error to a process exit code via exitCodeFor.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for OS signals and cancel the context.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		// Record which signal fired so exitCodeFor can pick the right code.
		if sig == syscall.SIGTERM {
			ctx = SetCancelSignal(ctx, "SIGTERM")
		} else {
			ctx = SetCancelSignal(ctx, "SIGINT")
		}
		cancel()
	}()

	cfg := &Config{}
	root := newRootCmd(ctx, cfg)

	err := root.ExecuteContext(ctx)
	code := exitCodeFor(err, ctx)

	// Print the error if it's not nil and not one of our well-known sentinels.
	if err != nil && code != ExitOK {
		fmt.Fprintf(os.Stderr, "harness: %v\n", err)
	}

	os.Exit(code)
}
