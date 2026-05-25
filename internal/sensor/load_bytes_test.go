package sensor

import (
	"os"
	"testing"
)

func TestLoadSensorBytes_RoundTripsExample(t *testing.T) {
	b, err := os.ReadFile("../../schemas/examples/sensor/assertion-computational-single.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s, err := LoadSensorBytes(b)
	if err != nil {
		t.Fatalf("LoadSensorBytes: %v", err)
	}
	if s.ID == "" {
		t.Fatal("ID is empty after LoadSensorBytes")
	}
}
