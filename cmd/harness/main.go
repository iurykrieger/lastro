package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// main is the harness CLI entry point. It:
//  1. Wires SIGINT/SIGTERM into a NotifyContext for clean cancellation.
//  2. Mirrors the same signals into a buffered channel so the
//     post-cancellation exit-code mapper can disambiguate SIGINT (130)
//     vs SIGTERM (143).
//  3. Creates the root command with all persistent flags and
//     dispatches via Cobra.
//  4. Maps the returned error to a process exit code via exitCodeFor
//     and exits.
func main() {
	// signal.NotifyContext gives us a context that's auto-canceled when
	// SIGINT or SIGTERM arrives. The cancellation propagates through
	// Cobra's ExecuteContext to lifecycle.RunSensor and friends.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// A separate buffered channel captures *which* signal fired so we
	// can pick the right exit code (130 vs 143). Go's signal package
	// supports multiple subscribers — NotifyContext's internal handler
	// and our sigCh both receive the signal independently.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	cfg := &Config{}
	root := newRootCmd(ctx, cfg)

	err := root.ExecuteContext(ctx)

	// If a signal fired (which is what canceled ctx), it is already
	// sitting in sigCh — channel reads synchronize with the goroutine
	// that delivered it, so this is race-free.
	select {
	case sig := <-sigCh:
		ctx = SetCancelSignal(ctx, sig.String())
	default:
	}

	code := exitCodeFor(err, ctx)

	if err != nil && code != ExitOK {
		fmt.Fprintf(os.Stderr, "harness: %v\n", err)
	}

	os.Exit(code)
}
