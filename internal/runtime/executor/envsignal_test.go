package executor

import (
	"testing"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
)

func TestSetupUnavailableSignal(t *testing.T) {
	s := sensor.Sensor{ID: "core-migrate", Angle: enums.AngleEnvironment}
	sig := setupUnavailableSignal(s, "db:migrate", func() time.Time { return time.Unix(0, 0).UTC() })
	if sig.Verdict != enums.VerdictInconclusive {
		t.Fatalf("verdict = %q", sig.Verdict)
	}
	if k, _ := sig.Evidence["observation_key"].(string); k != "setup-unavailable" {
		t.Fatalf("observation_key = %v", sig.Evidence["observation_key"])
	}
}
