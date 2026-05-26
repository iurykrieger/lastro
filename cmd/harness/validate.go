package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

// ErrUnimplemented is returned from subcommand stubs that have a
// real implementation queued in a later phase. main() maps it to
// exit code 70 (EX_SOFTWARE).
var ErrUnimplemented = errors.New("harness: subcommand not yet implemented")

// newValidateCmd returns the validate subcommand. Phase 2 replaces
// this stub with the real implementation; for now it parses flags
// and returns ErrUnimplemented so the Cobra wiring compiles.
func newValidateCmd(ctx context.Context, cfg *Config) *cobra.Command {
	var (
		useCaseIDs []string
		all        bool
	)
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate one or more use cases",
		Long: `validate runs the sensors associated with one or more use cases
and prints an aggregated verdict per use case.

Exactly one of --use-case (repeatable) or --all must be supplied.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(useCaseIDs) > 0) {
				return &UsageError{Msg: "supply exactly one of --use-case or --all"}
			}
			return ErrUnimplemented
		},
	}
	cmd.Flags().StringSliceVar(&useCaseIDs, "use-case", nil, "use case id (repeatable; conflicts with --all)")
	cmd.Flags().BoolVar(&all, "all", false, "validate every use case (conflicts with --use-case)")
	return cmd
}
