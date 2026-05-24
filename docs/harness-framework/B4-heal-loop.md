# B4 — Heal Loop

> Source plan: [`plan.md`](plan.md) §6.1 (Heal Loop), §10.4 (open: heal loop termination)

The heal loop is the framework's self-correction primitive: take a failing `Signal` or `AggregateSignal`, feed its `heal_hint` to an LLM, apply the proposed edit, re-validate only the affected sensors, repeat until pass or termination cap.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b4-heal-loop origin/main
```

## Parallelism

- **Can run in parallel with:** B5 (detection + generation), B6 (skill wrappers — `/heal` skill in B6 will wire onto this chunk once both land), B7 (CLI).
- **Must run after:** B2 (per-use-case aggregator drives the re-validate decision), B3 (executor for selective re-runs).
- **Blocks:** B6's `/heal` sub-skill specifically; the four other skills in B6 do not depend on B4.

## Scope

In:
- `internal/runtime/healLoop/` — pure Go orchestration:
  - Accepts a failing `Signal` or `AggregateSignal` (must carry a populated `heal_hint`).
  - Builds an LLM prompt from the hint + context (failing sensor, owning use case, fixture state).
  - Receives a proposed `EditPlan` (file + diff) from the LLM caller (the LLM call itself is abstracted behind an interface — actual prompt dispatch lives in `/heal` skill at B6, not here).
  - Applies the edit (write-through to repo) under a transactional guard (revert on re-validate fail).
  - Triggers selective re-validation via B3's lifecycle — only the sensors whose `uses:` overlap with the changed files.
  - Tracks iteration count; terminates per the cap from §10.4 (default 3).

Out:
- The LLM call and its prompt template (in `/heal` skill at B6 — this chunk owns the orchestration loop, not the inference).
- Reporting/UI of heal attempts (CLI, B7).
- Resolving sensor identity for "affected sensors" (uses the sensor identity scheme from §10.5 — recommended content-hash + sidecar; this chunk consumes whatever is decided).

## Inputs / Outputs

| Function | Input | Output |
|---|---|---|
| `healLoop.Run` | failing `Signal` or `AggregateSignal`, `LLMClient`, `Config{MaxIterations}` | `HealResult{Status, IterationsUsed, EditsApplied, FinalVerdict}` |
| `healLoop.AffectedSensors` | changed file paths, full sensor set | `[]Sensor` whose grounding/`uses:` overlap the changes |

`HealResult.Status` ∈ `{healed, exhausted, abandoned}` (abandoned = LLM refused to propose, or proposed an edit that didn't compile/parse).

## Dependencies

- B2 (per-use-case aggregator — to know what "re-validate" returns).
- B3 (lifecycle — to invoke selective re-runs).
- Phase A: `internal/signal`, `internal/aggregate`, `internal/sensor`, `internal/usecase`.
- An `LLMClient` interface — defined here, satisfied by the `/heal` skill scripts in B6.

## Open questions for `/brainstorming`

1. **Termination cap.** Plan §10.4 suggests 3. Hard default or per-policy override? Recommendation: per-policy override with default 3; surface on `ValidationPolicy` (coordinate with E9 if extension needed).
2. **Edit transactionality.** Apply edit → re-validate → revert on failure. What's the revert primitive? `git stash`, in-memory diff replay, or write-temp-then-swap? Recommendation: `git stash` if repo is git-managed (likely always); fall back to file backup otherwise.
3. **Affected-sensor scoping.** Plan says "re-validate affected sensors only". Affected = any sensor whose `uses:` (top-level stack components OR step-level fixtures) overlaps the changed files? Or any sensor in the same use case? Recommendation: file-level overlap, computed from sensor metadata + the changed paths.
4. **LLM client contract.** Sync `Propose(hint, context) → EditPlan`? Streaming? Recommendation: sync — heal is a slow path, simplicity wins.
5. **Heal loop interleaving.** If two use cases both fail, does one heal block the other? Recommendation: per-use-case loops run serially in v1; document parallel as a future extension.
6. **Abandon vs exhausted.** Distinguish "LLM gave up" from "ran out of iterations" in `HealResult`? Recommendation: yes — different operator response.

## Deliverable acceptance

- `healLoop.Run` against a synthetic failing sensor with a hand-written `heal_hint`: a stub LLM client returns a fix, the loop applies it, re-validates via a stub lifecycle, reports `healed` with `IterationsUsed=1`.
- Iteration cap test: stub LLM returns the same bad edit 4 times with cap=3 → `exhausted` after 3 iterations, file state reverted.
- `AffectedSensors` returns the correct subset on a 10-sensor manifest where 2 sensors `uses:` reference a changed file path.
- Tests run with `-race` clean; the executor invocation is properly serialized within a single `Run` call.
