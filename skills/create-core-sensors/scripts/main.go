// Command create-core-sensors-script is invoked by the /create-core-sensors
// slash command. Reads an LLM-emitted sensor YAML from --file and hands it
// to internal/sensor.Persist. On success: exit 0, nothing on stdout. On
// validation failure: exit 2 with a JSON persisterror.Error on stdout.
// On script-level failure (bad args, missing file): exit 1 with a
// message on stderr.
//
// Additional check beyond sensor.Persist: the sensor must have scope: core
// and must not have a use_case_id set.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/iurykrieger/lastro/internal/enums"
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

	// Pre-check: load the sensor and assert scope: core, no use_case_id.
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
	if s.Scope != enums.ScopeCore {
		pe := &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("scope must be %q for a core sensor, got %q", enums.ScopeCore, s.Scope),
		}
		_ = json.NewEncoder(os.Stdout).Encode(pe)
		os.Exit(2)
	}
	if s.UseCaseID != "" {
		pe := &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("use_case_id must be empty for a core sensor, got %q", s.UseCaseID),
		}
		_ = json.NewEncoder(os.Stdout).Encode(pe)
		os.Exit(2)
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
