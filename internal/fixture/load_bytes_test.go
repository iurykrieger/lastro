package fixture

import (
	"os"
	"testing"
)

func TestLoadFixtureBytes_RoundTripsExample(t *testing.T) {
	b, err := os.ReadFile("../../schemas/examples/fixture/input.yaml")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	fx, err := LoadFixtureBytes(b)
	if err != nil {
		t.Fatalf("LoadFixtureBytes: %v", err)
	}
	if fx.ID == "" {
		t.Fatal("ID is empty after LoadFixtureBytes")
	}
}
