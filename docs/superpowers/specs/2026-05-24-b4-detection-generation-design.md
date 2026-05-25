# B4 — Detection & Generation Skills: Design Spec

> Source: [`docs/harness-framework/B4-detection-generation.md`](../../harness-framework/B4-detection-generation.md), [`plan.md`](../../harness-framework/plan.md) §5, §9 Phase 1 + Phase 2.
> Status: drafted 2026-05-24, awaiting written-spec review.

## 1. Purpose

Deliver the three LLM-driven AI-primitive skills that produce the framework's detected entities:

- `/detect-stack` — LLM infers the project archetype + stack components; emits `stack-manifest.yaml`.
- `/detect-use-cases` — LLM identifies use cases and emits paired `use-cases/*.yaml` + `fixtures/*.yaml`.
- `/create-sensors` — for one use case, LLM emits one `sensor.yaml` per angle the archetype supports.

All semantic inference is in the slash-command prompt bodies. Go does only schema validation, cross-entity invariant checks, deterministic enrichment, schema-version bookkeeping, and atomic file writes. There is no `internal/detect/` package and no archetype-inference Go function — if a future contributor adds Go that *looks at source code and produces a hypothesis*, the PR is a blocker.

This chunk also makes the repository itself a Claude Code plugin so the three skills are discoverable as slash commands.

## 2. Scope

In:
- `plugin.json` (or `.claude-plugin/plugin.json`) at the repo root — plugin manifest.
- `skills/detect-stack/SKILL.md`, `skills/detect-use-cases/SKILL.md`, `skills/create-sensors/SKILL.md` — slash-command prompt bodies (each ≤200 lines per CLAUDE.md rule 4).
- `skills/<name>/scripts/main.go` — tiny per-skill Go programs that wrap the entity Persist functions and emit structured errors.
- New `Persist` function on each Phase A entity package: `internal/stack`, `internal/usecase`, `internal/fixture`, `internal/sensor`. Each is a thin orchestration around the existing loaders/validators that also handles enrichment, `schema_version` bump, and atomic write to `.harness/`.
- New `internal/persisterror/` package — shared structured error type returned by all four Persist functions and serialized to JSON by the skill scripts.
- One additive schema extension: `applicable_angles: [string]` field on `schemas/stack-manifest.yaml` and the `internal/stack.StackManifest` Go type. Populated by `stack.Persist` from the archetype via `internal/enums.ApplicableAngles[archetype]`; never authored by the LLM.
- Per-skill tests: `skills/<name>/scripts/main_test.go` covering happy path + each structured-error kind (positive/negative table tests with YAML fixtures under `testdata/`).
- Per-package tests for the new `Persist` functions, mirroring Phase A loader test style (golden + invalid cases).

Out:
- Any `internal/detect/` or `internal/sensors/` Go package — there is no Go-side detection or generation logic. Validation reuses Phase A.
- A shared `cmd/harness-write` CLI binary — each skill script is its own binary; entity write logic lives in `internal/<entity>`.
- A shared `lib/harnesspersist/` package — write logic lives directly in the entity packages because every consumer (the three skills here, plus future B5 heal re-emit and B6 CLI) already imports those packages for their loaders.
- Runtime execution of generated sensors (B2/B5).
- Caching of detection outputs (B6).
- A repair-prompt template assembly in Go — the script emits a structured error; what the slash-command body re-prompts with is prose in `SKILL.md`, not code.
- Atomicity of multi-entity bundles — there is no bundle. Each entity write is atomic on its own; the LLM is instructed to write in dependency order (fixtures before their use case; stack + fixtures before sensors).
- A `harness gc` to clean orphan fixtures left by partial writes — acceptable temp state for B4; deferred.

## 3. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **LLM emits content; Go validates and writes.** No inference Go anywhere in this chunk. | Framework principle (CLAUDE.md, B4 doc, README §B). Reviewable by the PR-checklist line "zero Go code in this chunk performs inference." |
| 2 | **Per-entity `Persist` functions on each Phase A package**, not a shared `lib/`. | Every Persist is closely coupled to its entity's loader/validator; the entity package is the natural home. Per CLAUDE.md rule 3, promote to `lib/` only on second-caller-pressure, which doesn't exist here. |
| 3 | **No staging directory.** Each LLM-emitted YAML goes via a temp file path directly to a `Persist` call; success writes one file to `.harness/`, failure writes nothing. | User-stated preference. Bundles are unnecessary because each entity's invariants can be checked against on-disk state at the moment of its write. |
| 4 | **No CLI binary.** Skill scripts call `internal/<entity>.Persist` directly. | User-stated preference; aligns with "scripts are tiny wrappers." Future B6 CLI will surface these as `harness write …` subcommands but is not B4's job. |
| 5 | **Per-entity write order is the LLM's responsibility, instructed in `SKILL.md`.** | Validation invariants determine order: fixtures before their use case; stack-manifest + fixtures before sensors. The order is straightforward enough to encode as prompt prose; no Go orchestrator needed. |
| 6 | **`/create-sensors` derives applicable angles from `stack-manifest.applicable_angles`**, not from a `ValidationPolicy` or via inference. Policy filters at *runtime*, not at *generation*. | Cleaner separation: generation is policy-agnostic, runtime is policy-aware. `/create-sensors` doesn't need to know about `ValidationPolicy` at all. |
| 7 | **`stack.Persist` injects `applicable_angles` from `internal/enums.ApplicableAngles[archetype]`** as a derived field. LLM never authors it. The canonical source of truth is `schemas/enums/archetypes.yaml`'s per-value `applicable_angles` list; the Go `ApplicableAngles` map mirrors it (drift-tested in `internal/enums/drift_test.go`). | Deterministic data belongs in Go. Avoids a separate helper binary or `--list-angles` subcommand. Denormalizing into the manifest means `/create-sensors` reads one file (the manifest) instead of two (manifest + enums YAML). |
| 8 | **Slash-command body drives retries**, not the Go script. Loop cap (3) is prose in `SKILL.md`. | Matches "LLM holds the inference loop" principle; retries are visible in the transcript; scripts stay credentials-free. |
| 9 | **Structured error format: JSON to stdout, exit code 2.** Single error object per call (each call is one entity). | One entity per call → no need for an errors array. Exit codes: `0` success, `2` validation failure (JSON on stdout), `1` script-level failure (bad args, etc., human text on stderr). |
| 10 | **`schema_version` is bumped on each re-emit** (patch component). Script reads existing target file, increments, injects into the new content before writing. | User-stated preference. Means `schema_version` is the *content revision* now, not the framework schema version that plan §7 originally implied — flagged in §6. |
| 11 | **Re-run policy: overwrite + bump.** Existing `.harness/` files are overwritten with the LLM's latest output; `schema_version` is patch-bumped. | User-stated preference; idempotent and Git-trackable. Manual edits get clobbered, but `.harness/` is committed so `git diff` surfaces what changed. |
| 12 | **No confidence floor in B4.** The `confidence` field doesn't exist on `StackManifest`, `UseCase`, `Fixture`, or `Sensor` schemas today (only on `Signal` / `AggregateSignal`). Adding it across four entity schemas + their loaders + their examples is a non-trivial cross-cutting change the B4 brainstorm didn't lock down. Deferred to a follow-up (see §12). | Keeps B4 focused; avoids silently expanding scope into Phase A schema edits. |
| 13 | **No bundle atomicity; partial state can persist on failure.** | User-accepted simplification (a consequence of "no staging"). Orphan fixtures from a failed use-case write don't break anything; a future `harness gc` can sweep them. Slash-command body advises the LLM to either fix-and-retry or `rm` partial writes on give-up. |
| 14 | **Completeness for `/create-sensors`** (one sensor per applicable angle) is enforced by the slash-command body, not by Go. The LLM is instructed to verify coverage after all writes. | Cross-call check is hard at the `Persist` level (each call sees one sensor). Trusting the prompt is acceptable for the MVP; B5/B6 can add a Go-side coverage verifier later. |
| 15 | **`go run` during dev, pre-compile when stable.** Slash-command bodies invoke `go run ./skills/<name>/scripts/ …` for the first iteration; switch to a checked-in pre-compiled binary once interfaces freeze. | Plan B4 doc recommendation. Avoids per-skill build configuration up front. |

## 4. Package surface

### 4.1 `internal/<entity>.Persist`

Four near-identical signatures, one per entity package:

```go
package stack
func Persist(content []byte, harnessDir string) error

package usecase
func Persist(content []byte, harnessDir string) error

package fixture
func Persist(content []byte, harnessDir string) error

package sensor
func Persist(content []byte, harnessDir string) error
```

Each function follows the same pipeline:

1. **Parse + schema validate** via the existing Phase A loader (e.g., `usecase.Load([]byte)`). If invalid → return `persisterror.Error{Kind: SchemaViolation, …}`.
2. **Cross-entity invariant check** against on-disk state under `harnessDir` (see §4.2 table). If invalid → return the corresponding `persisterror.Error`.
3. **Enrichment** (only `stack.Persist`): compute `applicable_angles` from the parsed archetype via `internal/enums.ApplicableAngles[archetype]`; inject into the entity struct.
4. **`schema_version` bump**: read the target file at `<harnessDir>/<entity-dir>/<id>.yaml` if it exists, parse its `schema_version`, patch-bump (e.g., `1.0.3` → `1.0.4`). If the target doesn't exist, use the LLM's emitted `schema_version` as-is (default `1.0.0`).
5. **Atomic write**: marshal the (possibly enriched, schema-version-bumped) entity back to YAML; `os.WriteFile` to `<target>.tmp`; `os.Rename` over `<target>`.

Confidence-floor check is deliberately not in this pipeline — see decision #12 and §12 follow-ups for the rationale and the deferred work.

The target path is derived from entity type and id:

| Entity | Target path under `harnessDir` |
|---|---|
| StackManifest | `stack-manifest.yaml` (single file; no id-based filename) |
| Fixture | `fixtures/<id>.yaml` |
| UseCase | `use-cases/<id>.yaml` |
| Sensor | `sensors/<id>.yaml` |

The `harnessDir` is the script's `--harness-dir` flag value, defaulting to `.harness` in the repo root.

### 4.2 Cross-entity invariants per Persist

| Persist | Cross-entity check (against on-disk state under `harnessDir`) |
|---|---|
| `stack.Persist` | None. (Enrichment only.) |
| `fixture.Persist` | None. (Allows writing fixtures before their owning use case.) |
| `usecase.Persist` | (a) Every `{{ }}` token in `given/when/then` resolves via `internal/usecase/template.Resolver` against on-disk `fixtures/*.yaml` + this use case's `entry_points`. (b) Every id in `fixture_ids` exists as a file under `harnessDir/fixtures/` with `use_case_id == this.id`. |
| `sensor.Persist` | (a) `stack-manifest.yaml` exists; sensor's top-level `uses:` ⊆ that manifest's `components[*].id` (via `internal/sensor.Grounding`). (b) `use-cases/<sensor.use_case_id>.yaml` exists. (c) For every step with `uses:`, every id is the id of a fixture under `harnessDir/fixtures/` with `use_case_id == sensor.use_case_id`. (d) sensor's `angle` is in the manifest's `applicable_angles`. |

The functions read on-disk state lazily (only when the corresponding check is reached). They do not lock `harnessDir`; concurrent writes are out of scope for B4 (a single skill invocation is sequential).

### 4.3 `internal/persisterror`

Shared structured error type returned by every Persist:

```go
package persisterror

type Kind string

const (
    SchemaViolation       Kind = "schema_violation"
    FixtureBinding        Kind = "fixture_binding"      // usecase step uses unknown / mis-owned fixture
    Grounding             Kind = "grounding"            // sensor top-level uses unknown stack component
    TemplateResolution    Kind = "template_resolution"  // {{ }} token cannot resolve
    MissingRequiredField  Kind = "missing_required_field"
    UnknownEnumValue      Kind = "unknown_enum_value"
    AngleNotApplicable    Kind = "angle_not_applicable" // sensor.angle ∉ stack-manifest.applicable_angles
    MissingDependency     Kind = "missing_dependency"   // e.g., sensor references absent use-case
)
// (ConfidenceBelowFloor intentionally absent — deferred per decision #12.)

type Error struct {
    Kind       Kind                   `json:"kind"`
    EntityType string                 `json:"entity_type"` // "stack-manifest" | "use-case" | "fixture" | "sensor"
    EntityID   string                 `json:"entity_id,omitempty"`
    File       string                 `json:"file,omitempty"` // the input temp path
    Path       string                 `json:"path,omitempty"` // YAML jsonpath of the violation
    Value      any                    `json:"value,omitempty"`
    Expected   string                 `json:"expected,omitempty"`
    Details    map[string]any         `json:"details,omitempty"`
    Message    string                 `json:"message"`
}

func (e *Error) Error() string { return e.Message }
```

The struct implements `error`. Skill scripts type-assert and `json.NewEncoder(os.Stdout).Encode(err)` on failure.

### 4.4 Skill script shape

Each skill script is small (~30–50 LOC). Example for `detect-stack`:

```go
// skills/detect-stack/scripts/main.go
package main

import (
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "os"

    "github.com/iurykrieger/lastro/internal/persisterror"
    "github.com/iurykrieger/lastro/internal/stack"
)

func main() {
    file := flag.String("file", "", "Path to the LLM-emitted stack-manifest YAML")
    harnessDir := flag.String("harness-dir", ".harness", "Target .harness directory")
    flag.Parse()
    if *file == "" {
        fmt.Fprintln(os.Stderr, "missing --file")
        os.Exit(1)
    }
    content, err := os.ReadFile(*file)
    if err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
    if err := stack.Persist(content, *harnessDir); err != nil {
        var pe *persisterror.Error
        if errors.As(err, &pe) {
            _ = json.NewEncoder(os.Stdout).Encode(pe)
            os.Exit(2)
        }
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

The `detect-use-cases` script differs only in dispatching on `--type fixture|use-case` to choose between `fixture.Persist` and `usecase.Persist`. The `create-sensors` script mirrors `detect-stack` but calls `sensor.Persist`.

### 4.5 Skill body shape (`SKILL.md`)

Each skill body is markdown with YAML frontmatter:

```markdown
---
name: detect-stack
description: Infer the project archetype + stack components and emit `.harness/stack-manifest.yaml`.
---

# /detect-stack

You are detecting the stack of the repository at the current working directory.

## What to inspect
- Manifests: go.mod, package.json, pyproject.toml, Cargo.toml, etc.
- Directory structure for archetype hints (cmd/ + main.go → CLI; …)
- Framework conventions visible in source files

## What to emit
A single YAML file matching `schemas/stack-manifest.yaml`:
- `archetype`: one of the enum values in `schemas/enums/archetypes.yaml`
- `components`: list of detected StackComponent items
- DO NOT include `applicable_angles` — the validator injects it.

## How to write
1. Use the Write tool to put your YAML at `/tmp/stack-manifest.yaml`.
2. Run: `go run ./skills/detect-stack/scripts/ --file /tmp/stack-manifest.yaml`
3. If exit code is 0, you are done.
4. If exit code is 2, read the JSON error from stdout, fix the YAML, re-run. Stop after 3 attempts and report the unresolved error to the user.
```

The other two `SKILL.md` files follow the same pattern, with additional instructions for write ordering (`/detect-use-cases`) and angle iteration (`/create-sensors`).

## 5. Per-skill input/output contracts

| Skill | LLM context (slash-command body) | LLM emits | Script does | Lands at |
|---|---|---|---|---|
| `/detect-stack` | repo root | one stack-manifest YAML | `stack.Persist` → schema + inject `applicable_angles` + bump + write | `.harness/stack-manifest.yaml` |
| `/detect-use-cases` | repo root + `.harness/stack-manifest.yaml` (LLM reads) | for each detected use case: N fixtures (call `Persist` per fixture), then 1 use case (call `Persist`) | `fixture.Persist` then `usecase.Persist` per entity | `.harness/fixtures/<id>.yaml`, `.harness/use-cases/<id>.yaml` |
| `/create-sensors <use-case-id>` | `.harness/stack-manifest.yaml` + `.harness/use-cases/<use-case-id>.yaml` (LLM reads); `applicable_angles` list comes from the manifest | one sensor per angle in `applicable_angles` | `sensor.Persist` per sensor | `.harness/sensors/<id>.yaml` |

### 5.1 `/detect-stack`

- Single entity per skill invocation.
- LLM may need to re-emit if schema-invalid — retry loop in `SKILL.md`.
- On success, `.harness/stack-manifest.yaml` contains: LLM-authored `archetype` and `components`, plus script-injected `applicable_angles`.

### 5.2 `/detect-use-cases`

- LLM is told: for each use case, write all its fixtures first (`--type fixture`), then the use case (`--type use-case`).
- Each call validates one entity. A use-case write fails with `Kind: FixtureBinding` if its referenced fixtures are not yet on disk → LLM writes the missing fixture, retries.
- Atomicity is per call. If write of fixture N+1 fails after fixtures 1..N succeeded, the prior fixtures remain on disk. The slash-command body tells the LLM: "if you give up on a use case, `rm` the fixture files you wrote for it."

### 5.3 `/create-sensors`

- Invoked with a single use-case-id argument.
- Slash-command body reads `.harness/stack-manifest.yaml` for `applicable_angles` and `.harness/use-cases/<id>.yaml` for the use case.
- LLM emits one sensor per angle; calls `Persist` per sensor.
- After all writes, LLM verifies coverage (one sensor per angle) by listing `.harness/sensors/` and checking each angle is represented for this use case.

## 6. Schema changes

### 6.1 `applicable_angles` on stack-manifest

Additive: a new field on `schemas/stack-manifest.yaml` and `internal/stack.StackManifest`.

```yaml
# schemas/stack-manifest.yaml (excerpt)
applicable_angles:
  type: array
  items: { type: string }
  description: |
    Derived field. Populated by `stack.Persist` from the archetype via
    `internal/enums.ApplicableAngles[archetype]`. The canonical source of
    truth for the archetype × angle matrix is
    `schemas/enums/archetypes.yaml` (each value carries its own
    `applicable_angles` list); the Go map mirrors it under a drift contract.
    Never authored by the LLM.
```

Existing stack-manifest examples under `schemas/examples/stack-manifest/` get the field added so loader tests still pass. The `internal/stack` loader is extended to accept and round-trip the new field.

### 6.2 `schema_version` is now the content revision

This is a divergence from plan §7's original framing (where `schema_version` was the framework schema version pinned at creation). User-confirmed preference: `Persist` patch-bumps `schema_version` on every re-emit.

Implications:
- Phase A loaders must tolerate version drift on read. If any loader pins to an exact version (e.g., rejects anything not `1.0.0`), B4 relaxes it to accept the `1.x.x` range.
- The framework's own schema version is no longer recorded per-file. If we need that later, we'll add a separate `framework_schema_version` field; out of scope for B4.

## 7. Retry flow (end-to-end example)

For `/detect-use-cases`, when the LLM writes a use case before one of its fixtures:

```
# LLM: writes fx_create_order_request to /tmp/fx_request.yaml via Write tool
$ go run ./skills/detect-use-cases/scripts/ --type fixture --file /tmp/fx_request.yaml
$ echo $?
0
# LLM: writes the use case to /tmp/uc.yaml referencing fx_create_order_request AND fx_create_order_response
$ go run ./skills/detect-use-cases/scripts/ --type use-case --file /tmp/uc.yaml
{"kind":"fixture_binding","entity_type":"use-case","entity_id":"create-order",
 "file":"/tmp/uc.yaml",
 "details":{"missing_fixture_ids":["fx_create_order_response"]},
 "message":"Use case references fixtures not found in .harness/fixtures/"}
$ echo $?
2
# LLM: reads the JSON, recognizes the missing fixture, writes fx_create_order_response to /tmp/fx_response.yaml
$ go run ./skills/detect-use-cases/scripts/ --type fixture --file /tmp/fx_response.yaml
$ echo $?
0
# LLM: retries the use-case write
$ go run ./skills/detect-use-cases/scripts/ --type use-case --file /tmp/uc.yaml
$ echo $?
0
```

Cap = 3 attempts per write call, enforced by the LLM following the `SKILL.md` instruction.

## 8. Testing

### 8.1 `internal/<entity>.Persist` tests

Each Persist function gets a `persist_test.go` with table-driven cases:

| Case category | Examples |
|---|---|
| Happy path — new file | Write a valid entity into an empty `harnessDir`; assert file exists with `schema_version` from input. |
| Happy path — existing file (bump) | Pre-seed `harnessDir/<target>` with `schema_version: 1.0.2`; write new content; assert file has `schema_version: 1.0.3`. |
| Schema violation | Each required field missing → `Kind: SchemaViolation`. |
| Enum violation | Bad archetype / angle / role / kind → `Kind: UnknownEnumValue`. |
| Cross-entity (per entity, see §4.2) | `usecase.Persist`: missing fixture → `Kind: FixtureBinding`. `sensor.Persist`: missing stack component → `Kind: Grounding`; sensor angle not in `applicable_angles` → `Kind: AngleNotApplicable`. |
| Atomic write under failure | Inject a write error after validation; assert no partial file on disk and target unchanged from pre-state. |

Fixtures under `internal/<entity>/testdata/persist/` (valid + invalid YAMLs).

### 8.2 `internal/persisterror` test

A small `persisterror_test.go` covering: JSON round-trip with each `Kind`, `Error()` returns `Message`, `errors.As` works on `*persisterror.Error`.

### 8.3 Skill script tests

Each `skills/<name>/scripts/main_test.go` exercises the binary as a subprocess (via `os/exec`) against fixture YAMLs:

- Success: exit `0`, no stdout (or only the bumped YAML if echo is added).
- Validation failure: exit `2`, stdout is a JSON `persisterror.Error` matching expectations.
- Bad args: exit `1`, human message on stderr.

### 8.4 Skill-body (`SKILL.md`) testing

Not unit-tested. Validated by dogfood in B7 (the framework's own use cases exercise these three skills end-to-end).

## 9. Plugin packaging

The repository root grows two things:

- **`plugin.json` (or `.claude-plugin/plugin.json`)** — Claude Code plugin manifest. Minimum fields: `name`, `version`, `description`. Format follows Claude Code's plugin schema; concrete field set confirmed at implementation time.
- **`skills/` at root** — three skill directories with `SKILL.md` + `scripts/`.

The plugin contains only the three slash commands; it does not register hooks, agents, or MCP servers in this chunk.

## 10. Out of scope (deferred / handled elsewhere)

| Concern | Where it lives |
|---|---|
| Sensor execution, signal collection, aggregation | B2/B5 |
| Heal loop (re-validation after `/heal`) | B3 |
| Skill-level orchestrators (`/validate-use-case`) | B5 |
| `harness` top-level CLI (`harness detect`, etc.) | B6 |
| Dogfood end-to-end (the framework validating itself) | B7 |
| `harness gc` for orphan fixtures | future / not currently planned |
| Bundle-level atomicity (transactional multi-entity writes) | rejected — partial state is acceptable |
| Pre-compiled binary distribution of skill scripts | optional follow-up once interfaces stabilize |
| Confidence-floor enforcement and the `confidence` field on `StackManifest` / `UseCase` / `Fixture` / `Sensor` schemas | post-B4 (deferred — see decision #12 and §12) |
| `framework_schema_version` field (separate from content revision) | post-B4, if/when schema migrations become a concern |
| Drift test asserting `internal/enums.ApplicableAngles` matches `schemas/enums/archetypes.yaml` | already in `internal/enums/drift_test.go` per the comment in `archetype_angles.go` — verify still passes after B4's changes; no new drift test required |

## 11. Acceptance criteria

- `/detect-stack` against `examples/http-api-sample/` (or any Go HTTP API): LLM emits a stack-manifest; `stack.Persist` injects `applicable_angles`; `.harness/stack-manifest.yaml` contains `archetype: http-api`, components, and a populated `applicable_angles` matching `internal/enums.ApplicableAngles[ArchetypeHTTPAPI]`.
- `/detect-use-cases` against the same sample: for at least one public entry point, paired `.harness/use-cases/<id>.yaml` + matching `.harness/fixtures/*.yaml` exist; every `{{ }}` token in the use case resolves via `internal/usecase/template.Resolver`; running again patch-bumps `schema_version` on every overwritten file.
- `/create-sensors <use-case-id>` against an emitted use case: one `.harness/sensors/<id>.yaml` per angle listed in the stack manifest's `applicable_angles`; every sensor passes `internal/sensor.Grounding` and per-step fixture-binding checks.
- Simulated bad LLM output exercises every `Kind` in `internal/persisterror`: each Persist returns the expected typed error, the corresponding skill script exits 2 with matching JSON on stdout, and `.harness/` is unchanged.
- PR review checklist: zero Go in this chunk performs inference. No "scan handlers," no "guess archetype," no "assemble sensor template from angle name." If a reviewer finds one, it's a blocker.

## 12. Open follow-ups (not blocking B4)

1. **B5 should add a Go-side coverage verifier** for `/create-sensors` completeness, so the cross-call "one sensor per applicable angle" check isn't left entirely to the prompt.
2. **B6's `harness` CLI** should expose `harness write <entity>` subcommands wrapping the four `Persist` functions, supplanting the per-skill scripts as the canonical surface.
3. **Confidence floor on emitted entities.** B4 doc Q4 recommended requiring a `confidence` field on every emitted entity with rejection below `0.7`. B4 defers this because the field is absent from `StackManifest`, `UseCase`, `Fixture`, and `Sensor` schemas today (it only exists on `Signal` / `AggregateSignal`), and adding it touches four Phase A schemas + their loaders + their examples + their tests. Should be picked up as its own brainstorm + plan, ideally alongside B5's policy-runtime work so the floor value is configurable from `ValidationPolicy` rather than hardcoded. Adding the field also unlocks the `ConfidenceBelowFloor` `persisterror.Kind` deliberately omitted in §4.3.
4. **Pre-compiled binaries**: replace `go run` invocations in `SKILL.md` with checked-in binaries once the script interfaces freeze (probably after dogfood in B7).
5. **`framework_schema_version` field**: revisit once schema migrations become a real concern. Currently `schema_version` doubles as both framework + content version; this divergence is documented in §6.2.
