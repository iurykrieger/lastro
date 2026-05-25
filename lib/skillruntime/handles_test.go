package skillruntime

import (
	"strings"
	"testing"
)

const (
	validULID1 = "01HMG12RX9N6Z8WJ3D6PNHVQXC"
	validULID2 = "01HMG12RXATAFM4N0F0X5Y4SGE"
)

func TestParseHandle_OK(t *testing.T) {
	sensorID, runID, err := ParseHandle(validULID1 + ":" + validULID2)
	if err != nil {
		t.Fatalf("ParseHandle: %v", err)
	}
	if sensorID != validULID1 {
		t.Errorf("sensorID = %q, want %q", sensorID, validULID1)
	}
	if runID != validULID2 {
		t.Errorf("runID = %q, want %q", runID, validULID2)
	}
}

func TestParseHandle_NoColon(t *testing.T) {
	_, _, err := ParseHandle(validULID1)
	if err == nil || !strings.Contains(err.Error(), "missing ':'") {
		t.Errorf("expected missing-colon error, got %v", err)
	}
}

func TestParseHandle_WrongLength(t *testing.T) {
	_, _, err := ParseHandle("short:" + validULID2)
	if err == nil {
		t.Errorf("expected error on short sensor id")
	}
}

func TestParseHandle_NonULIDChars(t *testing.T) {
	bad := strings.Repeat("?", 26)
	_, _, err := ParseHandle(bad + ":" + validULID2)
	if err == nil {
		t.Errorf("expected error on non-ULID chars")
	}
}

func TestFormatHandle(t *testing.T) {
	got := FormatHandle(validULID1, validULID2)
	want := validULID1 + ":" + validULID2
	if got != want {
		t.Errorf("FormatHandle = %q, want %q", got, want)
	}
}
