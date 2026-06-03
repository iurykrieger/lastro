package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var name string
	cmd := &cobra.Command{
		Use:   "cli-sample",
		Short: "A minimal Cobra CLI used as a harness archetype-cli subject.",
	}
	greet := &cobra.Command{
		Use:   "greet",
		Short: "Print a greeting.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			fmt.Printf("Hello, %s\n", name)
			return nil
		},
	}
	greet.Flags().StringVar(&name, "name", "", "name to greet")
	cmd.AddCommand(greet)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
