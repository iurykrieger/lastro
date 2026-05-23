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
