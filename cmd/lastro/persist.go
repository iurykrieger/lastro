package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/stack"
	"github.com/iurykrieger/lastro/internal/usecase"
)

func persistDetectStack(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-stack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted stack-manifest YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	if err := stack.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func persistDetectUseCases(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("detect-use-cases", flag.ContinueOnError)
	fs.SetOutput(stderr)
	entityType := fs.String("type", "", "Entity type: fixture | use-case")
	file := fs.String("file", "", "Path to the LLM-emitted YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	var persistErr error
	switch *entityType {
	case "fixture":
		persistErr = fixture.Persist(content, *harnessDir)
	case "use-case":
		persistErr = usecase.Persist(content, *harnessDir)
	default:
		fmt.Fprintf(stderr, "invalid --type %q (want fixture or use-case)\n", *entityType)
		return 1
	}
	if persistErr != nil {
		var pe *persisterror.Error
		if errors.As(persistErr, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, persistErr)
		return 1
	}
	return 0
}

func persistCreateSensors(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-sensors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted sensor YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	s, err := sensor.LoadSensorBytes(content)
	if err != nil {
		pe := &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", Message: err.Error()}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	coreDir := filepath.Join(*harnessDir, "sensors", "core")
	if info, statErr := os.Stat(coreDir); statErr == nil && info.IsDir() {
		coreStore, loadErr := sensor.LoadDirectory(coreDir)
		if loadErr != nil {
			fmt.Fprintln(stderr, "composition validation: load core sensors:", loadErr)
			return 1
		}
		store, storeErr := sensor.NewStore(append(coreStore.All(), s)...)
		if storeErr != nil {
			fmt.Fprintln(stderr, "composition validation: build store:", storeErr)
			return 1
		}
		if compErr := sensor.ValidateComposition(s, store); compErr != nil {
			pe := &persisterror.Error{
				Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
				Message: "composition validation: " + compErr.Error(),
			}
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		if wkErr := sensor.ValidateWithKeys(s, store); wkErr != nil {
			pe := &persisterror.Error{
				Kind: persisterror.UnknownWithKey, EntityType: "sensor", EntityID: s.ID,
				Message: wkErr.Error(),
			}
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
	} else {
		fmt.Fprintln(stderr, "warning: .harness/sensors/core/ not found; skipping composition validation")
	}
	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func persistCreateCoreSensors(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("create-core-sensors", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Path to the LLM-emitted sensor YAML")
	harnessDir := fs.String("harness-dir", ".harness", "Target .harness directory")
	if err := fs.Parse(args[1:]); err != nil {
		return 1
	}
	if *file == "" {
		fmt.Fprintln(stderr, "missing required --file")
		return 1
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintln(stderr, "read input:", err)
		return 1
	}
	s, err := sensor.LoadSensorBytes(content)
	if err != nil {
		pe := &persisterror.Error{Kind: persisterror.SchemaViolation, EntityType: "sensor", Message: err.Error()}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if s.Scope != enums.ScopeCore {
		pe := &persisterror.Error{
			Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("scope must be %q for a core sensor, got %q", enums.ScopeCore, s.Scope),
		}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if s.UseCaseID != "" {
		pe := &persisterror.Error{
			Kind: persisterror.SchemaViolation, EntityType: "sensor", EntityID: s.ID,
			Message: fmt.Sprintf("use_case_id must be empty for a core sensor, got %q", s.UseCaseID),
		}
		_ = json.NewEncoder(stdout).Encode(pe)
		return 2
	}
	if err := sensor.Persist(content, *harnessDir); err != nil {
		var pe *persisterror.Error
		if errors.As(err, &pe) {
			_ = json.NewEncoder(stdout).Encode(pe)
			return 2
		}
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
