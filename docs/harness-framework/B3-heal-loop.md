# B3 — Heal Loop (Orchestration Only)

> Source plan: [`plan.md`](plan.md) §6.1 (Heal Loop), §10.4 (open: termination cap)
>
> Scope is smaller than the pre-rebase B4: heal *hint synthesis* already lives in `internal/aggregate/synthesize.go` (built during E8). This chunk owns only the iteration loop, edit application, and selective re-validation.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b3-heal-loop origin/main
```

## Parallelism

- **Can run in parallel with:** B4 (detection + generation), B5 (skill wrappers), B6 (CLI).
- **Must run after:** B1 (per-use-case aggregator drives the re-validate decision), B2 (lifecycle for selective re-runs).
- **Blocks:** B5's `/heal` sub-skill specifically; the four other skills in B5 don't depend on B3.

## What Phase A already delivered (consume, do not rebuild)

| Need | Existing package | Key API |
|---|---|---|
| `HealHint` type | `internal/aggregate` | `HealHint` struct on `AggregateSignal` |
| Per-sensor hint synthesis | `internal/aggregate` | `synthesizeHealHint`, `synthesizeStreamHealHint`, `observationalMissingHint` — all invoked by `Rollup` |
| Locus collection | `internal/aggregate` | `collectLoci(signals, verdict)` |

A failing sensor's `AggregateSignal` already carries a populated `HealHint`. This chunk *consumes* it; it does not generate hints.

## Scope

In:
- `internal/runtime/healloop/` — pure Go orchestration:
  - Input: a failing per-sensor `AggregateSignal` (or a per-use-case verdict carrying multiple).
  - Build an LLM prompt from the hint + context (failing sensor, owning use case, fixture state).
  - Receive a proposed `EditPlan` (file + diff) from an `LLMClient` interface (the LLM call itself is satisfied by the `/heal` skill scripts at B5 — this chunk defines the interface, not the impl).
  - Apply the edit transactionally (revert on re-validate fail).
  - Trigger selective re-validation via B2's lifecycle — only sensors whose grounding `uses:` or step `uses:` overlap the changed files.
  - Track iteration count; terminate per the cap from §10.4 (default 3).

Out:
- Hint *synthesis* (already in `internal/aggregate`).
- The LLM call itself and its prompt template (in `/heal` skill at B5; this chunk owns the loop, not the inference).
- CLI/UI of heal attempts (B6).

## Package layout (proposed)

```
internal/runtime/healloop/
```

## Inputs / Outputs

| Function | Input | Output |
|---|---|---|
| `healloop.Run` | failing `aggregate.AggregateSignal` (or `UseCaseVerdict`), `LLMClient`, `Config{MaxIterations}` | `HealResult{Status, IterationsUsed, EditsApplied, FinalVerdict}` |
| `healloop.AffectedSensors` | changed file paths, full sensor set | `[]Sensor` whose `uses:` overlap the changes |

`HealResult.Status` ∈ `{healed, exhausted, abandoned}` (abandoned = LLM refused to propose or proposed an unparseable edit).

## Dependencies

- B1 (per-use-case aggregator — to know what re-validate returns).
- B2 (lifecycle — to invoke selective re-runs).
- Phase A: `internal/aggregate` (for `HealHint`, `Locus`), `internal/sensor` (for grounding lookup), `internal/usecase`.
- An `LLMClient` interface defined here, satisfied by `/heal` scripts in B5.

## Open questions for `/brainstorming`

1. **Termination cap.** Plan §10.4 suggests 3. Hard default or per-policy override? Recommendation: per-policy override with default 3; check whether `policy.EffectivePolicy` exposes this field — extend `internal/policy` if not.
2. **Edit transactionality.** `git stash` (assumes git-managed repo), in-memory diff replay, or write-temp-then-swap? Recommendation: `git stash` if repo is git-managed; fall back to file backup otherwise.
3. **Affected-sensor scoping.** File-level overlap from sensor metadata + changed paths, or use-case-level? Recommendation: file-level. Check whether `internal/sensor` exposes the file-association data needed; extend if not.
4. **Multi-hint loops.** When a use case fails with N per-sensor hints, do we heal them sequentially (one iteration per hint) or build one consolidated prompt? Recommendation: sequential — one hint per iteration, in `Locus`-priority order, to keep edits small and easy to revert.
5. **LLM client contract.** Sync `Propose(hint, context) → EditPlan`? Recommendation: sync — heal is a slow path; simplicity wins.
6. **Heal loop interleaving.** If two use cases both fail, does one heal block the other? Recommendation: per-use-case loops run serially in v1.

## Deliverable acceptance

- `healloop.Run` against a synthetic failing `AggregateSignal` (with a hand-written `HealHint`) and a stub `LLMClient`: applies the proposed edit, re-validates via a stub lifecycle, reports `healed` with `IterationsUsed=1`.
- Iteration cap test: stub LLM returns the same bad edit 4 times with cap=3 → `exhausted` after 3 iterations, file state reverted.
- `AffectedSensors` returns the correct subset on a 10-sensor manifest where 2 sensors `uses:` overlap a changed file path.
- `git stash` revert path verified: edit applied → re-validate fails → stash popped → file restored byte-identical.
- Tests run with `-race` clean.
