# B4 — Detection & Generation Skills (LLM-driven)

> Source plan: [`plan.md`](plan.md) §5 (Skills table rows 1–3), §9 Phase 1 + Phase 2
>
> **Architectural rewrite vs pre-rebase B5.** The whole point of the framework is **LLM-driven** detection and generation. Go packages in this chunk do *not* infer archetypes, scan for entry points, or assemble sensor templates — that work lives in the slash-command prompt body. Go does only deterministic validation, persistence, and repair-prompt assembly.

## Branching (mandatory)

```bash
git fetch origin
git checkout -b feat/b4-detection-generation origin/main
```

## Parallelism

- **Can run in parallel with:** B1, B2, B3, B5, B6 — only consumes Phase A entity types; touches `skills/`, `internal/detect/`, `internal/sensors/`, never the runtime tree.
- **Must run after:** Phase A (✓).
- **Blocks:** B7 (examples + dogfood — needs all three skills working).

## Division of responsibility (read before designing)

| Concern | Lives in | Why |
|---|---|---|
| Archetype inference, entry-point identification, sensor assembly | `skills/<name>/skill.md` (prompt body) | LLM has the world knowledge to do this; Go would be reinventing it |
| Schema + invariant validation of LLM output | `internal/detect/...`, `internal/sensors/` Go packages | Deterministic; uses the frozen schemas from Phase A |
| Writing validated YAML to `.harness/<subdir>/` | Same Go packages | Deterministic file I/O |
| Repair-prompt assembly when validation fails | Shared `lib/repairprompt/` | Deterministic templating; the *prompt content* lives in templates, the *loop* lives in Go |
| Retry/repair loop control (cap, backoff) | Skill script Go binary | Deterministic loop |

If a brainstorm finds itself writing Go code that *infers* archetypes or *guesses* fixtures, stop — that's a prompt change, not Go.

## Scope

In:
- `skills/detect-stack/` — `skill.md` (the slash-command prompt) + `scripts/` Go binary that validates the LLM's `stack-manifest.yaml` output, writes it to `.harness/stack-manifest.yaml`, and assembles a repair prompt on validation failure.
- `skills/detect-use-cases/` — same shape; outputs **paired** `use-cases/*.yaml` + `fixtures/*.yaml`. Atomicity invariant: an unpaired use case (without fixtures) is rejected. The skill prompt enforces emit-both; the script validates it.
- `skills/create-sensors/` — same shape; outputs N sensor YAMLs per `(UseCase × applicable angle)`. The skill prompt receives the resolved applicable angles as input and fans out.
- Backing Go packages — **validation + persistence + repair-prompt only**:
  - `internal/detect/stack/` — validates LLM output against `schemas/stack-manifest.yaml`; writes to `.harness/`.
  - `internal/detect/usecases/` — validates paired output; enforces atomicity; writes to `.harness/use-cases/`, `.harness/fixtures/`.
  - `internal/sensors/` — validates grounding (top-level `uses:` ⊆ stack components) and fixture-binding (step `uses:` ⊆ owned fixtures) using `internal/sensor.Grounding` (Phase A — verify the entry points there); writes to `.harness/sensors/`.
- Shared helper: `lib/repairprompt/` — templates a "your previous output failed validation because X; fix only that and re-emit" prompt.

Out:
- Anything that looks like inference, scaffolding, scanning, or guessing in Go. Stop and move it to the prompt.
- Runtime execution of generated sensors (B2, B5).
- Caching of detection outputs (B6's CLI concern — caches the *LLM-produced YAML*, not intermediate scans).

## Inputs / Outputs

| Skill | LLM input | LLM output | Script validates → writes to |
|---|---|---|---|
| `/detect-stack` | repo root listing + key files | `stack-manifest.yaml` (with `archetype`) | `.harness/stack-manifest.yaml` |
| `/detect-use-cases` | repo root + `stack-manifest.yaml` | `use-cases/*.yaml` + `fixtures/*.yaml` (atomic) | `.harness/use-cases/`, `.harness/fixtures/` |
| `/create-sensors` | `UseCase`, applicable angles (pre-resolved), `stack-manifest.yaml` | N `sensor.yaml`s | `.harness/sensors/` |

## Dependencies

- Phase A entity packages (all 9), specifically:
  - `internal/stack` for stack-manifest type
  - `internal/usecase`, `internal/fixture`, `internal/entrypoint` for use-case + fixture validation
  - `internal/sensor` for sensor type + grounding/fixture-binding checks
  - `internal/policy` for resolving applicable angles (callers pass the result to `/create-sensors`; this chunk does not re-resolve)
- No dependency on B1–B3 or B5–B6.

## Open questions for `/brainstorming`

1. **Skill script transport.** Slash command invokes a Go binary in `scripts/`. Is that binary statically compiled and checked in, `go run`-launched, or invoked through the harness CLI (B6) once it exists? Recommendation: `go run` during development; pre-compile + checked-in once B6 lands. Avoid making B6 a hard prereq.
2. **`/detect-use-cases` atomicity enforcement.** Plan §5 says a use case without fixtures is rejected. Where does the rejection live — in the LLM's prompt (don't emit unpaired output) and the Go validator (refuse if it happens)? Recommendation: both belt-and-suspenders.
3. **Retry/repair budget.** Cap the loop at 2 retries (first repair includes the validator's error message in the prompt). Hard or per-skill?
4. **`/create-sensors` angle source.** Resolved applicable angles come from `policy.Resolve` (a Phase A API). Does this skill import `internal/policy` directly, or take a pre-resolved angle list as input? Recommendation: pre-resolved list — keeps the skill chain explicit.
5. **Confidence surfacing.** Plan §10.3 inferential confidence floor. Does the LLM emit a `confidence` field in its YAML output (so the validator records it on the manifest), or do we treat absence as 1.0? Recommendation: require `confidence`; reject output that omits it.
6. **`.harness/` layout.** Confirm the on-disk layout: `.harness/stack-manifest.yaml`, `.harness/use-cases/<id>.yaml`, `.harness/fixtures/<id>.yaml`, `.harness/sensors/<id>.yaml`. Lock it here so B5 and B6 can read from the same path.

## Deliverable acceptance

- `/detect-stack` against a Go HTTP API sample: LLM emits a `stack-manifest.yaml`; script validates against schema and writes to `.harness/stack-manifest.yaml`; archetype is `http-api`; confidence ≥ 0.7.
- `/detect-use-cases` against the same sample: paired `(use-case, fixtures)` per public entry point; every `{{ }}` token in use-case text resolves against the emitted fixtures + entry points (use `internal/usecase/template.Resolver` to verify).
- `/create-sensors` against one emitted use case + a default `EffectivePolicy`: one sensor per applicable angle; all sensors pass `internal/sensor.Grounding` and fixture-binding validators.
- Retry/repair test: simulate a deliberately-malformed LLM response → one repair-prompt round → success; second consecutive failure surfaces a typed error.
- No Go code in this chunk performs inference (assertion: searchable absence of words like "infer", "scan", "guess" in the validator packages — manual review on PR).
