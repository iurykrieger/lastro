package validator

import (
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
)

// Report is the structured artifact written by ValidateAll to
// <target>/.harness/reports/<run-id>/report.json.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Target        string          `json:"target"`
	StartedAt     time.Time       `json:"started_at"`
	EndedAt       time.Time       `json:"ended_at"`
	UseCases      []UseCaseResult `json:"use_cases"`
	Summary       Summary         `json:"summary"`
}

// UseCaseResult is one entry in Report.UseCases. Verdict mirrors the
// persistedVerdict envelope from the /validate-use-case skill.
type UseCaseResult struct {
	UseCaseID  string              `json:"use_case_id"`
	Verdict    string              `json:"verdict"` // pass | fail | inconclusive
	SensorRuns []SensorRunSummary  `json:"sensor_runs"`
	HealHint   *aggregate.HealHint `json:"heal_hint,omitempty"`
	Stdout     string              `json:"-"` // raw JSONL, retained for test debugging
}

// SensorRunSummary mirrors persistedVerdict.sensor_runs entries.
type SensorRunSummary struct {
	SensorID string `json:"sensor_id"`
	Verdict  string `json:"verdict"`
}

// Summary aggregates use-case verdicts in one Report.
type Summary struct {
	Total        int `json:"total"`
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Inconclusive int `json:"inconclusive"`
}

// AllPassed reports whether every use case in the report had verdict=pass.
// An empty report (Total==0) is NOT considered passing — that means no
// use cases were detected, which is itself a regression.
func (r *Report) AllPassed() bool {
	return r.Summary.Total > 0 && r.Summary.Passed == r.Summary.Total
}

// Failed returns the subset of UseCases with verdict=fail. Returns a
// non-nil empty slice when none failed.
func (r *Report) Failed() []UseCaseResult {
	out := make([]UseCaseResult, 0)
	for _, uc := range r.UseCases {
		if uc.Verdict == "fail" {
			out = append(out, uc)
		}
	}
	return out
}
