package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

// ErrHealGated is returned by heal until B3 (internal/runtime/healloop)
// lands. main() maps it to exit code 70.
var ErrHealGated = errors.New("harness heal is gated on B3 (internal/runtime/healloop). Track docs/harness-framework/B3-heal-loop.md")

func newHealCmd(ctx context.Context, cfg *Config) *cobra.Command {
	var (
		sensorID    string
		runID       string
		allFailing  bool
		maxItersInt int
	)
	cmd := &cobra.Command{
		Use:   "heal",
		Short: "Apply LLM-proposed fixes to failing sensors (gated on B3)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ErrHealGated
		},
	}
	cmd.Flags().StringVar(&sensorID, "sensor", "", "heal the latest failing run for this sensor")
	cmd.Flags().StringVar(&runID, "run-id", "", "pin a specific run id (requires --sensor)")
	cmd.Flags().BoolVar(&allFailing, "all-failing", false, "scan .harness/runtime/ and heal every failing aggregate")
	cmd.Flags().IntVar(&maxItersInt, "max-iterations", 0, "per-sensor iteration cap (0 = use policy default)")
	// Mark as unused for the stub phase to silence linters; real wiring lands with B6.3.
	_ = errors.New
	return cmd
}
