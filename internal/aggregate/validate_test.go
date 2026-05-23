package aggregate

import (
	"strings"
	"testing"
)

// withFieldRemoved returns happyPathJSON with the given top-level JSON field
// removed (string-search based — sufficient for these flat test inputs).
func withFieldRemoved(t *testing.T, field string) string {
	t.Helper()
	prefix := `"` + field + `":`
	idx := strings.Index(happyPathJSON, prefix)
	if idx < 0 {
		t.Fatalf("field %q not found in happyPathJSON", field)
	}
	end := strings.Index(happyPathJSON[idx:], ",")
	if end < 0 {
		commaBefore := strings.LastIndex(happyPathJSON[:idx], ",")
		return happyPathJSON[:commaBefore] + happyPathJSON[idx+len(happyPathJSON[idx:]):]
	}
	return happyPathJSON[:idx] + happyPathJSON[idx+end+1:]
}

func TestParseRejectsMissingRequiredField(t *testing.T) {
	cases := []string{"sensor_id", "use_case_id", "angle", "started_at", "verdict", "confidence", "rollup"}
	for _, field := range cases {
		t.Run(field, func(t *testing.T) {
			bad := withFieldRemoved(t, field)
			if _, err := ParseAggregate(strings.NewReader(bad)); err == nil {
				t.Errorf("expected error when %q removed, got nil", field)
			}
		})
	}
}

func TestParseRejectsInvalidVerdict(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "maybe"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for invalid verdict, got nil")
	}
}

func TestParseRejectsConfidenceOutOfRange(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"confidence": 1.0`, `"confidence": 1.5`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for confidence > 1.0, got nil")
	}
}

func TestParseRejectsFailWithoutHealHint(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "fail"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for fail without heal_hint, got nil")
	}
}

func TestParseRejectsWarnWithoutHealHint(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"verdict": "pass"`, `"verdict": "warn"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for warn without heal_hint, got nil")
	}
}

func TestParseRejectsBadArithmetic(t *testing.T) {
	// Replace pass_count: 1 with pass_count: 2 so the sum no longer equals total_signals: 1.
	bad := strings.Replace(happyPathJSON, `"pass_count": 1`, `"pass_count": 2`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for pass+warn+fail+inconclusive != total_signals, got nil")
	}
	if !strings.Contains(err.Error(), "rollup") || !strings.Contains(err.Error(), "sum") {
		t.Errorf("error should mention 'rollup' and 'sum': %v", err)
	}
}

func TestParseRejectsPassWithHealHint(t *testing.T) {
	// Inject a heal_hint into a pass-verdict aggregate. We do this by
	// replacing the closing brace of the rollup block with rollup-close +
	// a sibling heal_hint key.
	bad := strings.Replace(
		happyPathJSON,
		`"inconclusive_count": 0
  }
}`,
		`"inconclusive_count": 0
  },
  "heal_hint": {"summary": "x", "rationale": "y"}
}`,
		1,
	)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for pass with heal_hint, got nil")
	}
	if !strings.Contains(err.Error(), "heal_hint") {
		t.Errorf("error should mention 'heal_hint': %v", err)
	}
}
