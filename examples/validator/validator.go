package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iurykrieger/lastro/internal/aggregate"
	"github.com/iurykrieger/lastro/internal/usecase"
	"github.com/oklog/ulid/v2"
)

// ValidateAll enumerates use cases under target/.harness/use-cases,
// invokes /validate-use-case once per id (with cmd.Dir=target), parses
// the JSONL output, and aggregates into a Report. Writes the Report to
// <target>/.harness/reports/<run-id>/report.json.
//
// Skill exit codes 0/1/2 all produce a UseCaseResult. Exit code 3 is a
// script error and returns a non-nil error.
func ValidateAll(ctx context.Context, target string, skills *SkillBinaries) (*Report, error) {
	if skills == nil || skills.ValidateUseCase == "" {
		return nil, errors.New("SkillBinaries.ValidateUseCase is required")
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}

	ids, err := enumerateUseCases(abs)
	if err != nil {
		return nil, fmt.Errorf("enumerate use cases: %w", err)
	}
	sort.Strings(ids)

	r := &Report{
		SchemaVersion: ReportSchemaVersion,
		RunID:         ulid.Make().String(),
		Target:        abs,
		StartedAt:     time.Now().UTC(),
		UseCases:      make([]UseCaseResult, 0, len(ids)),
	}

	for _, id := range ids {
		res, err := runOne(ctx, abs, skills.ValidateUseCase, id)
		if err != nil {
			return nil, fmt.Errorf("run use case %q: %w", id, err)
		}
		r.UseCases = append(r.UseCases, res)
		r.Summary.Total++
		switch res.Verdict {
		case "pass":
			r.Summary.Passed++
		case "fail":
			r.Summary.Failed++
		case "inconclusive":
			r.Summary.Inconclusive++
		}
	}

	r.EndedAt = time.Now().UTC()
	if err := writeReport(abs, r); err != nil {
		return nil, fmt.Errorf("write report: %w", err)
	}
	return r, nil
}

func enumerateUseCases(target string) ([]string, error) {
	dir := filepath.Join(target, ".harness", "use-cases")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		uc, err := usecase.UnmarshalOnly(data)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		ids = append(ids, uc.ID)
	}
	return ids, nil
}

// persistedVerdict mirrors the envelope emitted as the final stdout line
// by /validate-use-case.
type persistedVerdict struct {
	UseCaseVerdict struct {
		Verdict    string  `json:"verdict"`
		Confidence float64 `json:"confidence"`
	} `json:"use_case_verdict"`
	UseCaseRunID string `json:"use_case_run_id"`
	SensorRuns   []struct {
		SensorID string `json:"sensor_id"`
		Verdict  string `json:"verdict"`
	} `json:"sensor_runs"`
}

// sensorEnvelope mirrors a per-sensor AggregateSignal line on the JSONL
// stream. We capture the first non-nil HealHint to attach to UseCaseResult.
type sensorEnvelope struct {
	SensorID string              `json:"sensor_id"`
	Verdict  string              `json:"verdict"`
	HealHint *aggregate.HealHint `json:"heal_hint,omitempty"`
}

func runOne(ctx context.Context, target, binPath, ucID string) (UseCaseResult, error) {
	cmd := exec.CommandContext(ctx, binPath, ucID)
	cmd.Dir = target
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	// Exit 3 = script error. Everything else (0/1/2) is verdict-derived
	// and parsable from stdout.
	if ee, ok := runErr.(*exec.ExitError); ok {
		if ee.ExitCode() == 3 {
			return UseCaseResult{}, fmt.Errorf("skill script error (exit 3) for %q: %s", ucID, stderr.String())
		}
	} else if runErr != nil {
		return UseCaseResult{}, fmt.Errorf("exec skill: %v: stderr=%s", runErr, stderr.String())
	}

	result := UseCaseResult{UseCaseID: ucID, Stdout: stdout.String()}

	var firstHint *aggregate.HealHint
	var pv *persistedVerdict
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var asPV persistedVerdict
		if err := json.Unmarshal([]byte(line), &asPV); err == nil && asPV.UseCaseVerdict.Verdict != "" {
			pv = &asPV
			continue
		}
		var se sensorEnvelope
		if err := json.Unmarshal([]byte(line), &se); err == nil && se.SensorID != "" {
			if firstHint == nil && se.HealHint != nil {
				firstHint = se.HealHint
			}
		}
	}

	if pv == nil {
		return UseCaseResult{}, fmt.Errorf("no persisted verdict in skill stdout for %q: %s", ucID, stdout.String())
	}

	result.Verdict = pv.UseCaseVerdict.Verdict
	for _, sr := range pv.SensorRuns {
		result.SensorRuns = append(result.SensorRuns, SensorRunSummary{
			SensorID: sr.SensorID,
			Verdict:  sr.Verdict,
		})
	}
	result.HealHint = firstHint
	return result, nil
}

func writeReport(target string, r *Report) error {
	dir := filepath.Join(target, ".harness", "reports", r.RunID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.json"), b, 0o644)
}
