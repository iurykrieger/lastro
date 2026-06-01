// Command create-sensors-script is invoked by the /create-sensors slash
// command. Reads an LLM-emitted sensor YAML from --file and hands it to
// internal/sensor.Persist. On success: exit 0, nothing on stdout. On
// validation failure: exit 2 with a JSON persisterror.Error on stdout.
// On script-level failure (bad args, missing file): exit 1 with a
// message on stderr.
//
// Composition validation: if .harness/sensors/core/ exists, loads the core
// primitives into a store and calls sensor.ValidateComposition on the
// consumer sensor. If core dir is absent, warns on stderr and skips.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func main() {
	file := flag.String("file", "", "Path to the LLM-emitted sensor YAML")
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

	// Load and validate the consumer sensor before persisting.
	s, err := sensor.LoadSensorBytes(content)
	if err != nil {
		pe := &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			Message:    err.Error(),
		}
		_ = json.NewEncoder(os.Stdout).Encode(pe)
		os.Exit(2)
	}

	// Composition validation: validate uses-steps against core primitives.
	coreDir := filepath.Join(*harnessDir, "sensors", "core")
	if info, statErr := os.Stat(coreDir); statErr == nil && info.IsDir() {
		coreStore, loadErr := sensor.LoadDirectory(coreDir)
		if loadErr != nil {
			fmt.Fprintln(os.Stderr, "composition validation: load core sensors:", loadErr)
			os.Exit(1)
		}
		store, storeErr := sensor.NewStore(append(coreStore.All(), s)...)
		if storeErr != nil {
			fmt.Fprintln(os.Stderr, "composition validation: build store:", storeErr)
			os.Exit(1)
		}
		if compErr := sensor.ValidateComposition(s, store); compErr != nil {
			pe := &persisterror.Error{
				Kind:       persisterror.SchemaViolation,
				EntityType: "sensor",
				EntityID:   s.ID,
				Message:    "composition validation: " + compErr.Error(),
			}
			_ = json.NewEncoder(os.Stdout).Encode(pe)
			os.Exit(2)
		}
	} else {
		fmt.Fprintln(os.Stderr, "warning: .harness/sensors/core/ not found; skipping composition validation")
	}

	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(os.Stdout).Encode(pe)
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
