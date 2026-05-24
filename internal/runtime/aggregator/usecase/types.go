// Package aggregator computes the use-case verdict (plan §6.3) from the
// AggregateSignals emitted by every sensor that validated one use case.
// See docs/superpowers/specs/2026-05-24-b1-composed-runtime-design.md §6.
package aggregator

import (
	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/enums"
)

// AngleHint pairs a non-pass verdict with its angle and heal hint.
// Used to surface warn and fail signals from one use case in one slice,
// preserving locus precision (no consolidation).
type AngleHint struct {
	Angle   enums.ValidationAngle
	Verdict enums.Verdict // always either warn or fail
	Hint    aggregate.HealHint
}

// UseCaseVerdict is the terminal output of aggregator.UseCase.
type UseCaseVerdict struct {
	UseCaseID           string
	Archetype           enums.Archetype
	Verdict             enums.Verdict // pass | fail | inconclusive (warn lives at signal level only)
	Confidence          float64       // weighted average, [0.0, 1.0]
	ObligatorySatisfied bool          // true iff every obligatory effective verdict in {pass, warn}
	EvaluatedAngles     []enums.ValidationAngle
	FailingAngles       []enums.ValidationAngle // post-floor verdict == fail (canonical order)
	WarningAngles       []enums.ValidationAngle // post-floor verdict == warn (canonical order)
	HealHints           []AngleHint             // one per fail + warn, canonical order
}
