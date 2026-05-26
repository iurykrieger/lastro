# B6 — CLI (`cmd/harness`) (Design)

> Source chunk: [`docs/harness-framework/B6-cli.md`](../../harness-framework/B6-cli.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §8 (Repo layout), §9 Phase 6 (DX & ergonomics)
> Phase A entities consumed (read-only): E1 (enums), E4 (UseCase), E5 (Fixture), E6 (Sensor), E8 (AggregateSignal), E9 (ValidationPolicy)
> Phase B chunks consumed: B1 (per-use-case aggregator), B2 (lifecycle). B3 (heal loop) and B4 (skills) integrate as they land.
> Brainstorm date: 2026-05-25

## 1. Purpose

B6 delivers the user-facing `harness` binary at `cmd/harness/main.go` — a Cobra-based CLI that composes the deterministic runtime built in B1 + B2 (and B3 when it lands) into operator-friendly commands. It is the sibling surface to B5's slash commands; both wrap the same Go entry points.

Two subcommands ship in v1:

1. **`harness validate`** — synchronous validation of one or all use cases. Drives `internal/lifecycle.RunSensor` / `StartSensor` + `StopSensor` per sensor in the use case's DAG, then `internal/runtime/aggregator/usecase.Aggregate` for the per-use-case verdict. Pure determinism — no LLM involvement.
2. **`harness heal`** — *gated on B3 landing.* Consumes a failing per-sensor `AggregateSignal` (or scans `.harness/runtime/` for all failing aggregates), feeds it through `internal/runtime/healloop.Run` (B3), and re-validates only affected sensors.

A third subcommand — **`harness detect`** — is **deferred out of v1.** Detection is LLM-driven (B4); shipping it in the CLI binary requires an embedded Anthropic SDK + API key handling. v1 ships without it; the slash-command path in B4 covers the detect workflow during dogfood. A follow-up phase adds `harness detect` once CI demand for non-Claude-Code usage materializes (see §11 Future work).

B6 does **not** own:

- Runtime mechanics (B1 + B2 + B3 own those).
- Sensor generation or YAML persistence to `.harness/` (B4 owns those; the CLI only reads what B4 writes).
- The slash-command surface (B5).
- Examples / dogfood self-validation (B7).
- Per-sensor disk persistence — `internal/lifecycle` already writes `.harness/runtime/<sensor-id>/<run-id>/aggregate.json` + `signals.jsonl`. The CLI prints stdout JSON; it does not duplicate the layout.

## 2. Scope

**In:**

- New `cmd/harness/` package; `main` boots a Cobra root and dispatches to subcommand handlers.
- `cmd/harness/validate.go` — full `validate` subcommand.
- `cmd/harness/heal.go` — `heal` subcommand stub that returns `ErrUnimplemented` until B3 lands; full implementation added once `internal/runtime/healloop/` exists.
- `cmd/harness/skill.go` — **hidden** `__skill <name>` subcommand. Internal dispatch surface so B5's skill scripts can shell out to the same `harness` binary instead of re-wiring runtime. Not documented in user-facing `--help`.
- Output: a single JSON document (when `--output json`) or pretty-printed sections (when `--output text`, default). JSON document includes a run-metadata wrapper plus the verdict(s).
- Structured logging via `slog` (stdlib); `--quiet` silences info, `--verbose` enables debug. Logs go to stderr; stdout is reserved for the JSON / text output.
- Exit codes: 0 pass, 1 fail, 2 inconclusive. Documented in `--help`.
- Cross-surface integration test asserting `harness validate --output json` and `/validate-use-case` produce byte-identical JSON for the same inputs (deferred clause: only once B5's `/validate-use-case` lands).
- Tests covering acceptance criteria from the source chunk and the additional cases this design surfaces.

**Out:**

- `harness detect` subcommand (deferred — see §11).
- Detection result caching (deferred with detect).
- Reports / dashboards / file-output flags (`--output-file`). Stdout pipe + shell redirection covers v1.
- Multiplexed run history UI. `.harness/runtime/<sensor-id>/<run-id>/` directories accumulate; pruning is a future `harness clean` concern.
- Watch mode (`harness validate --watch`). Future.
- Per-step timeout override at the CLI level — `--timeout <duration>` is a sensor-wide wall clock, mirroring B2.
- New Anthropic SDK dependency. B6 stays free of LLM coupling.

## 3. Open questions resolved

The source chunk listed seven open questions. The brainstorm resolved each, plus surfaced and resolved several follow-ups:

| # | Question | Decision |
|---|---|---|
| 1 | Does `harness detect` exist? | **Not in v1.** Drop the subcommand from the initial release. Detection happens via B4 slash commands (`/detect-stack`, `/detect-use-cases`). The plan §9 Phase 6 line is split: "DX (validate + heal CLI now) → detect CLI later." See §11 Future work for the model-(a) embedded-LLM path that follows when CI usage justifies it. |
| 2 | CLI framework choice | **Cobra** (`github.com/spf13/cobra`). De facto Go standard, integrates cleanly with the existing `cmd/validate-schemas/` style, has built-in `--help` generation, supports hidden subcommands (for `__skill`). Added to `go.mod` in B6.1. |
| 3 | JSON output schema | **Wrapped with run metadata.** The top-level JSON object carries `{ "schema_version", "run_id", "command", "started_at", "ended_at", "duration_ms", "harness_version", "result": { ... } }` where `result` is either one `UseCaseVerdict` (single `--use-case`) or `{ "verdicts": [...], "summary": {...} }` (`--all`). CI tooling parses `started_at`/`duration_ms`; existing schemas stay unchanged. |
| 4 | Caching scope | **N/A for v1** (no `harness detect`). When detect lands, cache the LLM-produced YAML under `.harness/cache/<skill>/<content-hash>.yaml` keyed by source-tree content. Stack-manifest only by default; opt-in for use-cases + fixtures. Documented now to prevent layout drift later. |
| 5 | `--all` orchestration | **Bounded parallelism.** Use cases run in parallel bounded by `runtime.GOMAXPROCS()`; sensors within a single use case follow `internal/sensor.ResolveExecutionOrder` (matches B2's design). A `--concurrency <n>` flag overrides the GOMAXPROCS cap. Serial execution available via `--concurrency 1`. |
| 6 | Exit code semantics | **0 pass / 1 fail / 2 inconclusive**, standardized across `validate` and (future) `heal`. `harness heal` adds a third meaning for code 1 — "heal exhausted" — and code 2 — "heal abandoned" — documented in its `--help`. Errors that aren't validation outcomes (missing config, schema load failure) exit with code 64 (EX_USAGE) or 70 (EX_SOFTWARE) per `sysexits.h`. |
| 7 | Skill ↔ CLI parity test | **Single golden integration test** in `cmd/harness/parity_test.go`. Fixed sample repo + frozen `.harness/` inputs → run `harness validate --output json` and (when B5 lands) `/validate-use-case`; assert byte-identical JSON modulo the run-metadata wrapper (which differs per invocation by design). Gated behind a build tag until B5 lands so it doesn't block B6's merge. |
| 8 | Configuration source (surfaced) | **Flag-driven, no config file in v1.** `--policy <path>` overrides the discovered `.harness/validation-policy.yaml`; `--repo-root <path>` overrides cwd discovery. Future `.harnessrc` deferred until clear demand. Env-var fallback: `HARNESS_REPO_ROOT`, `HARNESS_POLICY`. Flags > env > default discovery. |
| 9 | Logging library (surfaced) | **`log/slog` (stdlib).** `slog.NewTextHandler` for text mode, `slog.NewJSONHandler` for JSON mode (auto-selected by `--output`). Logs go to stderr; data goes to stdout. `--quiet` raises the level to `Error`, `--verbose` lowers to `Debug`. No third-party logging dep. |
| 10 | Hidden internal subcommand (surfaced) | **`harness __skill <name> [args...]`.** A hidden Cobra subcommand that B5's slash-command scripts invoke instead of `go run`'ing per-skill binaries. The double underscore signals "internal — do not run by hand." Reuses the same runtime wiring as user-facing commands. Resolves B5's open question 1 in favor of "shared binary with hidden subcommands." |
| 11 | Heal target identifier (surfaced) | **`--sensor <sensor-id> [--run-id <run-id>]`** instead of B6-cli.md's `--signal <id>`. Per `internal/aggregate.AggregateSignal`, signals carry `(sensor_id, started_at, ended_at)` but no global signal id — runs are addressed by `(sensor-id, run-id)` on disk under `.harness/runtime/`. Without `--run-id`, target the latest failing run for that sensor. `--all-failing` walks the runtime tree. |
| 12 | CLI persistence (surfaced) | **Lifecycle owns disk; CLI owns stdout.** Per-sensor `aggregate.json` + `signals.jsonl` already land at `.harness/runtime/<sensor-id>/<run-id>/` via `internal/lifecycle` (B2). The CLI does not add a parallel persistence layer for the wrapper or for per-use-case verdicts; both are emitted on stdout only. CI consumers capture them from the pipe. Future `harness history` reads back from the runtime tree directly. |
| 13 | Use-case discovery (surfaced) | **`--use-case <id>` (repeatable) or `--all`.** Repeatable `--use-case` lets callers pin a subset (`harness validate --use-case create-order --use-case cancel-order`). `--all` enumerates `.harness/use-cases/*.yaml`. Conflicting flags error with code 64. No glob support in v1. |
| 14 | Cancellation (surfaced) | **`SIGINT`/`SIGTERM` propagation.** The root command installs a signal handler that cancels the root `context.Context`; lifecycle propagates the cancellation through `RunSensor` and (for observational runs) `StopSensor`. Exit code on Ctrl-C: 130 (SIGINT) / 143 (SIGTERM) per shell convention. Tests cover both paths via a fake signaler. |

## 4. Package layout

```
cmd/
└── harness/                              (created here; does not exist yet)
    ├── main.go                           root command boot, --version, signal handler install
    ├── root.go                           Cobra root: global flags (--output, --quiet, --verbose, --policy, --repo-root, --concurrency, --timeout)
    ├── config.go                         flag + env-var resolution, .harness/ discovery
    ├── output.go                         text and JSON renderers; selects based on --output
    ├── logger.go                         slog handler construction; quiet/verbose level mapping
    ├── exit.go                           exit-code enum + Cobra runE -> os.Exit bridge
    ├── validate.go                       validate subcommand: --use-case (repeatable), --all, --concurrency
    ├── validate_runner.go                orchestrates per-use-case lifecycle + aggregator/usecase.Aggregate
    ├── heal.go                           heal subcommand: --sensor, --run-id, --all-failing (returns ErrUnimplemented until B3 lands)
    ├── heal_runner.go                    drives healloop.Run + selective re-validate; lit up when internal/runtime/healloop/ exists
    ├── skill.go                          hidden __skill subcommand: dispatches to internal handlers used by B5 scripts
    ├── parity_test.go                    cross-surface byte-identical-JSON assertion (build-tag-gated until B5 lands)
    └── *_test.go
```

**Dependencies:**

| Package | Imports (production) |
|---|---|
| `cmd/harness` | `internal/lifecycle`, `internal/runtime/aggregator/usecase`, `internal/runtime/executor`, `internal/sensor`, `internal/usecase`, `internal/fixture`, `internal/policy`, `internal/usecase/template`, `internal/enums`, `internal/aggregate`, `github.com/spf13/cobra`, stdlib `log/slog`, `context`, `os/signal` |
| `cmd/harness` (post-B3) | adds `internal/runtime/healloop` |

`cmd/harness` is a leaf package — no `internal/` code imports it. It depends only one-way on the runtime tree. No cyclic dependencies.

The `go.mod` gains exactly one new direct dependency: `github.com/spf13/cobra`. Cobra's transitive deps (`spf13/pflag`, `inconshreveable/mousetrap`) come along automatically.

## 5. Subcommands

### 5.1 `harness validate`

**Synopsis:**

```
harness validate (--use-case <id>... | --all) [flags]
```

**Flags (subcommand-specific):**

| Flag | Type | Default | Purpose |
|---|---|---|---|
| `--use-case <id>` | string, repeatable | — | Validate the named use case. Conflicts with `--all`. |
| `--all` | bool | false | Validate every use case under `.harness/use-cases/`. Conflicts with `--use-case`. |
| `--concurrency <n>` | int | `runtime.GOMAXPROCS()` | Max use cases evaluated in parallel. Sensors within a use case follow the DAG regardless. |

**Behavior:**

1. **Discovery.** Load `.harness/use-cases/*.yaml`, `.harness/fixtures/*.yaml`, `.harness/sensors/*.yaml`, `.harness/stack-manifest.yaml`, `.harness/validation-policy.yaml` (or path from `--policy`) via existing Phase A loaders. Error code 64 if any required file is missing.
2. **Use-case selection.** `--all` enumerates the use-case store; `--use-case` filters by ID. Each requested ID must exist; missing IDs error with code 64.
3. **Per-use-case execution** (parallel up to `--concurrency`):
   a. Topo-sort sensors via `sensor.ResolveExecutionOrder`, grouped into wavefront layers — sensors with no unsatisfied `depends_on` form layer 0, sensors depending only on layer 0 form layer 1, etc.
   b. For each layer, run all member sensors **in parallel** via a `sync.WaitGroup`; advance to the next layer only after the current layer completes. Per-sensor execution: `lifecycle.RunSensor` (assertion) or `lifecycle.StartSensor` + `lifecycle.StopSensor` (observational). Both return `aggregate.AggregateSignal`. This matches B2's recommendation ("parallel where the DAG allows; serial within a chain"); B2 punted multi-sensor orchestration to the caller, and `cmd/harness/validate_runner.go` is that caller.
   c. Feed the slice of `AggregateSignal`s + the resolved `policy.EffectivePolicy` into `aggregator.UseCase` → `UseCaseVerdict`.
4. **Aggregate exit code.**
   - All verdicts `pass` → exit 0.
   - Any verdict `fail` → exit 1.
   - Otherwise (any `inconclusive`, no `fail`) → exit 2.
5. **Output.** Render via §6.

**Error semantics:**

- Sensor execution errors (lifecycle returns non-nil error) bubble up as `inconclusive` for that use case, with `--verbose` logging the stack. Do not crash the whole `--all` run for one bad sensor.
- Schema load failure / missing YAML → exit 64 (no output JSON; logs go to stderr).
- Unknown flag combination → exit 64.

### 5.2 `harness heal` (gated on B3)

**Synopsis:**

```
harness heal (--sensor <sensor-id> [--run-id <run-id>] | --all-failing) [flags]
```

**Flags (subcommand-specific):**

| Flag | Type | Default | Purpose |
|---|---|---|---|
| `--sensor <sensor-id>` | string | — | Heal the latest failing run for this sensor. Conflicts with `--all-failing`. |
| `--run-id <run-id>` | string | latest | Pin a specific run id under `<sensor-id>/`. Only valid with `--sensor`. |
| `--all-failing` | bool | false | Scan `.harness/runtime/*/*/aggregate.json`, heal each `verdict=fail` aggregate. |
| `--max-iterations <n>` | int | (see precedence below) | Per-sensor heal-loop iteration cap. Precedence: CLI flag > `EffectivePolicy.HealMaxIterations` (if exposed by B3's policy extension) > hard default 3 (plan §10.4). |

**Behavior (post-B3):**

1. **Target discovery.**
   - `--sensor`: load `<runtime-root>/<sensor-id>/<run-id>/aggregate.json` (or latest by mtime); error 64 if not found.
   - `--all-failing`: walk `.harness/runtime/*/*/aggregate.json`, filter `verdict=fail`, sort by `ended_at` descending.
2. **Per-target heal loop** (serial, per B3 design):
   - Call `healloop.Run(failingAggregate, llmClient, healloop.Config{MaxIterations})`.
   - `llmClient` is the same Anthropic-API shim used by B5's `/heal` skill — `cmd/harness` constructs it once at boot. (If/when detect lands in the CLI, the same shim is reused.)
   - Selective re-validation happens inside `healloop.Run` via the lifecycle.
3. **Aggregate exit code.**
   - All targets `healed` → exit 0.
   - Any `exhausted` → exit 1.
   - Any `abandoned` (LLM refused / unparseable edit) → exit 2.

**Until B3 lands:** the subcommand exists in `cmd/harness/heal.go` but returns `cobra.ErrSubCommandRequired`-equivalent message: `"harness heal is gated on B3 (internal/runtime/healloop). Track docs/harness-framework/B3-heal-loop.md."` Exit code 70.

### 5.3 `harness __skill` (hidden)

**Synopsis:**

```
harness __skill <skill-name> [args...]
```

**Behavior:**

- Hidden Cobra subcommand. Not listed in user-facing `--help` (Cobra `Hidden: true`).
- Dispatches to internal handlers — one per B5 skill — that share the same runtime wiring as `validate` / `heal`. Initial set:
  - `harness __skill run-sensor <sensor-id>` — wraps `lifecycle.RunSensor`.
  - `harness __skill start-sensor <sensor-id>` — wraps `lifecycle.StartSensor`, prints the JSON handle.
  - `harness __skill stop-sensor <sensor-id> <run-id>` — wraps `lifecycle.StopSensor` (looks up handle via the registry).
  - `harness __skill validate-use-case <id>` — identical to `harness validate --use-case <id>` but with skill-flavored output for B5's scripts.
  - `harness __skill heal <sensor-id> [<run-id>]` — wraps `healloop.Run` (post-B3).
- B5's slash-command `scripts/` directories invoke `harness __skill <name>` instead of `go run`'ing per-skill binaries. Resolves B5's open question 1 in favor of (a) "shared binary with hidden subcommands."
- The skill surface (`skills/<name>/skill.md`) describes user-facing semantics; the `__skill` subcommand provides the deterministic Go path it shells into.

## 6. Output

### 6.1 Text mode (`--output text`, default)

Human-readable summary. Example for `harness validate --use-case create-order`:

```
✓ create-order  (pass, confidence 0.94, 3 sensors, 1.2s)
  ✓ build           pass   confidence 1.00   245ms
  ✓ unit-test       pass   confidence 1.00   612ms
  ✓ e2e-test        pass   confidence 0.83   343ms
```

Failure example:

```
✗ create-order  (fail, confidence 0.62, 3 sensors, 1.4s)
  ✓ build           pass   confidence 1.00   232ms
  ✗ unit-test       fail   confidence 1.00   598ms
    70 of 700 unit tests failed
    suggested_locus: src/handlers/orders.ts:createOrder
  ⚠ e2e-test        inconclusive  confidence 0.55  571ms
    LLM verdict below floor (0.7)
```

Glyph mapping: `✓` pass, `✗` fail, `⚠` inconclusive. ASCII fallback (`[OK]`/`[FAIL]`/`[WARN]`) when `--no-color` or `NO_COLOR` env var is set.

### 6.2 JSON mode (`--output json`)

Single JSON document on stdout. Stable schema; versioned via `cli_schema_version`.

```json
{
  "cli_schema_version": "1.0.0",
  "run_id": "01J...ULID...",
  "command": "validate",
  "args": ["--use-case", "create-order"],
  "started_at": "2026-05-25T12:34:56.789Z",
  "ended_at":   "2026-05-25T12:34:58.012Z",
  "duration_ms": 1223,
  "harness_version": "0.1.0",
  "result": {
    "verdicts": [
      {
        "use_case_id": "create-order",
        "archetype": "http-api",
        "verdict": "pass",
        "confidence": 0.94,
        "obligatory_satisfied": true,
        "evaluated_angles": ["build", "unit-test", "e2e-test"],
        "failing_angles": [],
        "warning_angles": [],
        "heal_hints": [],
        "sensors": [
          {
            "sensor_id": "create-order--build",
            "angle": "build",
            "verdict": "pass",
            "confidence": 1.0,
            "started_at": "2026-05-25T12:34:56.790Z",
            "ended_at":   "2026-05-25T12:34:57.035Z",
            "rollup": { "total_signals": 1, "pass_count": 1, "fail_count": 0, "warn_count": 0, "inconclusive_count": 0 },
            "runtime_path": ".harness/runtime/create-order--build/01J.../"
          }
        ]
      }
    ],
    "summary": {
      "total_use_cases": 1,
      "pass_count": 1,
      "fail_count": 0,
      "inconclusive_count": 0
    }
  }
}
```

**Notes:**

- `result.verdicts[].sensors[]` is a flattened view of the AggregateSignals consumed by the use-case aggregator. Each entry includes `runtime_path` so a CI tool can read the full `aggregate.json` + `signals.jsonl` if it needs more detail.
- For `harness heal`, the result shape switches to `{ "targets": [{ "sensor_id", "run_id", "status", "iterations_used", "edits_applied", "final_verdict" }] }` — defined fully in B6.3 (post-B3) sub-spec.
- For single `--use-case`, `result` is still wrapped in `verdicts` (array of one) for caller uniformity.

### 6.3 Quiet / verbose

- `--quiet`: suppress stderr logs at `Info`; still emit `Error`. Stdout output unchanged.
- `--verbose`: emit `Debug` to stderr — includes per-step durations, sensor DAG order, fixture-binding resolution. Stdout output unchanged.

## 7. Logging

Logger is constructed once in `cmd/harness/logger.go`:

```go
func newLogger(format string, quiet, verbose bool) *slog.Logger {
    level := slog.LevelInfo
    if quiet { level = slog.LevelError }
    if verbose { level = slog.LevelDebug }
    var h slog.Handler
    switch format {
    case "json": h = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
    default:     h = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
    }
    return slog.New(h)
}
```

Conventions:

- Logs always go to stderr; stdout is reserved for the run JSON or text output.
- One log line per use-case start / end (`Info`), per sensor start / end (`Debug`), per fixture-binding error (`Error`).
- Sensitive fields (fixture payloads in the binder) are redacted at the log handler; the binder already logs IDs, not payloads.

## 8. CLI ↔ skill parity

**Acceptance criterion (deferred until B5 lands):**

> Same `.harness/` inputs → `harness validate --output json` and `/validate-use-case` produce byte-identical JSON in `result`, modulo the run-metadata wrapper.

Test fixture in `cmd/harness/testdata/parity/` — a frozen mini-`.harness/` tree plus a recorded LLM transcript stub. The test:

1. Runs `harness validate --use-case <id> --output json`.
2. Parses the JSON, strips the wrapper, retains `result`.
3. Invokes the equivalent skill via `harness __skill validate-use-case <id>` (which is what B5's script shells into).
4. Asserts `result` is byte-identical.

Build-tag-gated (`//go:build parity`) until B5 lands so B6's CI is not blocked.

## 9. Cobra wiring

Root command in `cmd/harness/root.go`:

```go
func newRootCmd(ctx context.Context, cfg *Config) *cobra.Command {
    root := &cobra.Command{
        Use:   "harness",
        Short: "Use-case-driven validation framework",
        SilenceUsage: true,   // don't dump usage on every error
        SilenceErrors: true,  // we handle exit-code mapping in main.go
    }
    root.PersistentFlags().StringVar(&cfg.Output, "output", "text", "text | json")
    root.PersistentFlags().BoolVar(&cfg.Quiet, "quiet", false, "suppress info logs")
    root.PersistentFlags().BoolVar(&cfg.Verbose, "verbose", false, "emit debug logs")
    root.PersistentFlags().StringVar(&cfg.Policy, "policy", "", "path to validation policy (default: .harness/validation-policy.yaml)")
    root.PersistentFlags().StringVar(&cfg.RepoRoot, "repo-root", "", "repo root (default: cwd)")
    root.PersistentFlags().IntVar(&cfg.Concurrency, "concurrency", 0, "max parallel use cases (0 = GOMAXPROCS)")
    root.PersistentFlags().DurationVar(&cfg.Timeout, "timeout", 0, "wall-clock cap per sensor (0 = no cap)")

    root.AddCommand(newValidateCmd(ctx, cfg))
    root.AddCommand(newHealCmd(ctx, cfg))
    root.AddCommand(newSkillCmd(ctx, cfg))   // Hidden: true
    root.AddCommand(newVersionCmd())          // prints harness_version
    return root
}
```

`main.go`:

```go
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()
    cfg := &Config{}
    root := newRootCmd(ctx, cfg)
    if err := root.ExecuteContext(ctx); err != nil {
        os.Exit(exitCodeFor(err, ctx))
    }
}
```

`exitCodeFor` (in `cmd/harness/exit.go`) maps known sentinel errors (`ErrUseCaseFail`, `ErrUseCaseInconclusive`, `ErrHealExhausted`, `ErrHealAbandoned`, `context.Canceled`, etc.) to documented exit codes. Unknown errors → 70.

## 10. Phasing

B6 lands in four sub-PRs against `feat/b6-cli`:

| Phase | What lands | Unblocked when |
|---|---|---|
| **B6.1** | `cmd/harness/{main,root,config,logger,output,exit}.go`, `--version`, Cobra wiring with stub subcommands. Adds Cobra to `go.mod`. Tests: root-flag parsing, exit-code mapping. | Now (Phase A + B1 + B2 already merged). |
| **B6.2** | `cmd/harness/validate.go` + `validate_runner.go`. Full `harness validate` against existing `internal/lifecycle` + `internal/runtime/aggregator/usecase`. Text + JSON output. Tests cover the deliverable acceptance bullets. | After B6.1. |
| **B6.3** | `cmd/harness/heal.go` + `heal_runner.go`. Full `harness heal`. Tests cover `--sensor`, `--all-failing`, iteration cap, `healed`/`exhausted`/`abandoned`. | After B3 (`internal/runtime/healloop/`) lands. |
| **B6.4** | `cmd/harness/skill.go`. Hidden `__skill` subcommand for each B5 skill. Parity test (`cmd/harness/parity_test.go`) under build tag. | After B5 (or in parallel with B5; B5 consumes B6.4 via `harness __skill`). |

B6.1 + B6.2 are the v1 ship. B6.3 follows B3. B6.4 follows B5.

## 11. Future work (not in v1)

**`harness detect` subcommand (model (a) — embedded LLM client).**

When CI usage demands non-Claude-Code-driven detection, add `harness detect` with:

- A new `internal/llmclient/` package abstracting the Anthropic SDK (`github.com/anthropics/anthropic-sdk-go`). API key resolution: `ANTHROPIC_API_KEY` env > `--api-key` flag > error.
- Prompt body loaded from `skills/detect-*/skill.md` (shared with B5).
- Same validate-then-persist path as B4's slash-command scripts: emit YAML, validate via Phase A loaders, write to `.harness/`.
- Detection result caching at `.harness/cache/<skill>/<content-hash>.yaml` keyed by relevant source-tree hash.
- Flag set: `--cache` (use cache if present), `--no-cache` (force re-detect), `--api-key <key>` override.

Gated behind a build tag (`//go:build detect`) so the default `harness` binary stays SDK-free until detect lands.

**`harness clean`.**

Prune `.harness/runtime/<sensor-id>/<run-id>/` directories older than a configurable age. Useful for long-lived CI workspaces.

**`harness history <use-case-id>`.**

Browse past runs by reading the runtime tree directly.

**`.harnessrc` config file.**

Persist `--policy` / `--repo-root` / `--concurrency` defaults per project. Deferred until clear demand.

## 12. Testing strategy

| Test | Layer | Scope |
|---|---|---|
| `cmd/harness/root_test.go` | unit | Cobra flag parsing, env-var fallback, conflicting-flag detection (exit 64). |
| `cmd/harness/exit_test.go` | unit | Sentinel-error → exit-code mapping. |
| `cmd/harness/output_test.go` | unit | JSON renderer determinism (sorted keys, stable timestamps via `Now` injection); text renderer glyph mapping + `NO_COLOR`. |
| `cmd/harness/validate_test.go` | integration | Drive a fake `lifecycle.Lifecycle` + fake aggregator. Assert: all-pass → exit 0; one fail → exit 1; inconclusive only → exit 2; per-sensor crash bubbles as inconclusive at the use-case level, not whole-run abort. |
| `cmd/harness/concurrency_test.go` | integration | `--concurrency 1` serializes; default unbounded saturates GOMAXPROCS; `--concurrency 4` caps at 4 in-flight. |
| `cmd/harness/cancellation_test.go` | integration | SIGINT mid-validate cancels lifecycle, returns exit 130; SIGTERM returns 143. |
| `cmd/harness/heal_test.go` | integration (post-B3) | Drive a fake `healloop.LLMClient`. Assert healed/exhausted/abandoned semantics. |
| `cmd/harness/parity_test.go` | integration (post-B5, build tag) | Byte-identical JSON across CLI and skill paths for the same `.harness/` inputs. |
| `cmd/harness/golden/*.json` | golden | Frozen JSON output for stable input, regenerated by `go test -update`. Covers single-use-case pass, single-use-case fail, `--all` mixed, heal stub error. |

All tests run with `-race`. Integration tests use a fake lifecycle that returns pre-recorded `AggregateSignal`s — no real subprocess spawning at the `cmd/harness/` level. Real-subprocess coverage stays in `internal/runtime/executor/` and `internal/lifecycle/`.

## 13. Deliverable acceptance (from source chunk, mapped)

| Source chunk criterion | How this design satisfies it | Test |
|---|---|---|
| `harness --help` and `harness <subcommand> --help` produce sensible output. | Cobra auto-generation; subcommand `Short` + `Long` strings written in §5. | manual + `cmd/harness/help_test.go` snapshot. |
| `harness validate --all` on a passing sample exits 0; broken sample exits 1 with failure summary listing failing sensors. | §5.1 behavior + text/JSON renderers in §6. | `cmd/harness/validate_test.go` + golden tests + B7 integration. |
| `harness heal --all-failing` on broken sample applies LLM fix, re-validates, exits 0. | §5.2, gated on B3. | `cmd/harness/heal_test.go` (post-B3) + B7. |
| `harness validate --output json` produces parseable JSON matching a documented schema. | §6.2 schema. | `cmd/harness/output_test.go` golden. |
| Cross-surface integration test: CLI and skill produce identical JSON. | §8. | `cmd/harness/parity_test.go` (post-B5). |
| `harness detect` (if model (a)) on a sample drives the LLM → valid `.harness/` artifacts → exits 0. | Deferred to future work §11. | Out of v1. |

## 14. Non-goals

- Watch mode, daemon mode, file-watching.
- HTTP / gRPC server for remote validation.
- Dashboard / TUI.
- Multi-repo orchestration (the CLI operates on one repo root).
- Per-sensor disk persistence at the CLI layer — lifecycle owns it.
- Anthropic SDK / LLM coupling in v1.
- `.harnessrc` config file.

## 15. Risks

- **Cobra version drift.** Cobra 1.x has been stable for years, but pin a specific minor in `go.mod` (`v1.8.x` at time of writing) to avoid surprise CLI-syntax changes. Mitigation: explicit `go.mod` pin + `go.sum` review on dep updates.
- **JSON schema drift between CLI and skill output.** Mitigated by the parity test in §8 once B5 lands. Until then, the CLI's `cli_schema_version` field documents the wire-format contract for any CI consumer.
- **Hidden `__skill` surface becoming a load-bearing public API by accident.** Documented in `cmd/harness/skill.go` as internal-only; not surfaced in `--help`; B5's scripts are the only callers.
- **`--concurrency` interacting badly with observational sensors.** Observational sensors run on independent goroutines inside `lifecycle.StartSensor`; the concurrency cap applies only to use-case-level parallelism. Document this clearly in `--help` to avoid the user expecting per-sensor parallelism control here.
- **B3 / B5 schedule slip.** B6.3 and B6.4 are gated; B6.1 + B6.2 ship independently. If B3/B5 slip, the v1 CLI (`validate` only) still delivers value.

## 16. Decisions still open for review

These were flagged inline above but warrant explicit user attention before B6.1 starts:

1. **Decision (#1) — drop `harness detect` from v1.** This deviates from plan §9 Phase 6 wording. Confirm: ship `validate` + `heal` first, add `detect` as a follow-up phase tied to CI demand?
2. **Decision (#3) — wrapped JSON schema with `cli_schema_version`.** Confirm: a stable CLI-output schema versioned independently of the entity schemas is acceptable, vs. matching `UseCaseVerdict` exactly?
3. **Decision (#10) — hidden `__skill` subcommand.** Confirm: B5's scripts shell into the same `harness` binary (option (a) from B5's open question 1), and B6 lands the dispatch surface as B6.4?

If any of these need to flip, revise the spec before writing the implementation plan.
