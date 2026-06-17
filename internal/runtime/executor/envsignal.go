package executor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
)

// MissingEnvError reports ambient environment variables a step requires
// (via ${{ env.* }} refs, its env: map, or a composed primitive's
// declared env) that the merged host+env_file view does not provide.
// Pre-spawn: the step's process is never started, so no recipe ever sees
// a silent empty value.
type MissingEnvError struct {
	Step    int
	Names   []string
	EnvFile string // "" when no env_file is declared
}

func (e *MissingEnvError) Error() string {
	if e.Step > 0 {
		return fmt.Sprintf("executor: step %d: missing required env var(s): %s", e.Step, strings.Join(e.Names, ", "))
	}
	return "executor: missing required env var(s): " + strings.Join(e.Names, ", ")
}

// EnvFileInvalidError reports an unparseable manifest-declared env_file.
// Like MissingEnvError it is an environment problem, not an app crash —
// the crash-hint synthesizer must not dress it up as one.
type EnvFileInvalidError struct {
	Path  string
	Cause error
}

func (e *EnvFileInvalidError) Error() string {
	return "executor: env_file " + e.Path + ": " + e.Cause.Error()
}

func (e *EnvFileInvalidError) Unwrap() error { return e.Cause }

// missingEnvSignal synthesizes the typed pre-spawn signal for absent
// ambient env vars. Verdict inconclusive: the application is not proven
// broken, the environment is incomplete (mirrors provision-auth's
// auth-not-provisioned contract).
func missingEnvSignal(s sensor.Sensor, names []string, envFile string, now func() time.Time) signal.Signal {
	sources := "host environment"
	if envFile != "" {
		sources += " and " + envFile
	}
	return envProblemSignal(s, "missing-env",
		// comma-separated, no spaces: machine-parseable
		signal.Evidence{"missing": strings.Join(names, ","), "sources": sources},
		"Missing required env var(s): "+strings.Join(names, ", "),
		"The step needs ambient configuration the harness could not find in the "+sources+
			". Export the variable(s) before invoking the harness, or add them to the project's env_file"+
			" — no step process was spawned.",
		now)
}

// envFileInvalidSignal surfaces an unparseable manifest-declared env_file.
func envFileInvalidSignal(s sensor.Sensor, envFile, parseErr string, now func() time.Time) signal.Signal {
	return envProblemSignal(s, "env-file-invalid",
		signal.Evidence{"env_file": envFile, "error": parseErr},
		"Declared env_file could not be parsed: "+envFile,
		"The stack manifest binds an env_file the dotenv parser rejects. Fix the file's syntax"+
			" (or correct the manifest's env_file path) — no step was run.",
		now)
}

// setupUnavailableSignal is emitted when a setup node's command cannot be
// resolved/run. Verdict inconclusive: a missing setup step is an incomplete
// environment, not an application defect.
//
// TODO(#52 Phase 7+): wire into the executor when setup-node command-resolution
// detection lands. Added now as the typed-signal vocabulary (mirrors missingEnvSignal).
func setupUnavailableSignal(s sensor.Sensor, ref string, now func() time.Time) signal.Signal {
	return envProblemSignal(s, "setup-unavailable",
		signal.Evidence{"setup": ref},
		"Setup step unavailable: "+ref,
		"The setup command ("+ref+") could not be resolved or executed. Provide the missing script/target — no behavioral conclusion was drawn.",
		now)
}

func envProblemSignal(s sensor.Sensor, key string, ev signal.Evidence, summary, rationale string, now func() time.Time) signal.Signal {
	ev["observation_key"] = key
	return signal.Signal{
		SchemaVersion: observationSignalSchemaVersion,
		SensorID:      s.ID,
		UseCaseID:     s.UseCaseID,
		Angle:         s.Angle,
		EmittedAt:     now(),
		Verdict:       enums.VerdictInconclusive,
		Confidence:    1,
		Evidence:      ev,
		HealHint:      &signal.HealHint{Summary: summary, Rationale: rationale},
	}
}

// writeSignal best-effort persists one synthesized signal to signals.jsonl.
func writeSignal(sw *jsonlWriter, sig signal.Signal) {
	if b, err := json.Marshal(sig); err == nil {
		_ = sw.WriteLine(b)
	}
}

// missingRequiredEnv returns the names of required declared env vars that
// are unset or empty in the merged view, sorted.
func missingRequiredEnv(decl map[string]sensor.EnvSpec, view envView) []string {
	var missing []string
	for name, spec := range decl {
		if !spec.IsRequired() {
			continue
		}
		if v, ok := view.lookup(name); !ok || v == "" {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}
