# B5 — Skill Wrappers (Design)

> Source chunk: [`docs/harness-framework/B5-skill-wrappers.md`](../../harness-framework/B5-skill-wrappers.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §5 (Skills table rows 4–8), §6 (Sensor Execution Model)
> Phase A entities consumed (read-only): E4 (UseCase), E5 (Fixture), E6 (Sensor), E7 (Signal), E8 (AggregateSignal), E9 (ValidationPolicy)
> Phase B chunks consumed: B1 (fixture binder + per-use-case aggregator), B2 (executor + lifecycle), B3 (heal loop — sub-PR 3 only)
> Brainstorm date: 2026-05-25

## 1. Purpose

B5 owns the six slash-command surfaces that wrap the Go runtime so Claude (or any LLM agent) can drive validation interactively. Each skill is a thin shell over an existing internal package — the Go code in `scripts/` does I/O, error reporting, and exit-code mapping; the runtime logic stays in `internal/lifecycle`, `internal/runtime/aggregator/usecase`, and `internal/runtime/healloop`.

Six skills (the source chunk specified five; brainstorming added `/tail-sensor-signals`):

| Skill | Wraps | Kind |
|---|---|---|
| `/run-sensor` | `lifecycle.RunSensor` | assertion, blocking |
| `/start-sensor` | `lifecycle.StartSensor` | observational, async spawn |
| `/stop-sensor` | `lifecycle.StopSensor` | observational, terminal |
| `/tail-sensor-signals` | `<runDir>/signals.jsonl` reader | observational, read-only |
| `/validate-use-case` | `lifecycle.RunSensor` + `aggregator.UseCase` | orchestrator |
| `/heal` | `healloop.ApplyEdit` + `lifecycle.RunSensor` + `aggregator.UseCase` | single-shot, hook-driven loop |

B5 does **not** own:

- Detection or sensor generation — B4.
- Runtime mechanism (executor, fixture binding, rollup, heal-hint synthesis) — B1, B2, Phase A.
- The CLI binary `harness` — B6 (sibling surface; same underlying packages, different transport).
- The Claude Code hook *runtime* — that is the harness itself; B5 ships a sample config and a small detection script.
- Long-term retention of `.harness/runtime/` artifacts — pruning is a future B6 concern.

## 2. Scope

**In:**

- Six skills under `skills/<name>/` — each a `skill.md` (LLM-facing prompt) plus `scripts/main.go` (Go binary).
- Two new shared libraries:
  - `lib/skillio/` — repo discovery, JSONL stdout, structured stderr, exit-code conventions. Shared by every skill (B4's three included).
  - `lib/skillruntime/` — B5-only bootstrap that loads `.harness/` artifacts and wires a `*lifecycle.Lifecycle`. Also holds the DAG-aware scheduler used by `/validate-use-case` and `/heal` re-validate.
- Two new `.harness/` artifacts:
  - `.harness/runtime/use-cases/<usecase-id>/<run-id>/verdict.json` — per-use-case `UseCaseVerdict`.
  - `.harness/runtime/<sensor-id>/heal-state.json` — per-sensor heal counter (sibling of run directories).
- A sample post-tool-use hook config under `skills/heal/hooks/` that fires the heal loop from Claude Code.
- Tests covering the deliverable acceptance criteria below.

**Out:**

- The `harness` binary (B6).
- B4's three detection/generation skills, even though they share `lib/skillio/`.
- `LLMClient` implementation — B3 defines the interface; the CLI satisfies it; the `/heal` skill does **not** (the hook + Claude in the conversation drive the loop instead).
- Pruning, log rotation, or compression of `.harness/runtime/` artifacts.

## 3. Open questions resolved

The source chunk listed five open questions. Brainstorming on 2026-05-25 resolved each and surfaced four follow-ups; all resolutions below.

| # | Question | Decision |
|---|---|---|
| 1 | Skill-script transport | **`go run` per skill.** Each skill ships `scripts/main.go` invoked via `go run ./skills/<name>/scripts <args>`. No committed binaries; identical dev/CI behavior; ~1s cold start is acceptable for human-in-loop skills. When B6's `harness` binary stabilizes, transport may be swapped without rewriting skill markdown. |
| 2 | `/validate-use-case` parallelism | **Parallel where the DAG allows, serial within a chain.** Matches B2's model. Sibling sensors with no `depends_on` between them schedule concurrently via an in-process worker pool. |
| 3 | `/stop-sensor` handle format | **`<sensor-id>:<run-id>` colon-separated string.** Both halves are ULIDs (already used by B2). The script splits and calls `lifecycle.LoadHandle(sensorID, runID)`. No new sidecar file — B2's `running_sensors.json` is the source of truth. |
| 4 | `/heal` LLMClient implementation | **Single-shot script; loop in Claude Code hook.** `/heal` is one iteration: take signal id + Claude-piped `EditPlan` → apply via `healloop.ApplyEdit` → re-validate `AffectedSensors` → report. A `PostToolUse` hook on `/validate-use-case` and `/run-sensor` detects fail and prompts Claude to invoke `/heal` again until cap or pass. B3's `LLMClient` is reserved for the `harness heal` CLI. |
| 5 | Failure surface | **Non-zero exit on `verdict=fail`.** Exit code scheme is uniform across all six skills: `0` pass, `1` fail, `2` inconclusive, `3` script error. Matches B6's CLI conventions for surface parity. |

Follow-ups surfaced during the brainstorm:

| # | Question | Decision |
|---|---|---|
| F1 | Sixth skill `/tail-sensor-signals` | **Added.** Observational sensors emit signals between `/start-sensor` and `/stop-sensor`; the LLM has no read path without this. Pure file reader over `<runDir>/signals.jsonl`; no new runtime API. |
| F2 | Persistence location for heal state | **`.harness/runtime/<sensor-id>/heal-state.json`.** Per-sensor counter, sibling of the per-run directories. Counter resets to 0 on `verdict=pass`. |
| F3 | `expectedObs` source for `/start-sensor` | **Carried on the loaded `Sensor` struct.** B4's `/create-sensors` populates the expected-observation list at generation time; B5 passes it through to `lifecycle.StartSensor`. (Open: confirm the exact field name once B4 lands; if absent, B5 reads from an explicit `--expect` flag as a fallback.) |
| F4 | Synthetic dependency-failed termination reason | **Reuse `TerminationReason=stopped`** with disambiguation in `heal_hint.summary` (`"skipped: depends_on <id> failed"`). Avoids schema churn during Phase B. Adding a `skipped` enum is a follow-up if downstream consumers need to branch on it. |

## 4. Architecture

```
┌──────────────────────────┐         ┌──────────────────────────┐
│ Claude Code session      │         │ harness CLI (B6)         │
│  invokes skills via      │         │  invokes lifecycle       │
│  /run-sensor, /heal, …   │         │  directly                │
└─────────────┬────────────┘         └────────────┬─────────────┘
              │                                   │
              ▼                                   ▼
        skills/<name>/scripts/main.go     cmd/harness/main.go
              │                                   │
              ▼                                   │
       lib/skillruntime ◄──────────────────────────┘
       lib/skillio  (also B4's skills)
              │
              ▼
       internal/lifecycle  (B2)
       internal/runtime/aggregator/usecase  (B1)
       internal/runtime/healloop  (B3, sub-PR 3 only)
              │
              ▼
       internal/runtime/executor  (B2)
       internal/{sensor, fixture, usecase, aggregate, policy, signal}  (Phase A)
```

Both surfaces share `lib/skillruntime` (boot + scheduler), but the CLI may construct a `*Lifecycle` directly when it doesn't need the skill conventions. B5 only owns the path through `lib/skillruntime`.

## 5. Layout

```
skills/
├── detect-stack/              ← B4 (not in this spec; listed for context)
├── detect-use-cases/          ← B4
├── create-sensors/            ← B4
│
├── run-sensor/
│   ├── skill.md               ≤200 lines (CLAUDE.md rule 4); synthesize at ≥100
│   └── scripts/main.go
├── start-sensor/
│   └── (same shape)
├── stop-sensor/
│   └── (same shape)
├── tail-sensor-signals/
│   └── (same shape)
├── validate-use-case/
│   └── (same shape)
└── heal/
    ├── skill.md
    ├── scripts/main.go
    └── hooks/
        ├── settings.snippet.json     sample PostToolUse config
        └── check-heal-needed.sh      sample detection script

lib/
├── skillio/                          shared by B4 + B5 skills
│   ├── repo.go                       find repo root, locate .harness/
│   ├── output.go                     JSONL stdout, structured stderr
│   └── errors.go                     typed error envelope
└── skillruntime/                     B5-only consumers
    ├── boot.go                       BootLifecycle(repoRoot) → *Lifecycle + cleanup
    ├── handles.go                    parse "<sensor-id>:<run-id>" ↔ *lifecycle.Handle
    └── schedule.go                   DAG-aware worker pool (for /validate-use-case + /heal)
```

**`lib/` rationale (CLAUDE.md rule 3):** every B5 script calls `BootLifecycle`; B4's three scripts plus B5's six all use the same stdout/stderr/exit conventions. Both libs have ≥2 callers from the start, so they're created in the first sub-PR that lands rather than promoted later.

**Sub-PR coordination:** whichever of sub-PR 1 or sub-PR 2 lands first introduces `lib/skillio/` and `lib/skillruntime/`. The second rebases and consumes them as-is.

## 6. Transport contract

| Surface | Convention |
|---|---|
| Invocation | `go run ./skills/<name>/scripts <args>` from the skill markdown |
| Ids | `argv` positional (`run-sensor <sensor-id>`, `stop-sensor <sensor-id>:<run-id>`, `heal <signal-id>`) |
| Structured payloads | `stdin` JSON — only `/heal` uses this (Claude pipes the `EditPlan`) |
| Streamed signals | `stdout` JSONL, one signal/aggregate per line |
| Terminal record | last line on `stdout`: `AggregateSignal` (sensor skills) or `UseCaseVerdict` (`/validate-use-case`) |
| Script-level errors | `stderr` single-line JSON: `{"error":"…","code":"…","details":{…}}` |
| Exit codes | `0` pass · `1` fail · `2` inconclusive · `3` script error |

`stdout` is a clean JSONL stream — consumers can `tail -n 1` for just the terminal record, or pipe through `jq` for analysis. `stderr` is reserved for script errors only; runtime errors (failed sensor, malformed YAML) surface as a terminal `AggregateSignal` with `Verdict=inconclusive` on `stdout`.

## 7. `.harness/` directories B5 touches

| Path | Owner | B5's role |
|---|---|---|
| `.harness/sensors/<id>.yaml` | B4 | read on every `BootLifecycle` |
| `.harness/use-cases/<id>.yaml` | B4 | read on every `BootLifecycle` |
| `.harness/fixtures/<id>.yaml` | B4 | read on every `BootLifecycle` |
| `.harness/runtime/<sensor-id>/<run-id>/` | B2 | `/run-sensor` reads `aggregate.json`; `/start-sensor`+`/stop-sensor` reference via Handle; `/tail-sensor-signals` reads `signals.jsonl` |
| `.harness/runtime/running_sensors.json` | B2 | `/stop-sensor` resolves a handle via `lifecycle.LoadHandle`; `/tail-sensor-signals` polls for sensor termination |
| `.harness/runtime/<sensor-id>/heal-state.json` | **B5 (new)** | `/heal` increments; hook reads to check cap |
| `.harness/runtime/use-cases/<usecase-id>/<run-id>/verdict.json` | **B5 (new)** | `/validate-use-case` writes; `/heal` re-validate reads/overwrites |

## 8. Skill flows

### 8.1 `/run-sensor <sensor-id>`

```
1. argv parse → sensor-id
2. lc, cleanup := skillruntime.BootLifecycle(repoRoot)
3. if sensor.Kind == observational → stderr code 3 ("use /start-sensor"); exit 3
4. agg, err := lc.RunSensor(ctx, sensorID, nil)
5. replay <runDir>/signals.jsonl line-by-line to stdout
6. emit agg as the final stdout line
7. exit code: agg.Verdict (pass=0, fail=1, inconclusive=2)
```

Signal replay happens **after** `RunSensor` returns; no race with the in-flight executor write. The blocking nature of `lifecycle.RunSensor` guarantees `signals.jsonl` is closed before the replay starts.

### 8.2 `/start-sensor <sensor-id>`

```
1. argv parse → sensor-id
2. lc, cleanup := skillruntime.BootLifecycle(repoRoot)
3. expectedObs := sensor.ExpectedObservations          // F3 — field name pending B4; nil fallback OK
4. handle, err := lc.StartSensor(ctx, sensorID, expectedObs)
5. emit {"handle": "<sensor-id>:<run-id>", "run_dir": handle.RunDir, "pid": handle.PID}
6. exit 0
```

`lifecycle.StartSensor` returns `ErrAssertionSensor` if invoked on a `kind:assertion` sensor → propagate to stderr code 3.

### 8.3 `/stop-sensor <sensor-id>:<run-id>`

```
1. argv parse → "<sensor-id>:<run-id>"; validate both halves are ULID-shaped (26 chars, base32)
2. lc, cleanup := skillruntime.BootLifecycle(repoRoot)
3. h, err := lc.LoadHandle(sensorID, runID)
4. agg, err := lc.StopSensor(ctx, h)
5. emit agg to stdout
6. exit code: agg.Verdict
```

Live and dead handles both work — B2 reads `aggregate.json` from disk if the registry entry is gone.

### 8.4 `/tail-sensor-signals <sensor-id>:<run-id> [--follow] [--since=<n>]`

```
1. argv parse → handle, flags
2. signalsPath := .harness/runtime/<sensor-id>/<run-id>/signals.jsonl
3. mode A (snapshot):  read all lines from --since offset → stdout → exit 0
4. mode B (--follow):  read all lines → emit
   poll loop (200ms):
     read new bytes → emit
     if handle missing from running_sensors.json AND no new bytes for 1s → exit 0
```

- No new runtime API; pure file reader.
- Polling, not fsnotify — 200ms latency is invisible for human-perceivable signal rates; avoids fsnotify-on-WSL/Windows quirks.
- `--since=<n>` lets the LLM resume after a previous tail without re-reading.
- Exit `0` on graceful EOF; `3` on bad handle or unreadable file. The skill does not opine on verdict (`/stop-sensor` owns the terminal aggregate).

### 8.5 `/validate-use-case <usecase-id>`

```
1. argv parse → usecase-id; load .harness/use-cases/<id>.yaml
2. lc, cleanup := skillruntime.BootLifecycle(repoRoot)
3. sensors := all sensors with use_case_id == <id>
4. order := sensor.ResolveExecutionOrder(sensors)
5. policy := policy.Resolve(global, local)
6. ucRunID := ulid.Make()
7. Schedule sensors via DAG-aware worker pool (§9)
8. For each completed sensor:
     agg := lc.RunSensor(ctx, s.ID, s.ExpectedObservations)
     emit agg as JSON line on stdout
     record into []AggregateSignal
9. verdict := aggregator.UseCase(uc, signals, policy)
10. write verdict to .harness/runtime/use-cases/<usecase-id>/<ucRunID>/verdict.json
    (include sensor_runs: [{sensor_id, run_id}, …] for traceability)
11. emit verdict as final stdout line
12. exit per verdict.Verdict (0/1/2)
```

### 8.6 `/heal <signal-id>`

```
1. argv parse → signal-id (the failing signal/aggregate to heal)
2. stdin → EditPlan JSON (Claude wrote it just before invoking the script)
3. lc, cleanup := skillruntime.BootLifecycle(repoRoot)
4. state := load .harness/runtime/<sensor-id>/heal-state.json
5. if state.Iteration >= state.MaxIterations:
       stderr {"code":"heal-exhausted","iterations":N}; exit 3
6. plan := decode(stdin); validate against B3's EditPlan schema (imported from internal/runtime/healloop)
7. applied, undo := healloop.ApplyEdit(plan)
8. affected := healloop.AffectedSensors(plan.ChangedPaths, allSensors)   // allSensors loaded by BootLifecycle
9. for each s in affected (DAG-scheduled per §9):
       agg := lc.RunSensor(ctx, s.ID, s.ExpectedObservations)            // see F3 caveat on field name
       stream agg to stdout
10. verdict := aggregator.UseCase(uc, allAggs, policy)                   // policy resolved by BootLifecycle
11. if verdict.Verdict == fail:
       undo()
       state.Iteration++; append history entry
       persist state
    else:
       state.Iteration = 0; clear history; persist
12. write verdict to use-cases/<usecase-id>/<run-id>/verdict.json
13. emit verdict as final stdout line
14. exit per verdict (0/1/2)
```

**Rollback policy:** always rollback on `verdict=fail`. The hook requests a fresh `EditPlan` next iteration; cumulative bad edits would compound. Implementation: `git stash` if `.git/` exists, file backup otherwise (B3 owns this choice).

**`EditPlan` JSON shape** (B3 owns the canonical type):

```json
{
  "summary": "<one-line description>",
  "edits": [
    { "path": "src/handlers/orders.ts", "before": "…", "after": "…" }
  ],
  "rationale": "<why this fixes the failing signal>"
}
```

`before`/`after` lets the apply step do exact-string replacement (no patch fuzz). The array supports cross-file edits.

### 8.7 Hook config (sample, `skills/heal/hooks/`)

`settings.snippet.json`:

```json
{
  "PostToolUse": [
    {
      "matcher": "Skill\\((validate-use-case|run-sensor)\\)",
      "hooks": [{ "type": "command", "command": ".claude/scripts/check-heal-needed.sh" }]
    }
  ]
}
```

`check-heal-needed.sh` pseudo-logic:

```
1. Read tool exit code (from $CLAUDE_TOOL_EXIT_CODE)
2. If 0 → exit silently (verdict passed)
3. If 1 → read latest verdict.json + sensor heal-state.json
4. If state.Iteration < state.MaxIterations:
     stdout: "Validation failed with signal <id>. Iteration N/M. Propose an EditPlan and run /heal <id>."
5. If state.Iteration >= state.MaxIterations:
     stdout: "Heal exhausted after N attempts. Manual intervention needed."
```

The hook **does not invoke** `/heal` directly — it prompts Claude (via stdout text Claude reads on the next turn) to do so. Claude is the loop driver; the hook is the per-iteration trigger. This keeps inference in Claude and loop control declarative in the hook config.

**Open detail:** exact Claude Code hook env-var names and matcher syntax need confirmation during implementation. The principle (`PostToolUse` on validate/run-sensor surface) is stable; the exact field names are an implementation detail.

## 9. DAG-aware scheduler (`lib/skillruntime/schedule.go`)

Used by `/validate-use-case` (all sensors) and `/heal` re-validate (affected sensors only).

```
inputs:
  - []Sensor with depends_on edges
  - lifecycle.RunSensor invoker
  - max parallelism (default runtime.NumCPU())

state:
  ready    set of sensors with all deps in done
  done     map[sensorID]AggregateSignal
  running  in-flight goroutines (bounded by errgroup)

loop:
  while len(done) < len(sensors):
    submit all ready sensors not already running to errgroup
    wait for any completion
    on completion:
      done[s.ID] = agg
      if agg.Verdict == fail:
        for each transitive dependent of s:
          done[dep.ID] = synthesizeSkipped(dep, s)
      else:
        recompute ready set
    on ctx cancel: cancel errgroup, mark remaining sensors inconclusive

return done as ordered []AggregateSignal
```

`synthesizeSkipped`:

```go
aggregate.AggregateSignal{
    SensorID: dep.ID, UseCaseID: dep.UseCaseID, Angle: dep.Angle,
    Verdict: enums.VerdictInconclusive,
    TerminationReason: enums.TerminationStopped,
    HealHint: aggregate.HealHint{
        Summary: fmt.Sprintf("skipped: depends_on %s failed", failed.SensorID),
        Rationale: "sensor %s's AggregateSignal verdict=fail; %s did not execute",
    },
}
```

~80 LOC in `lib/skillruntime/schedule.go`. Reused by `/heal`'s affected-sensor re-validation, so it justifies its place in the shared library on first introduction.

## 10. Sub-ordering & branching

Three independent sub-PRs, each branching from `origin/main` (no stacking, per the B5 source chunk and CLAUDE.md convention):

| Sub-PR | Branch | Skills | Blocked on |
|---|---|---|---|
| 1 | `feat/b5-lifecycle-wrappers` | `/run-sensor`, `/start-sensor`, `/stop-sensor`, `/tail-sensor-signals` | none — B1+B2 in main |
| 2 | `feat/b5-validate-use-case` | `/validate-use-case` | none — B1+B2 in main |
| 3 | `feat/b5-heal` | `/heal` + hook config | B3 landing |

Sub-PR 1 and 2 may race on `lib/skillio/` and `lib/skillruntime/`; whichever rebases second adjusts. Sub-PR 3 expects the libs to exist (any earlier sub-PR introduces them).

Branching template:

```
git fetch origin
git checkout -b feat/b5-<sub-pr-name> origin/main
```

## 11. Deliverable acceptance

### 11.1 Sub-PR 1 — Lifecycle quartet

- `/run-sensor <id>` on a passing assertion sensor → streams signals + terminal `AggregateSignal` JSON; exit 0.
- `/run-sensor <id>` on a failing assertion sensor → `heal_hint` non-empty (populated by `internal/aggregate/synthesize.go`); exit 1.
- `/run-sensor` on a `kind:observational` sensor → exit 3 with structured stderr `{"code":"wrong-kind","hint":"use /start-sensor"}`.
- `/start-sensor <id>` → emits `{handle, run_dir, pid}`; corresponding entry visible in `.harness/runtime/running_sensors.json`.
- `/stop-sensor <sensor-id>:<run-id>` → emits terminal `AggregateSignal` with `completeness` reflecting expected observations; exit matches verdict.
- `/stop-sensor` against a terminated sensor (registry entry already gone) → reads `aggregate.json` from disk; exit matches verdict.
- `/tail-sensor-signals <handle> --follow` against an observational sensor emitting 5 signals over 3s → all 5 in order, exit 0 within 1.2s of `/stop-sensor` from a sibling process.
- `/tail-sensor-signals <handle> --since=N` → resumes from line N.
- Malformed handle (not `id:id`, wrong length, non-ULID) → exit 3 with `{"code":"bad-handle"}`.

### 11.2 Sub-PR 2 — Use case orchestrator

- `/validate-use-case <id>` on a 3-sensor use case (2 assertion + 1 observational, mixed `depends_on`) → topo-sorted parallel exec; `UseCaseVerdict` byte-identical to `aggregator.UseCase(uc, signals, policy)`.
- Dependency-failed scenario: sensor B `depends_on` A, A fails → B's synthetic `AggregateSignal` has `Verdict=inconclusive`, `TerminationReason=stopped`, `heal_hint.summary` contains `"skipped: depends_on A failed"`.
- Determinism: same fixtures + sensors + policy → byte-identical `UseCaseVerdict` JSON across runs (golden test parallels B1's `aggregator/usecase` golden).
- `.harness/runtime/use-cases/<usecase-id>/<run-id>/verdict.json` written with `sensor_runs: [{sensor_id, run_id}, …]` traceability.
- Cancellation: `ctx` cancellation mid-validate kills all in-flight sensors; partial verdict reported with `Verdict=inconclusive`.
- `go test -race` clean.

### 11.3 Sub-PR 3 — Heal + hook

- `/heal <signal-id>` with a Claude-piped `EditPlan` against a known-failing signal → applies edit, re-validates only `healloop.AffectedSensors`, emits new verdict, exit matches.
- Re-validate fail → file state restored (git stash pop, or backup restore for non-git repos); heal-state iteration incremented.
- Re-validate pass → heal-state iteration reset to 0; history cleared.
- `.harness/runtime/<sensor-id>/heal-state.json` increments per attempt; cap exhaustion (counter ≥ `max_iterations`) → script refuses with `{"code":"heal-exhausted"}` on stderr, exit 3, **does not apply edit**.
- Malformed `EditPlan` on stdin → exit 3 with `{"code":"bad-edit-plan","details":{…}}`; heal-state unchanged.
- Sample hook config + detection script verified to fire correctly in a hand-driven dogfood walkthrough; actual production wiring is B7's job.

## 12. Test strategy

- **Unit:** argv parsing, error paths, handle parsing, exit-code mapping. Per-script ~150–250 LOC.
- **Integration:** real `BootLifecycle` against `internal/lifecycle/testdata/sensors/*.yaml` + a `t.TempDir()` `.harness/runtime/`. Each skill exercised end-to-end.
- **Golden:** `UseCaseVerdict` JSON, terminal `AggregateSignal` JSON, and `.harness/runtime/.../verdict.json` byte-identical across runs.
- **Race:** `go test -race` clean (parallelism in `/validate-use-case` and `/heal` re-validate makes this load-bearing).
- **Skill-file budget:** each `skills/<name>/skill.md` ≤200 lines, ≥100 line synthesis trigger (CLAUDE.md rule 4). Procedural detail goes into `scripts/main.go`.

## 13. Open follow-ups (post-implementation)

| # | Item | Notes |
|---|---|---|
| F-1 | `enums/termination-reasons.yaml` — add `skipped`? | Decision deferred (§3 follow-up F4). Revisit when CLI or heal loop needs to branch on dependency-failed sensors. |
| F-2 | Claude Code hook exact env-var contract | Implementation-time spike. Hook principle stable; field names need verification against current Claude Code version. |
| F-3 | `Sensor.ExpectedObservations` field | Confirm B4's `/create-sensors` populates this. If absent, B5's `/start-sensor` falls back to a CLI `--expect` flag. |
| F-4 | Per-skill compiled binary (vs `go run`) | Switch to checked-in binaries once skill interfaces freeze (post-Phase B). Cold-start improvement ~1s → ~10ms. |
| F-5 | `harness __internal` transport | When B6's CLI stabilizes, skills MAY route through `harness __internal <name>` instead of `go run`. Transparent to `skill.md`. |
