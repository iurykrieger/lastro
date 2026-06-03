package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd returns the `harness version` subcommand which prints
// the linker-injected HarnessVersion string. Useful for CI tooling that
// pins minimum versions.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the harness version and exit",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), HarnessVersion)
			return nil
		},
	}
}
