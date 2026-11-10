package lastro

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Bridge to the Rust proof core (crates/lastro-core).
//
// The Go engine, the TS engine and the Rust prover are independent
// implementations of the same semantics. The Rust side does not trust the
// caller: it enumerates the declared input space itself and only says
// `proved` when the implementation's table covers every point of it.
//
// The Go SDK talks to the native `lastro-prove` binary (JSON on stdin, JSON on
// stdout), built with `cargo build --release -p lastro-core`.

// RustClauseProof is the Rust prover's verdict on one clause.
type RustClauseProof struct {
	ClauseID                string          `json:"clauseId"`
	Verdict                 string          `json:"verdict"`
	Evidence                string          `json:"evidence"`
	Checked                 int             `json:"checked"`
	Counterexample          *Counterexample `json:"counterexample,omitempty"`
	FailureRateUpperBound95 *float64        `json:"failureRateUpperBound95,omitempty"`
	Mutants                 MutantStats     `json:"mutants"`
	Error                   string          `json:"error,omitempty"`
}

// RustCoverage is how much of the declared input space the table covers.
type RustCoverage struct {
	Space            *float64 `json:"space"`
	Rows             int      `json:"rows"`
	Complete         bool     `json:"complete"`
	Missing          int      `json:"missing"`
	FirstMissing     Env      `json:"firstMissing,omitempty"`
	OutOfDomain      int      `json:"outOfDomain"`
	Nondeterministic int      `json:"nondeterministic"`
}

// RustContradiction is an input for which the clauses cannot all hold.
type RustContradiction struct {
	Input   Env      `json:"input"`
	Clauses []string `json:"clauses"`
}

// RustProofReport is the Rust prover's report.
type RustProofReport struct {
	Engine         string              `json:"engine"`
	Model          string              `json:"model"`
	Coverage       RustCoverage        `json:"coverage"`
	Clauses        []RustClauseProof   `json:"clauses"`
	Contradictions []RustContradiction `json:"contradictions"`
	TableDigest    string              `json:"tableDigest"`
	OK             bool                `json:"ok"`
}

// Row is one input and the output the implementation gave for it.
type Row struct {
	Input  Env `json:"input"`
	Output Env `json:"output"`
}

// RustProver checks clauses against an implementation's table.
type RustProver interface {
	Prove(ctx context.Context, model Model, clauses []Clause, table []Row) (RustProofReport, error)
}

// ExecProver runs the native lastro-prove binary.
type ExecProver struct {
	// Path to lastro-prove.
	Path string
}

// FindRustProver locates lastro-prove: $LASTRO_PROVE, then PATH, then the cargo target
// directories of this repository (searched upward from the working directory). Nil when absent.
func FindRustProver() *ExecProver {
	if p := os.Getenv("LASTRO_PROVE"); p != "" {
		return &ExecProver{Path: p}
	}
	if p, err := exec.LookPath("lastro-prove"); err == nil {
		return &ExecProver{Path: p}
	}
	dir, err := os.Getwd()
	for err == nil {
		for _, profile := range []string{"release", "debug"} {
			p := filepath.Join(dir, "target", profile, "lastro-prove")
			if st, e := os.Stat(p); e == nil && !st.IsDir() {
				return &ExecProver{Path: p}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return nil
}

// Prove sends the model, clauses and table to lastro-prove.
func (p *ExecProver) Prove(ctx context.Context, model Model, clauses []Clause, table []Row) (RustProofReport, error) {
	req := struct {
		Model   Model    `json:"model"`
		Clauses []Clause `json:"clauses"`
		Table   []Row    `json:"table"`
	}{model, nonNil(clauses), nonNil(table)}
	body, err := json.Marshal(req)
	if err != nil {
		return RustProofReport{}, err
	}
	cmd := exec.CommandContext(ctx, p.Path)
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()
	var exit *exec.ExitError
	if runErr != nil && !(errors.As(runErr, &exit) && exit.ExitCode() == 1) {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(stdout.Bytes(), &e) == nil && e.Error != "" {
			return RustProofReport{}, fmt.Errorf("lastro-core: %s", e.Error)
		}
		return RustProofReport{}, fmt.Errorf("lastro-prove: %v %s", runErr, stderr.String())
	}
	var rep RustProofReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		return RustProofReport{}, fmt.Errorf("lastro-prove: unreadable report: %w", err)
	}
	for i := range rep.Clauses {
		if ce := rep.Clauses[i].Counterexample; ce != nil {
			ce.Input, ce.Output = NormalizeEnv(ce.Input), NormalizeEnv(ce.Output)
		}
	}
	return rep, nil
}

// Tabulate runs the implementation over the whole (finite) input space: the table the Rust prover checks.
func Tabulate(model Model, subject Subject, limit int) ([]Row, error) {
	if limit <= 0 {
		limit = 100_000
	}
	if size := SpaceSize(model.Inputs); size > float64(limit) {
		return nil, fmt.Errorf("input space has %s points (limit %d); use sampling instead", jsNumber(size), limit)
	}
	inputs, err := EnumerateSpace(model.Inputs)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(inputs))
	for _, in := range inputs {
		out, err := subject(copyEnv(in))
		if err != nil {
			return nil, fmt.Errorf("subject failed for %s: %w", Stringify(in), err)
		}
		rows = append(rows, Row{Input: in, Output: NormalizeEnv(out)})
	}
	return rows, nil
}

// rustAgrees is true when the Rust prover gave every clause the same verdict as the report.
func rustAgrees(clauses []ClauseResult, rust RustProofReport) bool {
	for _, c := range clauses {
		found := false
		for _, r := range rust.Clauses {
			if r.ClauseID == c.ClauseID {
				found = r.Verdict == c.Verdict
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
