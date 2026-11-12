# Receipt: P-005 Fix what the use-case proofs found

Status: **applied** · submitted by claude · baseline `sha256:0e92f4f9ddb491b98f226ec9f6390af68aae4c3e3c8d1907c44ba805f80f013c` · approval `sha256:5d8964e1e67a1b4d349dfc4146fb39e59fc7b11efc996abaef18e66af30aa076`

Fixes the contradiction check for clauses with no if-part and brings the descriptions of the Rust prover's coverage and of the proof stream in line with the code.

## Use cases

### ✏️ UC-015 Return contradictions to the team (modify)

- Before: After derivation, a consistency check enumerates every output (when there are at most 4096) for each input and looks for inputs where no output satisfies all applicable clauses. Each distinct set of conflicting clauses becomes a decision card (origin consistency-check) asking which clause should give way, with one option per clause and the first recommended. spec.ok is false when any exist. returnDecisions(spec) reopens the artifact and appends these cards, skipping questions it already has, so the artifact cannot be approved until they are answered and the intents their answers create are approved. The Rust prover runs the same check.
- After: After derivation, a consistency check enumerates every output (when there are at most 4096) for each input and looks for inputs where no output satisfies all applicable clauses; a clause with no if-part applies to every input. Each distinct set of conflicting clauses becomes a decision card (origin consistency-check) asking which clause should give way, with one option per clause and the first recommended. spec.ok is false when any exist. returnDecisions(spec) reopens the artifact and appends these cards, skipping questions it already has, so the artifact cannot be approved until they are answered and the intents their answers create are approved. The TypeScript, .NET and Go engines and the Rust prover run the same check and find the same contradictions.
- **P-005.I-01** `approved` The consistency check treats a clause with no if-part as applying to every input, so it conflicts with any clause that demands another output for some input; the TypeScript, .NET, Go and Rust engines find the same contradiction.
- Gains: P-005.PF-01

### ✏️ UC-025 Cross-check with an independent prover (modify)

- Before: With a Rust prover (lastro-core, loaded as WASM in TypeScript or run as the native lastro-prove binary from Go and .NET), prove() tabulates the code over the whole input space and the prover re-checks every clause from that table. The prover drops out-of-domain rows, counts missing inputs and nondeterministic rows, and calls coverage complete only when every input appears once; it says proved only with complete coverage, checks vacuity, mutants and consistency itself, and returns a SHA-256 digest of the table. The proof is ok only if the SDK report is ok, the prover is ok and both engines give every clause the same verdict.
- After: With a Rust prover (lastro-core, loaded as WASM in TypeScript or run as the native lastro-prove binary from Go and .NET), prove() tabulates the code over the whole input space and the prover re-checks every clause from that table. The prover drops out-of-domain rows and counts missing inputs and nondeterministic rows (an input that appears with two different outputs). It calls coverage complete when every input of the space has a row and no row is nondeterministic or out of domain; an input repeated with the same output still counts as covered once. It says proved only with complete coverage, checks vacuity, mutants and consistency itself, and returns a SHA-256 digest of the table. The proof is ok only if the SDK report is ok, the prover is ok and both engines give every clause the same verdict.
- **P-005.I-02** `approved` The Rust prover calls coverage complete when every input of the space has a row and no row is nondeterministic or out of domain; an input repeated with the same output still counts as covered once.
  - replaces UC-025.PF-01 (The prover calls coverage complete only when every input of the space appears exactly once in the table, in its domain.): It reads 'every input appears once' as refusing a repeated identical row; a repeated row with the same output now counts as covered.; now guarded by P-005.PF-02
- Gains: P-005.PF-02

### ✏️ UC-043 Stream a proof run as it happens (modify)

- Before: lastro prove --stream (TypeScript, Go and .NET CLIs) prints one compact JSON object per line instead of the single result: start (model, clause ids, scenario ids), progress every 100 runs of the code under proof, one clause event per clause in plan order (clauseId, intentIds, verdict, evidence, and the counterexample when there is one), one scenario event per scenario (scenarioId, status), crosscheck (enginesAgree) when the Rust prover ran, and result with the body prove prints without --stream. Exit codes and the files written are the same as without it. The Go and .NET tests compare their stream with the TypeScript CLI's on one workspace.
- After: lastro prove --stream (TypeScript, Go and .NET CLIs) prints one compact JSON object per line instead of the single result: start (the proposal and the ids of the proofs it runs), progress every 100 runs of the code under proof, one proof event per proof, the proposal's own and every use case's regression proof alike (proof, role, verdict, passed, and the counterexample when there is one), crosscheck (enginesAgree) when the Rust prover ran, and result with the body prove prints without --stream. Exit codes and the files written are the same as without it. The Go and .NET tests compare their stream with the TypeScript CLI's on one workspace.
- **P-005.I-03** `approved` prove --stream starts with a start event naming the proposal and the ids of the proofs it runs.
- **P-005.I-04** `approved` prove --stream prints one proof event per proof, the proposal's own and every regression proof alike, with the proof id, role, verdict, whether it passed and the counterexample when there is one, instead of clause and scenario events.
  - replaces UC-043.PF-02 (The stream has one clause event per clause of the plan.): It expects one clause event per clause; the stream emits one proof event per proof.; now guarded by P-005.PF-04
- Gains: P-005.PF-03, P-005.PF-04

## Proofs

- **P-005.PF-01** ✅ proved — A clause with no if-part (y is a) and a clause that demands y is b when x holds come back as a contradiction when a differs from b, whether or not the first has an if-part. (P-005.I-01, contradictions/contradictions)
  - `requires in.a ≠ in.b; ensures (out.cards ≥ 1) ∧ ¬out.ok`
- **P-005.PF-02** ✅ proved — The prover calls coverage complete exactly when every input has a row and no row is nondeterministic or out of domain; a repeated row with the same output still counts. (P-005.I-02, crosscheck/crosscheck)
  - `ensures out.complete ⇔ (in.table ∈ {"complete", "repeated-row"})`
- **P-005.PF-03** ✅ proved — The first streamed event is start, naming the proposal and the ids of the proofs it runs. (P-005.I-03, stream/stream)
  - `ensures out.startFirst ∧ out.startNamesProofs`
- **P-005.PF-04** ✅ proved — The stream has one proof event per proof of the proposal and no clause events. (P-005.I-04, stream/stream)
  - `ensures (out.proofEvents = out.proofs) ∧ (out.clauseEvents = 0)`

## Regression

133 use-case proof(s) ran; 0 failed. Engines agree: yes.

- Unproved use cases (no proofs yet): UC-033, UC-035, UC-038, UC-045

