package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
	aggregator "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
)

// updateGolden re-writes the testdata file when -update is passed.
var updateGolden = os.Getenv("UPDATE_GOLDEN") == "1"

func TestJSONRender_ValidatePassGolden(t *testing.T) {
	startedAt := time.Date(2026, 5, 25, 12, 34, 56, 789_000_000, time.UTC)
	endedAt := startedAt.Add(1223 * time.Millisecond)

	wrapper := &RunResult{
		CLISchemaVersion: CLISchemaVersion,
		RunID:            "01J0000000000000000000000",
		Command:          "validate",
		Args:             []string{"--use-case", "create-order"},
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		DurationMs:       1223,
		HarnessVersion:   "0.1.0",
		Result: map[string]any{
			"verdicts": []map[string]any{{
				"use_case_id":          "create-order",
				"archetype":            enums.ArchetypeHTTPAPI,
				"verdict":              enums.VerdictPass,
				"confidence":           0.94,
				"obligatory_satisfied": true,
				"evaluated_angles":     []enums.ValidationAngle{enums.AngleBuild},
				"failing_angles":       []enums.ValidationAngle{},
				"warning_angles":       []enums.ValidationAngle{},
				"heal_hints":           []aggregator.AngleHint{},
				"sensors": []map[string]any{{
					"sensor_id":  "create-order--build",
					"angle":      enums.AngleBuild,
					"verdict":    enums.VerdictPass,
					"confidence": 1.0,
					"started_at": startedAt,
					"ended_at":   endedAt,
					"rollup":     aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
				}},
			}},
			"summary": map[string]any{
				"total_use_cases":    1,
				"pass_count":         1,
				"fail_count":         0,
				"inconclusive_count": 0,
			},
		},
	}

	var buf bytes.Buffer
	if err := renderJSON(&buf, wrapper); err != nil {
		t.Fatalf("renderJSON: %v", err)
	}

	goldenPath := filepath.Join("testdata", "golden", "validate_pass.json")
	if updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(goldenPath, buf.Bytes(), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("updated %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with UPDATE_GOLDEN=1)", err)
	}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("JSON output does not match golden.\nGot:\n%s\n\nWant:\n%s", buf.String(), string(want))
	}

	// Sanity: the JSON parses back into a generic map.
	var roundTrip map[string]any
	if err := json.Unmarshal(buf.Bytes(), &roundTrip); err != nil {
		t.Fatalf("round-trip decode failed: %v", err)
	}
	if roundTrip["cli_schema_version"] != CLISchemaVersion {
		t.Errorf("cli_schema_version round-trip = %v, want %s", roundTrip["cli_schema_version"], CLISchemaVersion)
	}
}
