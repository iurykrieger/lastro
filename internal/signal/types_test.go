package signal

import (
	"testing"
)

func TestEvidence_Expected(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		e := Evidence{"expected": "ok"}
		v, ok := e.Expected()
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if v != "ok" {
			t.Errorf("expected value %q, got %v", "ok", v)
		}
	})
	t.Run("absent", func(t *testing.T) {
		e := Evidence{}
		v, ok := e.Expected()
		if ok {
			t.Errorf("expected ok=false, got true with value %v", v)
		}
	})
}

func TestEvidence_Actual(t *testing.T) {
	t.Run("present", func(t *testing.T) {
		e := Evidence{"actual": 42}
		v, ok := e.Actual()
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if v != 42 {
			t.Errorf("expected value 42, got %v", v)
		}
	})
	t.Run("absent", func(t *testing.T) {
		e := Evidence{}
		v, ok := e.Actual()
		if ok {
			t.Errorf("expected ok=false, got true with value %v", v)
		}
	})
}

func TestEvidence_FixtureID(t *testing.T) {
	t.Run("present_string", func(t *testing.T) {
		e := Evidence{"fixture_id": "order-input"}
		v, ok := e.FixtureID()
		if !ok {
			t.Fatal("expected ok=true, got false")
		}
		if v != "order-input" {
			t.Errorf("expected value %q, got %q", "order-input", v)
		}
	})
	t.Run("present_wrong_type", func(t *testing.T) {
		e := Evidence{"fixture_id": 123}
		v, ok := e.FixtureID()
		if ok {
			t.Errorf("expected ok=false for non-string value, got true with value %q", v)
		}
	})
	t.Run("absent", func(t *testing.T) {
		e := Evidence{}
		v, ok := e.FixtureID()
		if ok {
			t.Errorf("expected ok=false, got true with value %q", v)
		}
	})
}
