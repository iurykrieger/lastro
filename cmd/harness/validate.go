package main

import (
	"context"
	"errors"

	"github.com/spf13/cobra"
)

// ErrUnimplemented stays defined for stubs in other subcommands; kept
// here so callers still importing it continue to compile.
var ErrUnimplemented = errors.New("harness: subcommand not yet implemented")

// newValidateCmd returns the validate subcommand wired with real
// lifecycle + aggregator integration.
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

Exactly one of --use-case (repeatable) or --all must be supplied.

Exit codes:
  0 - all selected use cases passed
  1 - at least one use case failed
  2 - at least one use case was inconclusive and none failed
  64 - usage error (bad flags or missing input files)
  70 - internal error`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(useCaseIDs) > 0) {
				return &UsageError{Msg: "supply exactly one of --use-case or --all"}
			}
			return runValidate(ctx, cfg, useCaseIDs, all, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringSliceVar(&useCaseIDs, "use-case", nil, "use case id (repeatable; conflicts with --all)")
	cmd.Flags().BoolVar(&all, "all", false, "validate every use case (conflicts with --use-case)")
	return cmd
}
