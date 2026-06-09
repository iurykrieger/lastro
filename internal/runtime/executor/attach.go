package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/runtime/servicemgr"
	"github.com/iurykrieger/lastro/internal/runtime/sigstream"
	"github.com/iurykrieger/lastro/internal/sensor"
	"github.com/iurykrieger/lastro/internal/signal"
)

// attachArgs is the input to one attach step: a consumer sensor watching a
// running service's signal stream.
type attachArgs struct {
	Consumer      sensor.Sensor
	Attachment    servicemgr.Attachment
	ExpectedKeys  []string
	ObserveWindow time.Duration
	Now           func() time.Time
	SignalsW      *jsonlWriter // optional; nil in unit tests
	Stop          <-chan struct{}
}

// attachResult mirrors topStepResult's fields the caller needs.
type attachResult struct {
	Signals         []signal.Signal
	ObservationKeys []string
	TermReason      enums.TerminationReason
	StepErr         error
}

// execAttachStep tails the service's signal stream, applies the consumer's
// signal_matches to each service signal's matched_line, emits the consumer's
// own signals, and terminates when every expected key has been observed
// (completed) or the observation window elapses (timeout).
func execAttachStep(ctx context.Context, a attachArgs) attachResult {
	now := a.Now
	if now == nil {
		now = time.Now
	}

	type cm struct {
		key     string
		re      *regexp.Regexp
		verdict enums.Verdict
		conf    float64
		hint    *signal.HealHint
	}
	matchers := make([]cm, 0, len(a.Consumer.SignalMatches))
	for _, sm := range a.Consumer.SignalMatches {
		re, err := regexp.Compile(sm.Pattern)
		if err != nil {
			return attachResult{TermReason: enums.TerminationError, StepErr: fmt.Errorf("attach: bad pattern %q: %w", sm.Key, err)}
		}
		verdict := sm.Verdict
		if verdict == "" {
			verdict = enums.VerdictPass
		}
		conf := 1.0
		if sm.Confidence != nil {
			conf = *sm.Confidence
		}
		var hint *signal.HealHint
		if sm.HealHint != nil {
			hint = &signal.HealHint{Summary: sm.HealHint.Summary, Rationale: sm.HealHint.Rationale}
		}
		matchers = append(matchers, cm{key: sm.Key, re: re, verdict: verdict, conf: conf, hint: hint})
	}

	remaining := map[string]struct{}{}
	for _, k := range a.ExpectedKeys {
		remaining[k] = struct{}{}
	}

	var res attachResult
	window := a.ObserveWindow
	if window <= 0 {
		window = defaultObserveWindow
	}
	wctx, cancel := context.WithTimeout(ctx, window)
	defer cancel()

	// poll 0 → sigstream default (50ms).
	err := sigstream.Follow(wctx, a.Attachment.SignalsPath, 0, a.Stop, func(d sigstream.Decoded) bool {
		for _, m := range matchers {
			if !m.re.MatchString(d.MatchedLine) {
				continue
			}
			sig := signal.Signal{
				SchemaVersion: observationSignalSchemaVersion,
				SensorID:      a.Consumer.ID,
				UseCaseID:     a.Consumer.UseCaseID,
				Angle:         a.Consumer.Angle,
				EmittedAt:     now(),
				Verdict:       m.verdict,
				Confidence:    m.conf,
				Evidence:      signal.Evidence{"observation_key": m.key, "matched_line": d.MatchedLine},
				HealHint:      m.hint,
			}
			res.Signals = append(res.Signals, sig)
			res.ObservationKeys = append(res.ObservationKeys, m.key)
			if a.SignalsW != nil {
				if b, err := json.Marshal(sig); err == nil {
					_ = a.SignalsW.WriteLine(b)
				}
			}
			// remaining holds only ExpectedKeys; deleting a non-expected key is a no-op.
			delete(remaining, m.key)
		}
		return len(remaining) == 0 // satisfied-on-completeness
	})

	switch {
	case err == nil:
		res.TermReason = enums.TerminationCompleted
	case errors.Is(err, context.DeadlineExceeded):
		// DeadlineExceeded has two sources: the observe-window timeout (wctx,
		// derived here) or an outer ctx deadline imposed by the caller. Only
		// the former is a normal completeness rollup — the window elapsed
		// without all expected keys and the rollup turns the gap into the
		// verdict. An outer deadline is an externally imposed timeout and must
		// surface as such, not be masked as a clean completion.
		if ctx.Err() == nil {
			res.TermReason = enums.TerminationCompleted
		} else {
			res.TermReason = enums.TerminationTimeout
			res.StepErr = ctx.Err()
		}
	case errors.Is(err, context.Canceled):
		res.TermReason = enums.TerminationStopped
	default:
		res.TermReason = enums.TerminationError
		res.StepErr = err
	}
	return res
}

// defaultObserveWindow bounds how long an attaching observational consumer
// watches a service stream before rolling up on completeness.
const defaultObserveWindow = 30 * time.Second
