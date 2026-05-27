# B7 — Examples & Dogfood Self-Validation (Design)

> Source chunk: [`docs/harness-framework/B7-examples-dogfood.md`](../../harness-framework/B7-examples-dogfood.md)
> Plan references: [`plan.md`](../../harness-framework/plan.md) §11 (Acceptance Criteria), `CLAUDE.md` rule 7 (dogfood the framework)
> Phase A entities consumed (read-only): E1 (enums), E2 (StackComponent), E3 (EntryPoint), E4 (UseCase), E5 (Fixture), E6 (Sensor), E7 (Signal), E8 (AggregateSignal), E9 (ValidationPolicy)
> Phase B chunks consumed: B1 (composed runtime), B2 (executor + lifecycle), B3 (heal loop runtime), B5 (skill wrappers — `/validate-use-case`, `/heal`), B6 (CLI is not invoked from B7 tests; sibling surface only)
> Brainstorm date: 2026-05-27

## 1. Purpose

B7 closes Phase B by delivering the integration evidence that the framework actually works. It ships **two independent tracks** that share one small Go primitive:

1. **Track 1 — Synthetic-consumer integration tests.** Three sample subject repos under `examples/` (a passing HTTP API, its deliberately-broken twin, a minimal CLI). Go integration tests drive the framework's skill surface against each sample and assert plan §11's seven acceptance criteria. The framework is the system-under-test; the samples are mocks crafted to exercise each criterion.

2. **Track 2 — Dogfood self-validation gate.** A `.harness/` directory is produced manually (by running the detection skills against the framework root once) and committed to the repo. A `-tags=dogfood` Go test then runs `/validate-use-case` once per detected use case and fails CI if any verdict is not `pass`. No plan §11 mapping here — it's a regression gate that fires whenever a framework change breaks a sensor the framework itself uses.

Both tracks invoke the framework through the **skill surface** (`/validate-use-case`, `/heal` — built once per test process, then shelled out to via `exec.Command`). The CLI surface (`harness validate`, `harness heal`) is independently verified by `cmd/harness/*_test.go` and is not exercised here. This is deliberate: end users invoke skills, so dogfood must invoke skills.

B7 does **not** own:

- The CLI heal command (`cmd/harness/heal.go` remains gated on a future unblock; not B7's scope).
- LLM-driven detection runs in CI (detection is manual during B7 implementation, output committed).
- Performance benchmarks, run-history pruning, watch mode (out of scope per the B7 chunk doc).
- Additional sample archetypes beyond `http-api` and `cli` (later).

## 2. Scope

**In:**

- `examples/http-api-sample/` — minimal Go HTTP API (`GET /orders/:id`, `POST /orders` with body validation). Each sample is its own Go module.
- `examples/http-api-sample-broken/` — sibling sample identical to the passing twin except `handlers.go` omits the 400-validation branch in `CreateOrder`. Ships a committed `heal-fixture/editplan.json` carrying the EditPlan that fixes the bug.
- `examples/cli-sample/` — minimal Go Cobra CLI with one subcommand. Proves archetype branching in detection produces different downstream sensors.
- `examples/validator/` — a regular Go package (no build tags) exporting the `ValidateAll(ctx, target, skills)` primitive and a `Report` type. Both test tracks consume it.
- `examples/integration_test.go` + `examples/heal_test.go` (build tag `integration`) — Track 1, plan §11 criteria 1-5 and 7 (integration_test.go) + criterion 6 (heal_test.go).
- `examples/dogfood_test.go` (build tag `dogfood`) — Track 2, single test function.
- Framework's own `.harness/` at the repo root, produced manually and committed.
- `.gitignore` updates for `.harness/runtime/` and `.harness/reports/`.

**Out:**

- LLM invocations from test code. Detection is manual, run during B7 implementation, output committed.
- `harness heal` CLI wiring (stays gated; out of scope per B6 design).
- Makefile or shell-script orchestration (the user's "go test with build tags only" answer rules these out).
- Recorded LLM transcripts / replay infrastructure (the pre-committed `.harness/` fixtures strategy makes recording unnecessary).
- Cross-surface byte-identity assertions between `harness validate` and `/validate-use-case` (deferred from B6).

## 3. Architecture

```
                                  ┌────────────────────────────────────────┐
                                  │  shared primitive (no build tag):      │
                                  │  examples/validator/                   │
                                  │  ValidateAll(ctx, target, skills)      │
                                  │    enumerate target/.harness/use-cases │
                                  │    for each → exec.Command(skills      │
                                  │              .ValidateUseCase, ucID)   │
                                  │              with cmd.Dir = target     │
                                  │    parse persisted verdict envelope    │
                                  │    return *Report (+write report.json) │
                                  └─────────┬──────────────┬───────────────┘
                                            │              │
                                            ▼              ▼
       ┌───────────────────────────────────────┐    ┌──────────────────────────────┐
       │  Track 1 — Synthetic-consumer tests   │    │  Track 2 — Dogfood gate      │
       │  examples/integration_test.go         │    │  examples/dogfood_test.go    │
       │  examples/heal_test.go                │    │  -tags=dogfood               │
       │  -tags=integration                    │    │                              │
       │                                       │    │  Subject = repo root         │
       │  Subjects = three samples             │    │  Asserts: every detected     │
       │  Asserts: plan §11 criteria 1-7       │    │    framework use case passes │
       │                                       │    │                              │
       │  Framework = SYSTEM-UNDER-TEST        │    │  Framework = CONSUMER        │
       └───────────────────────────────────────┘    └──────────────────────────────┘
```

Three principles enforced throughout:

1. **Skills, not CLI.** All validation/heal invocations shell to `skills/<name>` binaries built once per test process. `harness validate` and `harness heal` are not invoked from B7 tests.
2. **No live LLM.** `ValidateAll` only invokes `/validate-use-case`. The detection skills (`/detect-stack`, `/detect-use-cases`, `/create-sensors`) are never called from test code.
3. **One primitive, two scopes.** `ValidateAll` enumerates use cases and runs `/validate-use-case` per one. Called from both tracks against different `target` paths — no parallel implementation.

## 4. Repository layout

```
lastro/
├── go.mod                             # existing main module
├── .gitignore                         # NEW lines: .harness/runtime/, .harness/reports/
├── .harness/                          # NEW — framework's own artifacts (dogfood subject)
│   ├── stack-manifest.yaml
│   ├── use-cases/*.yaml
│   ├── fixtures/*.yaml
│   ├── sensors/*.yaml
│   └── reports/                       # gitignored; written per dogfood run
├── cmd/harness/                       # existing
├── internal/                          # existing
├── lib/                               # existing
├── skills/                            # existing (incl. /heal, /validate-use-case)
└── examples/                          # NEW
    ├── README.md                      # what each sample shows, how to run tests
    ├── validator/                     # shared primitive (regular Go package)
    │   ├── validator.go
    │   ├── validator_test.go          # unit tests with fakes (no build tag)
    │   ├── report.go                  # Report / UseCaseResult / Summary types
    │   ├── skill_binaries.go          # NewSkillBinaries: pre-builds skill binaries
    │   └── copydir.go                 # copyDir helper for the heal test
    ├── integration_test.go            # Track 1, criteria 1-5, 7
    ├── heal_test.go                   # Track 1, criterion 6
    ├── dogfood_test.go                # Track 2
    ├── http-api-sample/               # own Go module
    │   ├── go.mod                     # module example.com/http-api-sample
    │   ├── main.go
    │   ├── handlers.go                # 1 GET + 1 POST with body validation
    │   ├── store.go                   # in-memory order store
    │   ├── README.md
    │   └── .harness/                  # hand-curated, committed
    │       ├── stack-manifest.yaml
    │       ├── use-cases/{uc-get-order,uc-create-order-success,uc-create-order-bad-input}.yaml
    │       ├── fixtures/{existing_order_id,valid_order_payload,invalid_order_payload}.yaml
    │       └── sensors/<6 sensors: 3 use cases × 2 angles>.yaml
    ├── http-api-sample-broken/        # own Go module
    │   ├── go.mod                     # module example.com/http-api-sample-broken
    │   ├── main.go
    │   ├── handlers.go                # missing 400 branch in CreateOrder
    │   ├── store.go
    │   ├── README.md
    │   ├── .harness/                  # byte-identical to passing twin
    │   └── heal-fixture/
    │       └── editplan.json          # hand-supplied EditPlan that heals the bug
    └── cli-sample/                    # own Go module
        ├── go.mod                     # module example.com/cli-sample
        ├── main.go                    # one Cobra subcommand: `greet --name X`
        ├── README.md
        └── .harness/
```

Six layout decisions:

1. **Framework `.harness/` at repo root.** Same path/shape a consumer checks in. Produced manually via the inferential skills during implementation, then committed.
2. **Each sample is its own Go module.** Real-world fidelity; the main module's `go test ./...` skips nested modules, so samples don't leak deps into the framework.
3. **`examples/validator/` is a normal Go package** with no build tags. Both test tracks import it; it has its own unit tests with skill-invocation fakes.
4. **Reports written to `<subject>/.harness/reports/<run-id>/report.json`** for both samples and dogfood. Gitignored.
5. **`heal-fixture/editplan.json` lives in the broken sample**, not in test code. The test reads it from disk and pipes to `/heal` — the bug and the fix stay co-located.
6. **No Makefile.** CI calls `go test` directly with build tags.

## 5. Per-sample anatomy

### 5.1 `examples/http-api-sample/` (canonical passing sample)

**Source (~80 LoC Go total):**

| File | Contents |
|---|---|
| `main.go` | `func main()` starts HTTP server on `:8080`, registers handlers |
| `handlers.go` | `GetOrder`, `CreateOrder` with body validation; `OrderInput` struct |
| `store.go` | in-memory `map[string]Order` with `Create` + `Get` |
| `go.mod` | `module example.com/http-api-sample`; deps: `net/http` (stdlib) only |

Two endpoints:
- `GET /orders/{id}` → 200 + JSON body for known ids; 404 otherwise.
- `POST /orders` → 201 for valid body; 400 for missing required field `item`.

**`.harness/` contents:**

| Artifact | Count | Description |
|---|---|---|
| `stack-manifest.yaml` | 1 | `archetype: http-api`; components: `go-stdlib`, `net/http`; non-empty `rationale` prose |
| `use-cases/uc-get-order.yaml` | 1 | given existing `{{fixtures.existing_order_id}}`, when `GET {{entry_points.get_order}}`, then 200 + body |
| `use-cases/uc-create-order-success.yaml` | 1 | given `{{fixtures.valid_order_payload}}`, when `POST {{entry_points.create_order}}`, then 201 + id |
| `use-cases/uc-create-order-bad-input.yaml` | 1 | given `{{fixtures.invalid_order_payload}}`, when `POST {{entry_points.create_order}}`, then 400 + error |
| `fixtures/*.yaml` | 3 | `existing_order_id`, `valid_order_payload`, `invalid_order_payload` |
| `sensors/*.yaml` | 6 | 2 angles (`e2e-test`, `unit-test`) × 3 use cases |

**Fixture-reuse demonstration (criterion §11.7):** `valid_order_payload` appears in step-level `uses:` of both `uc-create-order-success-e2e-test` and `uc-create-order-success-unit-test`. Criterion 7 walks sensor YAMLs and asserts this intersection.

### 5.2 `examples/http-api-sample-broken/`

Identical to the passing twin except:

- `handlers.go` — the `CreateOrder` validation branch returning 400 is **omitted**; invalid input falls through to the 201 path.
- `.harness/` is **byte-identical** to the passing sample's. (Use cases describe what the API *should* do; the bug is in the implementation, not the contract.)
- New file: `heal-fixture/editplan.json` — a hand-supplied `EditPlan` whose single `write` op replaces `handlers.go` with the corrected version.

**Expected behavior:**

- `ValidateAll(brokenSample)` returns 2 passing + 1 failing aggregate. The failing one is `uc-create-order-bad-input-e2e-test`, carrying a non-empty `heal_hint` synthesized by `internal/aggregate.synthesizeHealHint`.
- After applying `heal-fixture/editplan.json` via `/heal`, re-validation returns all-pass on iteration 1.

### 5.3 `examples/cli-sample/`

The archetype-`cli` subject. Proves detection produces archetype-appropriate sensors.

| File | Contents |
|---|---|
| `main.go` | One Cobra subcommand: `greet --name <s>` → prints `Hello, <s>` to stdout, exit 0 |
| `go.mod` | `module example.com/cli-sample`; dep: `github.com/spf13/cobra` |

- `stack-manifest.yaml` → `archetype: cli`; components: `go-stdlib`, `github.com/spf13/cobra`.
- One use case: `uc-greet-by-name` — given `{{fixtures.name}}`, when `cli-sample greet --name {{fixtures.name}}`, then stdout contains `"Hello, "` and exit=0.
- One fixture, two sensors (`e2e-test` + `unit-test`) — demonstrates fixture reuse for this sample too.

### 5.4 Framework `.harness/` (dogfood subject)

**Produced by detection during B7 implementation, then committed.** This spec does not enumerate the use cases — they're an output of `/detect-stack`, `/detect-use-cases`, `/create-sensors` run against the framework root. The implementation plan will include the manual procedure:

1. Run the three inferential skills against `lastro/` from a fresh checkout.
2. Hand-review the produced `.harness/` for sanity (archetype=`cli`; sensible use cases describing `harness` CLI flows, the skill scripts, runtime behaviors; sensors grounded against detected stack).
3. Commit `.harness/` to the repo.
4. `dogfood_test.go` then validates against it.

If detection produces something unusable, that's a B4 quality bug — fix in B4, regenerate, retry. Once committed, the dogfood test is the regression gate going forward.

## 6. Test driver

### 6.1 `examples/validator/` API

```go
package validator

// SkillBinaries holds absolute paths to pre-built skill binaries.
// Tests construct one in TestMain via NewSkillBinaries.
type SkillBinaries struct {
    ValidateUseCase string // path to the built /validate-use-case binary
    Heal            string // path to the built /heal binary
}

// NewSkillBinaries builds both skill binaries into workDir.
// Caller provides workDir (typically t.TempDir() from TestMain) and
// frameworkRoot (the lastro module root, where ./skills/... resolves).
// Built once per process; tests share the result.
func NewSkillBinaries(workDir, frameworkRoot string) (*SkillBinaries, error)

// Report is the structured artifact written to <target>/.harness/reports/<run-id>/report.json.
type Report struct {
    SchemaVersion int             `json:"schema_version"` // 1
    RunID         string          `json:"run_id"`         // ULID
    Target        string          `json:"target"`         // absolute path validated
    StartedAt     time.Time       `json:"started_at"`
    EndedAt       time.Time       `json:"ended_at"`
    UseCases      []UseCaseResult `json:"use_cases"`
    Summary       Summary         `json:"summary"`
}

type UseCaseResult struct {
    UseCaseID  string              `json:"use_case_id"`
    Verdict    string              `json:"verdict"` // pass | fail | inconclusive
    SensorRuns []SensorRunSummary  `json:"sensor_runs"`
    HealHint   *aggregate.HealHint `json:"heal_hint,omitempty"` // first non-nil from a failing sensor
    Stdout     string              `json:"-"`                   // raw JSONL retained for debugging
}

type SensorRunSummary struct {
    SensorID string `json:"sensor_id"`
    Verdict  string `json:"verdict"`
}

type Summary struct {
    Total        int `json:"total"`
    Passed       int `json:"passed"`
    Failed       int `json:"failed"`
    Inconclusive int `json:"inconclusive"`
}

// ValidateAll enumerates .harness/use-cases/*.yaml under target, invokes
// the validate-use-case skill once per use case (cmd.Dir=target), parses
// the persisted verdict envelope from stdout, aggregates into a Report,
// and writes the Report to <target>/.harness/reports/<run-id>/report.json.
//
// Skill exit codes 0/1/2 all produce a UseCaseResult (verdict captured
// from the persistedVerdict envelope). Exit code 3 (skill-level error)
// returns a non-nil error carrying the stderr context.
func ValidateAll(ctx context.Context, target string, skills *SkillBinaries) (*Report, error)

// Convenience helpers used by tests.
func (r *Report) AllPassed() bool             // Summary.Passed == Summary.Total
func (r *Report) Failed() []UseCaseResult     // returns []{} when none failed
```

**Contracts:**

- **No live LLM.** `ValidateAll` invokes only `/validate-use-case`. Detection skills are never shelled out from this package.
- **Subject-relative `.harness/`.** `cmd.Dir = target` so the skill's `skillio.FindRepoRoot(cwd)` resolves to the subject (sample or framework root).
- **Skill binaries built once.** `NewSkillBinaries` runs `go build` once per binary. Each `ValidateAll` invocation just `exec.Command`s the resulting binary path.
- **Stable ordering.** Use cases are sorted by id before invocation so report contents are deterministic across runs.

### 6.2 `examples/integration_test.go` (build tag `integration`)

```go
//go:build integration

package examples_test

var skills *validator.SkillBinaries

func TestMain(m *testing.M) {
    if testing.Short() { os.Exit(0) }
    tmp, _ := os.MkdirTemp("", "b7-skills-*")
    defer os.RemoveAll(tmp)
    sb, err := validator.NewSkillBinaries(tmp, frameworkRoot(nil))
    if err != nil { log.Fatalf("build skills: %v", err) }
    skills = sb
    os.Exit(m.Run())
}

func TestCriterion1_StackCoverage(t *testing.T)                // §11.1
func TestCriterion2_UseCasePerEntryPoint(t *testing.T)          // §11.2
func TestCriterion3_TemplateResolution(t *testing.T)            // §11.3
func TestCriterion4_SensorGrounding(t *testing.T)               // §11.4
func TestCriterion5_ValidateExecution_HappyPath(t *testing.T)   // §11.5 (passing)
func TestCriterion5_ValidateExecution_FailingPath(t *testing.T) // §11.5 (broken — failure shape only, no heal)
func TestCriterion7_FixtureReuseAcrossAngles(t *testing.T)      // §11.7
```

Each criterion's test iterates `[]string{"./http-api-sample", "./http-api-sample-broken", "./cli-sample"}` and uses `t.Run(sampleName, ...)` so failures pinpoint which sample regressed. Criteria 1-4 are static-artifact assertions that don't shell out; criteria 5 and 7 do.

### 6.3 `examples/heal_test.go` (build tag `integration`)

```go
//go:build integration

func TestCriterion6_HealOnBroken(t *testing.T) {
    if testing.Short() { t.Skip("heal test is slow") }

    tmp := t.TempDir()
    must(copyDir("./http-api-sample-broken", tmp))

    // 1. First validation: expect 1 failing use case with a heal hint.
    report1, err := validator.ValidateAll(t.Context(), tmp, skills)
    assertNoErr(t, err)
    assertFailedExactly(t, report1, "uc-create-order-bad-input")
    assertNonNilHealHint(t, report1)

    // 2. Apply the committed EditPlan via /heal.
    editPlan := must(os.ReadFile(filepath.Join(tmp, "heal-fixture", "editplan.json")))
    cmd := exec.CommandContext(t.Context(), skills.Heal, "uc-create-order-bad-input")
    cmd.Dir = tmp
    cmd.Stdin = bytes.NewReader(editPlan)
    var stdout, stderr bytes.Buffer
    cmd.Stdout, cmd.Stderr = &stdout, &stderr
    if err := cmd.Run(); err != nil {
        t.Fatalf("/heal failed (exit non-zero): %v\nstderr=%s", err, stderr.String())
    }

    // 3. Re-validate: expect all-pass.
    report2, err := validator.ValidateAll(t.Context(), tmp, skills)
    assertNoErr(t, err)
    if !report2.AllPassed() {
        t.Fatalf("post-heal validation failed: %+v", report2.Summary)
    }
}
```

### 6.4 `examples/dogfood_test.go` (build tag `dogfood`)

```go
//go:build dogfood

package examples_test

func TestFrameworkSelfValidation(t *testing.T) {
    if testing.Short() { t.Skip("dogfood is slow") }

    repoRoot := frameworkRoot(t) // resolves via runtime.Caller from this file
    tmp := t.TempDir()
    skills, err := validator.NewSkillBinaries(tmp, repoRoot)
    assertNoErr(t, err)

    report, err := validator.ValidateAll(t.Context(), repoRoot, skills)
    assertNoErr(t, err)

    if !report.AllPassed() {
        t.Fatalf("dogfood failed:\n  summary=%+v\n  failed=%v\n  report=%s/.harness/reports/%s/report.json",
            report.Summary, ucIDs(report.Failed()), repoRoot, report.RunID)
    }
}
```

Single test, single assertion. The report path is logged so CI artifact upload can surface it.

### 6.5 CI invocations

```bash
# Track 1 (synthetic samples + criteria + heal)
go test -tags=integration -v -timeout 5m ./examples/...

# Track 2 (framework dogfood)
go test -tags=dogfood -v -timeout 5m ./examples/...

# Untagged (validator unit tests with fakes)
go test ./examples/validator/...
```

Three independent invocations. They can run in parallel CI jobs because they touch different `.harness/reports/<run-id>/` directories.

## 7. Heal flow specifics

### 7.1 The bug, concretely

`examples/http-api-sample-broken/handlers.go` ships with `CreateOrder` missing its validation branch:

```go
// BROKEN
func CreateOrder(w http.ResponseWriter, r *http.Request) {
    var body OrderInput
    _ = json.NewDecoder(r.Body).Decode(&body)
    // ✗ no branch here — invalid input falls through.
    order := store.Create(body)
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(order)
}
```

The EditPlan applies the corrected shape:

```go
func CreateOrder(w http.ResponseWriter, r *http.Request) {
    var body OrderInput
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
        return
    }
    if body.Item == "" {
        http.Error(w, `{"error":"missing required field: item"}`, http.StatusBadRequest)
        return
    }
    order := store.Create(body)
    w.WriteHeader(http.StatusCreated)
    _ = json.NewEncoder(w).Encode(order)
}
```

### 7.2 EditPlan shape

Per `skills/heal/skill.md`, the EditPlan stdin shape is:

```json
{
  "files": [
    {
      "path": "handlers.go",
      "op": "write",
      "content": "<full corrected handlers.go contents>"
    }
  ],
  "rationale": "Add 400 Bad Request branch when required field 'item' is missing from POST /orders body, matching uc-create-order-bad-input expectation."
}
```

- `path` is relative to the skill's repo root, which resolves to the sample copy (because `cmd.Dir = tmp`).
- One file, one `write` op — minimal heal target.
- `content` carries the full corrected file (the skill contract is whole-file replace, not patch).

### 7.3 Why a temp-dir copy is non-negotiable

The `/heal` skill mutates the file system. Per `skill.md`: "snapshot/restore is in-memory file backup; verdict=pass means edit kept, verdict=fail means edit reverted." On a successful heal, the modified file persists after the skill exits.

Three failure modes if the test runs in-place:

1. Local re-runs see an already-fixed `handlers.go` and the broken assertions don't fire.
2. Developers' `git status` becomes noisy after every test run.
3. Parallel test invocations race on the same file.

`TestCriterion6_HealOnBroken` therefore copies `examples/http-api-sample-broken/` into `t.TempDir()` and runs all subsequent operations against that copy. `t.Cleanup` removes the tmp dir. The `heal-fixture/` subdirectory is included in the copy so the EditPlan is read from the temp location.

### 7.4 Heal state isolation

The skill writes `.harness/runtime/heal-state.json` tracking iteration count:

- Source-tree `.harness/runtime/` is gitignored (added in this chunk), so the copied sample inherits a clean state.
- First heal call increments iteration to 1. Test expects exit 0 (verdict=pass) on iteration 1 per the user's "single iteration" requirement.
- The test does NOT call `/heal` more than once. If a future regression makes heal need 2+ iterations, this test fails — that's intentional (criterion §11.6 says "first attempt").

## 8. Plan §11 acceptance criteria mapping

Each criterion maps to one (or two) test functions in Track 1. The criteria are properties of the committed `.harness/` artifacts plus runtime behavior — they do not re-run detection.

| # | Criterion (paraphrased) | Test | Sample(s) | What's asserted | How |
|---|---|---|---|---|---|
| 1 | `/detect-stack` covers ≥95% of declared deps + emits archetype with rationale | `TestCriterion1_StackCoverage` | all 3 | `archetype != ""`, `rationale` non-empty, `coverage(go.mod deps, stack-manifest.components) ≥ 0.95` | Load via `internal/stack`. Parse sample's `go.mod` for direct deps. Compute set-overlap. Samples are hand-curated to 100%. |
| 2 | `/detect-use-cases` produces ≥1 valid use case per public entry point with `given/when/then`, archetype-typed entry point, ≥1 fixture | `TestCriterion2_UseCasePerEntryPoint` | all 3 | Each use case has non-empty `given`/`when`/`then`; ≥1 `entry_point`; entry point's `type` matches sample's `archetype`; ≥1 fixture reference | Load via `internal/usecase`; cross-check `entry_point.type` against `stack-manifest.archetype`. |
| 3 | `{{ }}` interpolation resolves cleanly across all use cases | `TestCriterion3_TemplateResolution` | all 3 | Every `{{fixtures.X}}` / `{{entry_points.X}}` reference resolves to a defined id in the same use case | Call `internal/usecase/template.Resolver` per use case; assert no unresolved refs. |
| 4 | `/create-sensors` produces grounded sensors per `(use case × applicable obligatory angle)` | `TestCriterion4_SensorGrounding` | all 3 | Top-level `uses:` references only ids in stack-manifest; step-level `uses:` references only fixtures owned by sensor's use case; one sensor exists per obligatory angle per `ValidationPolicy` | Run `internal/sensor.Grounding(sensors, stack)`. Load `ValidationPolicy` via `internal/policy.Resolve`; intersect with sensors. |
| 5 | `/validate-use-case` executes the sensor graph and aggregates | `TestCriterion5_ValidateExecution_HappyPath` + `TestCriterion5_ValidateExecution_FailingPath` | passing + broken http-api | Passing: `ValidateAll(passing).AllPassed()`. Failing: `ValidateAll(broken).Failed()` contains exactly `uc-create-order-bad-input` with `HealHint != nil` | Drive via `validator.ValidateAll`. Skill stdout parsed for `persistedVerdict`. |
| 6 | Broken sample → `HealHint` actionable enough that `/heal` fixes it on first attempt | `TestCriterion6_HealOnBroken` | broken twin (in temp copy) | Pre-heal: 1 failing UC with non-nil hint. Apply committed `editplan.json` via `/heal`. Exit 0. Post-heal: `AllPassed()`. | Per §7 flow. EditPlan is committed; LLM never invoked. |
| 7 | Same fixture used across ≥2 angles in one use case | `TestCriterion7_FixtureReuseAcrossAngles` | all 3 | For each sample, ∃ a use case `uc` and a fixture id `f` such that `f` appears in step-level `uses:` of ≥2 sensors whose `use_case_id == uc` and whose `angle`s differ | Walk `.harness/sensors/*.yaml`; group by use case; find at least one fixture-id intersection across distinct angles. |

### Track 2 — no criterion mapping

| Test | Subject | Assertion |
|---|---|---|
| `TestFrameworkSelfValidation` | repo root | `ValidateAll(repoRoot).AllPassed()` + `report.json` written to `.harness/reports/<run-id>/` |

The dogfood pass is not a plan §11 assertion. It's a regression gate. It implicitly exercises criteria 1-5 against the framework's own committed `.harness/`, but never asserts them by criterion number.

## 9. Open questions (from B7 chunk doc) — resolved

| Q | B7 doc recommendation | Resolution |
|---|---|---|
| 1. Endpoints per sample | 2 (one GET, one POST) | **Confirmed.** `GET /orders/:id`, `POST /orders`. |
| 2. Broken-variant location | Separate `*-broken/` directory | **Confirmed.** `examples/http-api-sample-broken/` sibling. |
| 3. Dogfood failure mode | Yes — CI fails on dogfood failure | **Confirmed.** `go test -tags=dogfood ./examples/...` is a hard CI gate. |
| 4. Acceptance criteria coverage | One file per criterion (`plan_11_criterion_<n>_test.go`) | **Adjusted.** All criteria live in `examples/integration_test.go` as separate test funcs (one per criterion). Heal lives in `heal_test.go`. Dogfood lives in `dogfood_test.go`. Fewer files, clear per-test mapping. |
| 5. Fixture reuse demonstration | Runtime (generated by `/create-sensors`) | **Adjusted.** Demonstrated by *committed* sensor YAMLs (hand-curated to share fixtures across angles). Detection is manual; CI verifies the property holds in the committed artifacts. |
| 6. Heal acceptance seed bug | Missing status code branch | **Confirmed.** Missing 400 branch in `CreateOrder` of `http-api-sample-broken/handlers.go`. |

### Additional decisions surfaced during brainstorming

| Decision | Resolution |
|---|---|
| Live LLM in CI? | **No.** Pre-committed `.harness/` fixtures strategy. Detection runs manually during B7 implementation; outputs committed. |
| Heal path when `harness heal` CLI is gated? | **`/heal` skill with hand-supplied EditPlan.** CLI heal stays gated; out of scope. |
| Dogfood scope (which framework parts)? | **Whatever the detection skills find when run on the framework root.** Output committed; CI validates against it. |
| Test runner — Makefile, scripts, or `go test`? | **`go test` with build tags only.** No Makefile. `-tags=integration` for Track 1, `-tags=dogfood` for Track 2. |
| Sample set | **Three samples: `http-api-sample`, `http-api-sample-broken`, `cli-sample`.** |
| CI gate shape | **Per-use-case `/validate-use-case` invocation with a structured report; all verdicts must be `pass`.** |
| Skill invocation mechanic | **Build skill binaries once in `TestMain` via `go build`; `exec.Command` the binaries with `cmd.Dir = target`.** Avoids `go run -C` working-directory confusion. |
| Heal in-place vs temp-dir copy | **Temp-dir copy.** Source tree stays pristine; tests are re-runnable; no `git status` noise. |
| `-short` flag | **Tests skip under `-short`.** Both heal_test.go and dogfood_test.go check `testing.Short()`. |
| Parallelism | **Tests run serially within a track.** Sensor parallelism inside the skill is preserved. Failure attribution stays simple. |

## 10. Out of scope

- **Live LLM in CI.** Pre-committed fixtures only.
- **`harness heal` CLI unblock.** Stays gated; B7 uses `/heal` skill.
- **Detection skill output quality.** B4's responsibility. Reviewed manually during B7 implementation.
- **Cross-surface byte-identity tests** (`harness validate` vs `/validate-use-case`). Deferred from B6.
- **Performance / runtime caps.** No perf criteria in plan §11. `-timeout 5m` is the only timing gate.
- **Detection caching, watch mode, run-history pruning.** Future work per B7 chunk doc.
- **Additional sample archetypes** beyond `http-api` and `cli`. Later.

## 11. Deliverable acceptance

B7 is complete when:

1. `go test -tags=integration -v ./examples/...` passes with all 8 test functions green (`TestCriterion1` through `TestCriterion7_*` plus heal).
2. `go test -tags=dogfood -v ./examples/...` passes with `TestFrameworkSelfValidation` green.
3. `go test ./examples/validator/...` passes (validator unit tests with fakes).
4. Each `examples/<sample>/README.md` explains what the sample demonstrates and how to run it standalone.
5. `.gitignore` excludes `.harness/runtime/` and `.harness/reports/`.
6. The framework root's `.harness/` is committed and was produced by running the three inferential skills against the framework. The PR description includes the manual detection procedure used.
7. CI workflow file invokes both tracks as separate jobs (CI workflow specifics are an implementation detail for the plan; mention only that both invocations must run on every PR).

## 12. Future work (post-B7)

- **`harness detect` CLI** — embedded LLM detection via the CLI (deferred from B6 §11 Future work).
- **Recorded LLM transcripts** — if detection drifts often, add a `examples/<sample>/.harness/_detection-transcripts/` capture so re-detection can be replayed for regression testing.
- **More sample archetypes** — `library`, `worker`, `react-app`, etc., as each archetype's detection paths mature.
- **Performance baselines** — once Phase B is stable, add per-sample timing assertions to catch runtime regressions.
- **`harness clean`** — prune `.harness/runtime/` and `.harness/reports/` after N days.
