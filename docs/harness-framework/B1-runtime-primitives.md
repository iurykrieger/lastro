# B1 — Runtime Primitives

> Source plan: [`plan.md`](plan.md) §6.1 (Runtime components), §6.3 (Verdict aggregation), §4.7 (Validation Policy resolution)

The four self-contained primitives the rest of the runtime depends on: template interpolation, policy resolution, sensor DAG resolution, signal collection. Each is a small Go package with no cross-dependency on the others.

## Branching (mandatory)

Before starting any work on this chunk:

```bash
git fetch origin
git checkout -b feat/b1-runtime-primitives origin/main
```

Do **not** start from a stale `main` or an in-flight branch. Phase B chunks land in parallel; another chunk's branch may not be merged yet, and inheriting its tree will cause silent conflicts.

## Parallelism

- **Can run in parallel with:** B5 (detection + generation — totally separate code path).
- **Must run after:** Phase A (✓ — all entity packages already live in `internal/`).
- **Blocks:** B2 (composed runtime imports template, signalCollector, policy), B3 (executor imports resolver + template + signalCollector), B4 (heal loop imports policy), B6 (validate-use-case skill imports policy), B7 (CLI imports policy).

## Scope

In:
- `internal/runtime/template/` — `{{ fixtures.<id> }}` / `{{ entry_points.<id> }}` interpolation against a UseCase + its fixtures + its entry points. Pure string transform; no I/O.
- `internal/policy/` — load a `ValidationPolicy`, apply override merge (`repo > org > global`), expose `Resolved(useCase) → {obligatory, optional, disabled}` per angle.
- `internal/runtime/resolver/` — given a set of `Sensor`s, topologically sort by `depends_on`; fail fast on cycles; return execution order.
- `internal/runtime/signalCollector/` — read JSON Lines from an `io.Reader` (sensor stdout), validate each line against the `Signal` schema, hand off to a sink (channel or callback). One implementation; no aggregation logic.

Out:
- Per-sensor and per-use-case aggregation (B2).
- Fixture binding (B2 — depends on template).
- Step execution (B3).
- Anything that depends on more than one of these primitives (B2/B3).

## Inputs / Outputs

| Package | Input | Output |
|---|---|---|
| `template` | `UseCase`, `[]Fixture`, `[]EntryPoint`, raw template string | resolved string OR `UnresolvedRefError` |
| `policy` | `ValidationPolicy` files (repo/org/global), `UseCase` | `Resolved{Obligatory, Optional, Disabled []ValidationAngle}` |
| `resolver` | `[]Sensor` | `[]Sensor` in topological order, OR `CycleError` |
| `signalCollector` | `io.Reader`, `chan<- Signal`, `context.Context` | streamed `Signal`s + terminal error/EOF |

## Dependencies

- Phase A: `internal/enums`, `internal/usecase`, `internal/fixture`, `internal/entrypoint`, `internal/sensor`, `internal/signal`, `internal/policy` (entity types only — this chunk *adds* the resolution logic to that package; check current state of `internal/policy/` before starting).
- External libraries: prefer `github.com/santhosh-tekuri/jsonschema/v6` (already in use for schema validation) for signal validation; standard library `text/template` is too loose — implement custom `{{ namespace.id }}` resolver (closed grammar).

## Open questions for `/brainstorming`

1. **Template grammar.** Closed grammar (`{{ fixtures.<id> }}` and `{{ entry_points.<id> }}` only, hard error on anything else) or general-purpose with `text/template`? Recommendation: closed grammar. Plan §2 says use case text is tech-agnostic — open templates invite drift.
2. **Policy override semantics.** Per §4.7 the merge is `repo > org > global`. Is the merge per-angle (an angle's list is fully replaced by the higher-precedence layer) or per-list-entry (union/diff)? Recommendation: per-angle replacement — simpler to reason about, matches the YAML shape.
3. **Resolver algorithm.** Kahn's vs DFS. Both detect cycles. Recommendation: Kahn — yields topological order directly.
4. **Cross-use-case `depends_on`.** Plan doesn't forbid a sensor depending on a sensor from a different use case. Recommendation: allow; resolver works on the full sensor set, not per-use-case. Aggregation (B2) is per-use-case.
5. **Signal collector buffering.** Channel-based (back-pressure friendly) or callback (simpler, blocking)? Recommendation: channel + `context.Context` for cancellation — matches executor's likely shape.

## Deliverable acceptance

- All four packages compile, tests pass, ≥1 negative test per package (unresolved template ref, missing policy file, sensor cycle, malformed JSONL line).
- Template resolver rejects an unknown namespace with a typed error containing the offending token.
- Policy resolver loads a 3-layer fixture (global + org + repo) and returns the expected per-angle resolution for a sample use case.
- Resolver returns a deterministic topological order on a synthetic 5-node graph; rejects a cycle with the offending node ids in the error.
- Signal collector streams 100 well-formed signals and one malformed line; the malformed line surfaces as a typed error without halting the stream.
