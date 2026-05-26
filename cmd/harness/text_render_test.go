package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

func TestRenderValidateText_Unicode(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	out := &bytes.Buffer{}
	r := buildSimpleRunResult(enums.VerdictPass, enums.VerdictPass)
	if err := renderValidateText(out, r); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out.String(), "✓") {
		t.Errorf("expected ✓ glyph, got: %s", out.String())
	}
}

func TestRenderValidateText_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	out := &bytes.Buffer{}
	r := buildSimpleRunResult(enums.VerdictFail, enums.VerdictFail)
	if err := renderValidateText(out, r); err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out.String(), "[FAIL]") {
		t.Errorf("expected [FAIL] fallback, got: %s", out.String())
	}
}

func buildSimpleRunResult(useCaseVerdict, sensorVerdict enums.Verdict) *RunResult {
	now := time.Now()
	return &RunResult{
		Command: "validate",
		Result: map[string]any{
			"verdicts": []any{
				map[string]any{
					"use_case_id": "create-order",
					"verdict":     useCaseVerdict,
					"confidence":  0.9,
					"sensors": []any{
						map[string]any{
							"sensor_id":  "create-order--build",
							"angle":      enums.AngleBuild,
							"verdict":    sensorVerdict,
							"confidence": 1.0,
							"started_at": now,
							"ended_at":   now.Add(100 * time.Millisecond),
							"rollup":     aggregate.RollupCounts{TotalSignals: 1, PassCount: 1},
						},
					},
				},
			},
		},
	}
}
