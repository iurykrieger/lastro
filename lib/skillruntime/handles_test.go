package skillruntime

import (
	"strings"
	"testing"
)

const (
	validSensorID = "my-sensor-id"
	validULID     = "01HMG12RX9N6Z8WJ3D6PNHVQXC"
)

func TestParseHandle_OK(t *testing.T) {
	sensorID, runID, err := ParseHandle(validSensorID + ":" + validULID)
	if err != nil {
		t.Fatalf("ParseHandle: %v", err)
	}
	if sensorID != validSensorID {
		t.Errorf("sensorID = %q, want %q", sensorID, validSensorID)
	}
	if runID != validULID {
		t.Errorf("runID = %q, want %q", runID, validULID)
	}
}

func TestParseHandle_NoColon(t *testing.T) {
	_, _, err := ParseHandle(validSensorID)
	if err == nil || !strings.Contains(err.Error(), "missing ':'") {
		t.Errorf("expected missing-colon error, got %v", err)
	}
}

func TestParseHandle_SensorIDStartsWithDigit(t *testing.T) {
	_, _, err := ParseHandle("1bad:" + validULID)
	if err == nil {
		t.Errorf("expected error on sensor id starting with digit")
	}
}

func TestParseHandle_SensorIDUppercase(t *testing.T) {
	_, _, err := ParseHandle("Bad:" + validULID)
	if err == nil {
		t.Errorf("expected error on uppercase sensor id")
	}
}

func TestParseHandle_SensorIDEmpty(t *testing.T) {
	_, _, err := ParseHandle(":" + validULID)
	if err == nil {
		t.Errorf("expected error on empty sensor id")
	}
}

func TestParseHandle_RunIDWrongLength(t *testing.T) {
	_, _, err := ParseHandle(validSensorID + ":short")
	if err == nil {
		t.Errorf("expected error on short run id")
	}
}

func TestParseHandle_RunIDNonULIDChars(t *testing.T) {
	bad := strings.Repeat("?", 26)
	_, _, err := ParseHandle(validSensorID + ":" + bad)
	if err == nil {
		t.Errorf("expected error on non-ULID chars in run id")
	}
}

func TestFormatHandle(t *testing.T) {
	got := FormatHandle(validSensorID, validULID)
	want := validSensorID + ":" + validULID
	if got != want {
		t.Errorf("FormatHandle = %q, want %q", got, want)
	}
}
