// Package main is the harness CLI entry point. The exported helpers
// (Config, newRootCmd) are reachable inside the cmd/harness package
// for tests; outside callers should go through main().
package main

import (
	"context"
	"runtime"
	"time"

	"github.com/spf13/cobra"
)

// Config carries flag-derived configuration through the command tree.
// One instance lives in main(); subcommand handlers read it via the
// closure newValidateCmd/newHealCmd/... establish over it.
type Config struct {
	// Persistent flags.
	Output      string // "text" | "json"
	Quiet       bool
	Verbose     bool
	Policy      string        // path to validation-policy.yaml override
	RepoRoot    string        // repo root override (default: cwd)
	Concurrency int           // 0 = GOMAXPROCS
	Timeout     time.Duration // 0 = no cap
}

// HarnessVersion is set by the linker via -ldflags in release builds.
// Defaults to "dev" so unbuilt invocations are obvious in output.
var HarnessVersion = "dev"

// newRootCmd assembles the Cobra root and binds all persistent flags
// to cfg. ctx is the root context already wired with signal-based
// cancellation in main().
func newRootCmd(ctx context.Context, cfg *Config) *cobra.Command {
	root := &cobra.Command{
		Use:   "harness",
		Short: "Use-case-driven validation framework",
		Long: `harness validates a repository against detected use cases.

It composes the deterministic runtime (sensor execution + verdict
aggregation) into operator-friendly subcommands. See ` + "`harness validate --help`" + `
for the v1 surface.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVar(&cfg.Output, "output", "text", `output format: "text" | "json"`)
	root.PersistentFlags().BoolVar(&cfg.Quiet, "quiet", false, "suppress info logs (still emits errors)")
	root.PersistentFlags().BoolVar(&cfg.Verbose, "verbose", false, "emit debug logs to stderr")
	root.PersistentFlags().StringVar(&cfg.Policy, "policy", "", "path to validation policy (default: .harness/validation-policy.yaml)")
	root.PersistentFlags().StringVar(&cfg.RepoRoot, "repo-root", "", "repo root (default: current working directory)")
	root.PersistentFlags().IntVar(&cfg.Concurrency, "concurrency", 0, "max parallel use cases (0 = GOMAXPROCS)")
	root.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", 0, "wall-clock cap per sensor (0 = no cap)")

	root.AddCommand(newValidateCmd(ctx, cfg))
	root.AddCommand(newHealCmd(ctx, cfg))
	root.AddCommand(newVersionCmd())

	return root
}

// EffectiveConcurrency resolves cfg.Concurrency to the actual cap.
// 0 (default) maps to GOMAXPROCS; negative values clamp to 1.
func (c *Config) EffectiveConcurrency() int {
	if c.Concurrency <= 0 {
		return runtime.GOMAXPROCS(0)
	}
	return c.Concurrency
}
