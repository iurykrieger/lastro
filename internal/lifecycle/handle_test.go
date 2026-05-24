package lifecycle

import (
	"encoding/json"
	"testing"
	"time"
)

func TestHandle_JSONRoundTrip(t *testing.T) {
	h := Handle{
		SensorID:             "fake-sensor",
		RunID:                "01JZQ9G7M0H3FX8N1QPYAS78MV",
		RunDir:               "/abs/.harness/runtime/fake-sensor/01JZQ9G7M0H3FX8N1QPYAS78MV",
		PID:                  42,
		PGID:                 42,
		StartedAt:            time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		ExpectedObservations: []string{"order-received"},
		HarnessPID:           7,
		HarnessVersion:       "0.1.0",
		GOOS:                 "linux",
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Handle
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SensorID != h.SensorID || got.RunID != h.RunID || got.PID != h.PID || !got.StartedAt.Equal(h.StartedAt) {
		t.Errorf("round-trip mismatch:\ngot:  %+v\nwant: %+v", got, h)
	}
	if len(got.ExpectedObservations) != 1 || got.ExpectedObservations[0] != "order-received" {
		t.Errorf("ExpectedObservations lost: %v", got.ExpectedObservations)
	}
}
