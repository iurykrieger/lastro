// cmd/lastro/environment.go
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/environment"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

func detectEnvironment(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-environment", flag.ContinueOnError)
	fs.SetOutput(stderr)
	mode := fs.String("mode", "", "facts | persist")
	repo := fs.String("repo", ".", "Repo root to parse (facts mode)")
	file := fs.String("file", "", "Path to the LLM-emitted environment-model YAML (persist mode)")
	facts := fs.String("facts", "", "Path to the raw-facts YAML from facts mode (persist mode)")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}

	switch *mode {
	case "facts":
		f, err := environment.Parse(*repo)
		if err != nil {
			fmt.Fprintln(stderr, "parse:", err)
			return 1
		}
		b, err := yaml.Marshal(f)
		if err != nil {
			fmt.Fprintln(stderr, "marshal facts:", err)
			return 1
		}
		_, _ = stdout.Write(b)
		return 0
	case "persist":
		if *file == "" || *facts == "" {
			fmt.Fprintln(stderr, "persist mode requires --file and --facts")
			return 1
		}
		modelContent, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(stderr, "read model:", err)
			return 1
		}
		factsContent, err := os.ReadFile(*facts)
		if err != nil {
			fmt.Fprintln(stderr, "read facts:", err)
			return 1
		}
		if err := environment.Persist(modelContent, factsContent, *harnessDir); err != nil {
			var pe *persisterror.Error
			if errors.As(err, &pe) {
				_ = json.NewEncoder(stdout).Encode(pe)
				return 2
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(stderr, "detect-environment: invalid --mode %q (want facts or persist)\n", *mode)
		return 1
	}
}
