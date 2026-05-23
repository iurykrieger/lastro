package aggregate

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

const happyPathJSON = `{
  "schema_version": "1.0.0",
  "type": "aggregate",
  "sensor_id": "sensor-x",
  "use_case_id": "uc-x",
  "angle": "unit-test",
  "started_at": "2026-05-23T14:00:00Z",
  "ended_at":   "2026-05-23T14:00:42Z",
  "termination_reason": "completed",
  "verdict": "pass",
  "confidence": 1.0,
  "rollup": {
    "total_signals": 1,
    "pass_count": 1,
    "warn_count": 0,
    "fail_count": 0,
    "inconclusive_count": 0
  }
}`

func TestParseAggregateHappyPath(t *testing.T) {
	got, err := ParseAggregate(strings.NewReader(happyPathJSON))
	if err != nil {
		t.Fatalf("ParseAggregate: %v", err)
	}
	if got.Type != TypeAggregate {
		t.Errorf("Type = %q, want %q", got.Type, TypeAggregate)
	}
	if got.Verdict != enums.VerdictPass {
		t.Errorf("Verdict = %q, want %q", got.Verdict, enums.VerdictPass)
	}
	if got.Rollup.TotalSignals != 1 || got.Rollup.PassCount != 1 {
		t.Errorf("Rollup = %+v, want total=1 pass=1", got.Rollup)
	}
	if got.StartedAt.UTC() != time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC) {
		t.Errorf("StartedAt = %v, want 2026-05-23T14:00:00Z", got.StartedAt)
	}
}

func TestParseAggregateRoundTrip(t *testing.T) {
	parsed, err := ParseAggregate(strings.NewReader(happyPathJSON))
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(parsed); err != nil {
		t.Fatalf("encode: %v", err)
	}

	reparsed, err := ParseAggregate(&buf)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}

	if !reflect.DeepEqual(parsed, reparsed) {
		t.Errorf("round-trip mismatch:\n  first:  %+v\n  second: %+v", parsed, reparsed)
	}
}

func TestParseAggregateRejectsWrongType(t *testing.T) {
	bad := strings.Replace(happyPathJSON, `"type": "aggregate"`, `"type": "signal"`, 1)
	_, err := ParseAggregate(strings.NewReader(bad))
	if err == nil {
		t.Fatal("expected error for wrong type discriminator, got nil")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention 'type': %v", err)
	}
}

func TestParseAggregateRejectsMalformedJSON(t *testing.T) {
	_, err := ParseAggregate(strings.NewReader(`{not json}`))
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode JSON") {
		t.Errorf("error should mention 'decode JSON': %v", err)
	}
}

func TestParseAggregateRejectsEmptyInput(t *testing.T) {
	_, err := ParseAggregate(strings.NewReader(""))
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
}
