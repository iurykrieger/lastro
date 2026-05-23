package signal

import (
	"strings"
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
)

// validSignal returns a fully-populated Signal that passes Validate.
// Tests modify a single field to exercise one validator branch at a time.
func validSignal() Signal {
	return Signal{
		SchemaVersion: "1.0.0",
		SensorID:      "build-sensor",
		UseCaseID:     "create-order",
		Angle:         enums.AngleBuild,
		EmittedAt:     time.Date(2026, 5, 23, 10, 15, 0, 0, time.UTC),
		Verdict:       enums.VerdictPass,
		Confidence:    1.0,
		Evidence:      Evidence{"expected": "tsc exits 0", "actual": "tsc exited 0"},
	}
}

func TestValidate_HappyPath(t *testing.T) {
	if err := Validate(validSignal()); err != nil {
		t.Fatalf("Validate on valid signal: %v", err)
	}
}

func TestValidate_MissingVerdict(t *testing.T) {
	sig := validSignal()
	sig.Verdict = ""
	err := Validate(sig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "verdict") {
		t.Errorf("expected error to mention 'verdict', got: %v", err)
	}
}

func TestValidate_FailWithoutHealHint(t *testing.T) {
	sig := validSignal()
	sig.Verdict = enums.VerdictFail
	sig.HealHint = nil
	err := Validate(sig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "heal_hint") {
		t.Errorf("expected error to mention 'heal_hint', got: %v", err)
	}
}

func TestValidate_ConfidenceOutOfRange(t *testing.T) {
	sig := validSignal()
	sig.Confidence = 1.5
	err := Validate(sig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "confidence") {
		t.Errorf("expected error to mention 'confidence', got: %v", err)
	}
}

func TestValidate_UnknownAngle(t *testing.T) {
	sig := validSignal()
	sig.Angle = "not-a-real-angle"
	err := Validate(sig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "angle") {
		t.Errorf("expected error to mention 'angle', got: %v", err)
	}
}

func TestValidate_BadIDPattern(t *testing.T) {
	sig := validSignal()
	sig.SensorID = "Invalid_Id"
	err := Validate(sig)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sensor_id") && !strings.Contains(err.Error(), "pattern") {
		t.Errorf("expected error to mention 'sensor_id' or 'pattern', got: %v", err)
	}
}

func TestValidate_FailWithHealHint_OK(t *testing.T) {
	sig := validSignal()
	sig.Verdict = enums.VerdictFail
	sig.HealHint = &HealHint{
		Summary:   "test failed",
		Rationale: "the thing did not happen",
	}
	if err := Validate(sig); err != nil {
		t.Fatalf("Validate on valid failing signal: %v", err)
	}
}
