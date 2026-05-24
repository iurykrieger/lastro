# B4 — Detection & Generation Skills (LLM-driven, persist-only Go)

> Source plan: [`plan.md`](plan.md) §5 (Skills table rows 1–3), §9 Phase 1 + Phase 2

The whole point of the framework is **LLM-driven semantic detection**: the LLM reads the project, infers the archetype, identifies use cases and entry points, and writes the sensor manifests. Scripts in this chunk **only persist and run** what the LLM produces. There is no archetype-inference Go, no entry-point-scanning Go, no sensor-template-assembly Go.

If a brainstorm proposes a Go function that *looks at source code and produces a hypothesis*, stop. That belongs in the slash-command prompt body.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b4-detection-generation origin/main
```

## Parallelism

- **Can run in parallel with:** B1, B2, B3, B5, B6 — only consumes Phase A entity types; touches `skills/` and (if introduced) `lib/`. Never the runtime tree.
- **Must run after:** Phase A (✓).
- **Blocks:** B7 (examples + dogfood — needs all three skills working).

## Division of responsibility (read this before designing)

| Concern | Where | Why |
|---|---|---|
| Archetype inference | Slash-command prompt body | Semantic; LLM does it |
| Use-case identification, given/when/then authoring | Slash-command prompt body | Semantic |
| Fixture authoring | Slash-command prompt body | Semantic |
| Sensor authoring per `(UseCase × angle)` | Slash-command prompt body | Semantic |
| Schema validation of the YAML the LLM emitted | `skills/<name>/scripts/` Go | Deterministic; reuses Phase A frozen schemas |
| Invariant checks (atomicity of paired output; grounding `uses:` ⊆ stack components; step `uses:` ⊆ owned fixtures) | `skills/<name>/scripts/` Go | Deterministic; reuses Phase A validators |
| Writing validated YAML to `.harness/` | `skills/<name>/scripts/` Go | Deterministic file I/O |
| Reporting validation errors back to the LLM for re-emit | `skills/<name>/scripts/` Go (structured error) → slash-command body decides what to do with it | Deterministic error reporting; LLM decides whether/how to retry |

**Explicitly out of scope for Go:**
- No "scan the repo for Go files."
- No "look for HTTP handler signatures."
- No "guess the archetype from go.mod."
- No "assemble a sensor template from an angle name."

Those are all the LLM's job. Go's job stops at "is this YAML well-formed and does it satisfy the frozen invariants? If not, report the error precisely."

## Scope

In:
- `skills/detect-stack/skill.md` — the slash-command prompt. Tells the LLM what to look at, what schema to emit, and what `.harness/stack-manifest.yaml` should contain (`archetype`, `confidence`, stack components).
- `skills/detect-stack/scripts/` — small Go binary. Reads the LLM's YAML output (from stdin or a path), validates against `schemas/stack-manifest.yaml`, writes to `.harness/stack-manifest.yaml`. On validation failure, exits non-zero with a structured error.
- `skills/detect-use-cases/skill.md` — prompt body. Emits **paired** `use-cases/*.yaml` + `fixtures/*.yaml`. Atomicity: no orphan use case.
- `skills/detect-use-cases/scripts/` — Go binary. Validates paired output (atomicity, schemas, `{{ }}` token resolution via `internal/usecase/template.Resolver`), writes to `.harness/use-cases/` and `.harness/fixtures/`.
- `skills/create-sensors/skill.md` — prompt body. Takes a `UseCase` + a list of pre-resolved applicable angles (resolved upstream via `policy.Resolve`); emits N sensor YAMLs.
- `skills/create-sensors/scripts/` — Go binary. Validates grounding (top-level `uses:` ⊆ stack components — calls `internal/sensor.Grounding`) and fixture-binding (step `uses:` ⊆ owned fixtures), writes to `.harness/sensors/`.
- *(Optional, promote on second caller per CLAUDE.md rule 3)* `lib/harnesspersist/` — shared helpers: read YAML from stdin, validate against an embedded schema, write to a target path, return a structured error. If only one script needs it, keep it skill-local.

Out:
- `internal/detect/...` and `internal/sensors/` Go packages — **do not create them**. There is no Go-side detection logic to host. Validation lives in Phase A entity packages (`internal/sensor.Grounding`, etc.) and the scripts call into those.
- Repair-prompt template assembly in Go. The script's job is to emit a precise, structured error (which schema violation, which path, what value was rejected). The slash-command body decides whether to ask the LLM to retry and how to phrase the retry prompt. Keep prompt text out of compiled binaries.
- Runtime execution of generated sensors (B2, B5).
- Caching (B6's concern — caches the *LLM-produced YAML*, not intermediate scans).

## Inputs / Outputs

| Skill | What the LLM gets in the prompt | What the LLM emits | What the script validates → writes |
|---|---|---|---|
| `/detect-stack` | repo root listing + key files | `stack-manifest.yaml` (with `archetype`, `confidence`, components) | `.harness/stack-manifest.yaml` |
| `/detect-use-cases` | repo root + `stack-manifest.yaml` | paired `use-cases/*.yaml` + `fixtures/*.yaml` | `.harness/use-cases/`, `.harness/fixtures/` |
| `/create-sensors` | `UseCase`, applicable angles (pre-resolved), `stack-manifest.yaml` | N `sensor.yaml`s | `.harness/sensors/` |

## Dependencies

- Phase A entity packages — **consumed only for validation**:
  - `internal/stack` — stack-manifest schema/loader.
  - `internal/usecase`, `internal/fixture`, `internal/entrypoint`, `internal/usecase/template` — use-case + fixture validation; template token resolution.
  - `internal/sensor` — `Grounding`, fixture-binding checks, sensor schema/loader.
  - `internal/policy` — used by the **caller** of `/create-sensors` (not by the script itself) to pre-resolve applicable angles.
- No dependency on B1–B3 or B5–B6.

## Open questions for `/brainstorming`

1. **Skill script transport.** Slash command writes the LLM's YAML to a temp file (or pipes via stdin) and invokes the Go binary. Compile-and-ship vs `go run` during dev? Recommendation: `go run` until the binary stabilizes; pre-compile checked-in once interfaces freeze.
2. **`.harness/` layout.** Lock the on-disk layout here so B5/B6 can rely on it: `.harness/stack-manifest.yaml`, `.harness/use-cases/<id>.yaml`, `.harness/fixtures/<id>.yaml`, `.harness/sensors/<id>.yaml`. Anything else?
3. **Retry semantics.** The Go script reports a validation error. Does the slash-command body re-prompt the LLM with the error message (LLM-driven retry), or does Go loop with a cap? Per the skill-architecture rule: deterministic loop control is OK in Go, but the prompt content (what to ask the LLM to do on retry) lives in the skill body or a template file. Recommendation: keep the loop in the slash-command body, not in Go — that way retries are visible in the LLM transcript and the framework stays "LLM holds the inference loop."
4. **Confidence floor.** Plan §10.3 inferential confidence floor (default 0.7). Does the LLM emit a `confidence` field in its YAML, and does the script reject output below the floor? Recommendation: require `confidence` in every emitted entity; reject below floor; the slash-command body explains why and asks for re-emit.
5. **`/create-sensors` angle source.** Pre-resolved by the caller (preferred — keeps the chain explicit) or re-resolved inside the script (introduces a `policy.Resolve` call here)? Recommendation: pre-resolved.
6. **Shared `lib/` vs skill-local.** Per CLAUDE.md rule 3, start logic in the skill's `scripts/`; promote to `lib/` on second caller. Will any of (read-stdin, validate-against-schema, write-to-path, structured-error) actually be reused, or is each skill's persist step different enough to stay local? Recommendation: start with skill-local; revisit after the first two skills land.

## Deliverable acceptance

- `/detect-stack` against a Go HTTP API sample: LLM emits a `stack-manifest.yaml`; script validates against the schema and writes to `.harness/stack-manifest.yaml`; archetype = `http-api`; `confidence` ≥ 0.7.
- `/detect-use-cases` against the same sample: paired `(use-case, fixtures)` per public entry point; every `{{ }}` token in use-case text resolves against the emitted fixtures + entry points (script verifies via `internal/usecase/template.Resolver`); atomicity invariant holds.
- `/create-sensors` against one emitted use case + a default `EffectivePolicy`: one sensor per applicable angle; all sensors pass `internal/sensor.Grounding` and fixture-binding validators.
- Validation-failure path test: simulated bad LLM output → script exits non-zero with a structured error citing the failing schema path / invariant. (How the slash-command body reacts to that — re-prompt or fail — is the slash command's choice, exercised in dogfood B7.)
- **PR review checklist:** zero Go code in this chunk performs inference. No "if file exists then archetype = ...", no "scan handlers", no "guess fixture from response shape." If a reviewer spots one, it's a blocker.
