# B5 — Detection & Generation Skills

> Source plan: [`plan.md`](plan.md) §5 (Skills table rows 1–3), §9 Phase 1 + Phase 2

The three inferential skills that produce all framework inputs from a raw repo: `/detect-stack`, `/detect-use-cases`, `/create-sensors`. Each pairs a skill definition (slash command + scripts) with a Go backing package.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b5-detection-generation origin/main
```

## Parallelism

- **Can run in parallel with:** B1, B2, B3, B4, B6, B7 — this chunk only consumes Phase A entity types and adds new code paths under `internal/detect/`, `internal/sensors/`, `skills/`. No overlap with any runtime package.
- **Must run after:** Phase A (✓ — needs `internal/stack`, `internal/usecase`, `internal/fixture`, `internal/sensor`, `internal/policy` entity types).
- **Blocks:** B8 (examples + dogfood — needs all three skills working to detect a sample repo end-to-end). Does **not** block B6 or B7 at build time; they consume the *output formats* (already frozen schemas), not the implementations.

## Scope

In:
- `skills/detect-stack/` — slash command definition + `scripts/` invoking `internal/detect/stack`. Output: `stack-manifest.yaml` per E2 schema, with `archetype` field.
- `skills/detect-use-cases/` — slash command + `scripts/` invoking `internal/detect/usecases`. Output: paired `use-cases/*.yaml` + `fixtures/*.yaml` (atomic per plan §5 critical note).
- `skills/create-sensors/` — slash command + `scripts/` invoking `internal/sensors`. Output: N `sensors/*.yaml` per `(UseCase × applicable angle)` from the `ValidationPolicy`.
- Backing Go packages with the deterministic parts:
  - `internal/detect/stack/` — file/dependency scanning helpers, archetype inference scaffolding.
  - `internal/detect/usecases/` — entry-point inference scaffolding.
  - `internal/sensors/` — generation scaffolding: angle enumeration, grounding-aware sensor template assembly, schema validation pre-emit.
- The deterministic validators that catch bad LLM output before it's written: grounding (`uses:` refs only stack components), fixture-binding (step `uses:` refs only owned fixtures), schema validity.

Out:
- Runtime execution of generated sensors (B3, B6).
- The LLM prompts themselves are part of the skill files (≤ 200 lines per CLAUDE.md rule 4) — but the *retry/repair loop* logic when an LLM emits a bad sensor is part of `internal/sensors/` and belongs here.
- Caching of detection outputs (plan §9 Phase 6 — B7's CLI concern).

## Inputs / Outputs

| Skill | Input | Output |
|---|---|---|
| `/detect-stack` | repo root | `stack-manifest.yaml` |
| `/detect-use-cases` | repo root + `stack-manifest.yaml` | `use-cases/*.yaml` + `fixtures/*.yaml` (atomic) |
| `/create-sensors` | `UseCase`, `ValidationPolicy`, `stack-manifest.yaml` | N `sensors/*.yaml` (one per applicable angle) |

## Dependencies

- Phase A entity packages (all 9).
- No B1–B4 dependency. This is the producer side.

**Coordination note for parallel work:** the three skills can be implemented by three separate sub-branches off this chunk's branch if a single contributor wants to fan out further — they share `internal/lib/` only (per CLAUDE.md rule 3, promote shared logic on second caller).

## Open questions for `/brainstorming`

1. **Skill scripts language.** CLAUDE.md rule 3 says skill `scripts/` contain skill-specific code. Go binaries invoked from the slash command, or shell wrappers around `cmd/harness/` subcommands? Recommendation: Go binaries here (B5) — the CLI (B7) is a different surface and shouldn't be a hard prereq for skills.
2. **`/detect-use-cases` atomicity.** Plan §5 critical note: a use case without fixtures is rejected. How does the skill enforce atomicity if the LLM emits one but not the other? Recommendation: the skill's deterministic post-processor (in `internal/detect/usecases/`) validates pairing before writing; missing fixture → repair-prompt LLM once, then fail.
3. **`/create-sensors` angle enumeration.** The set of "applicable angles" comes from `ValidationPolicy` resolution (B1). Does this chunk import `internal/policy` (introduces a B1 dep), or take a pre-resolved angle list as input? Recommendation: take pre-resolved list — keeps B5 truly parallel with B1.
4. **Retry/repair budget.** How many times does the skill retry an LLM-generated invalid output before failing? Recommendation: 2; first retry includes the validator's error message in the prompt.
5. **Detection caching.** Plan §9 Phase 6. Where does the cache live and what's the invalidation key? Recommendation: out of scope here; B7's CLI can wrap with `--cache` flag.
6. **Archetype assignment confidence.** §10.3's confidence floor applies to inferential outputs. Does `/detect-stack` surface confidence in the manifest, or is it implicit? Recommendation: surface it — the runtime should see it for §6.3 weighting.

## Deliverable acceptance

- `/detect-stack` against a Go HTTP API sample repo produces a `stack-manifest.yaml` that schema-validates, lists ≥95% of go.mod direct deps as components, and reports `archetype: http-api` with rationale.
- `/detect-use-cases` against the same sample produces ≥1 paired `(use-case, fixtures)` per public HTTP handler; each use case schema-validates; every `{{ }}` token in the use case text resolves against the emitted fixtures + entry points.
- `/create-sensors` against one of the emitted use cases + a default `ValidationPolicy` produces one sensor per applicable angle; all sensors pass grounding (top-level `uses:` ⊆ stack components) and fixture-binding (step `uses:` ⊆ owned fixtures) validators.
- Retry/repair test: a deliberately-malformed LLM response triggers one repair-prompt and then succeeds; second consecutive failure surfaces a typed error.
