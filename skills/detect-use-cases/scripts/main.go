// Command detect-use-cases-script is invoked by the /detect-use-cases
// slash command. Reads an LLM-emitted YAML from --file and hands it to
// internal/fixture.Persist or internal/usecase.Persist depending on
// --type. Same exit-code contract as detect-stack: 0 success, 2 JSON
// validation failure, 1 script-level error.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func main() {
	entityType := flag.String("type", "", "Entity type: fixture | use-case")
	file := flag.String("file", "", "Path to the LLM-emitted YAML")
	harnessDir := flag.String("harness-dir", ".harness", "Target .harness directory")
	flag.Parse()

	if *file == "" {
		fmt.Fprintln(os.Stderr, "missing required --file")
		os.Exit(1)
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read input:", err)
		os.Exit(1)
	}

	var persistErr error
	switch *entityType {
	case "fixture":
		persistErr = fixture.Persist(content, *harnessDir)
	case "use-case":
		persistErr = usecase.Persist(content, *harnessDir)
	default:
		fmt.Fprintf(os.Stderr, "invalid --type %q (want fixture or use-case)\n", *entityType)
		os.Exit(1)
	}

	if persistErr != nil {
		var pe *persisterror.Error
		if errors.As(persistErr, &pe) {
			_ = json.NewEncoder(os.Stdout).Encode(pe)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, persistErr)
		os.Exit(1)
	}
}
