# Harness Framework

> A use-case-driven validation framework that detects what an application *is* and *does*, then dynamically synthesizes angle-specific sensors to validate it.

**Status:** design phase. The framework specification is complete; implementation has not started. See [`docs/harness-framework/plan.md`](docs/harness-framework/plan.md) for the canonical design and [`docs/harness-framework/`](docs/harness-framework/) for the per-entity decomposition.

## The idea

Most validation tools take a fixed shape — "run these tests," "check this lint config," "scan for these CVEs." Harness inverts that: it first detects what the application is (its archetype and stack), reverse-engineers the behaviors it exhibits (use cases), then generates the sensors needed to validate each behavior from every angle that makes sense for that archetype.

The four primitives:

- **Use cases** describe *what* the application does, in `given / when / then` text. Tech-agnostic, no code, no assertions. They reference structured `entry_points` (HTTP routes, queue channels, CLI commands…) and `fixtures` (concrete I/O payloads) via `{{ }}` interpolation.
- **The stack** is the toolbox — detected libraries and capabilities. Sensors may only use what the toolbox provides.
- **Sensors** are the solution. One per `(use case × applicable validation angle)`. They orchestrate execution and emit signals; they do not solve the use case, they validate it.
- **Signals** are the contract with the LLM. Structured JSON, never raw logs. Every failure carries a `heal_hint` — a compressed, actionable instruction the LLM can act on.

## Validation angles

Each use case is validated from one or more angles, gated by a per-archetype `ValidationPolicy`:

`security`, `build`, `code-structure`, `unit-test`, `e2e-test`, `contracts`, `logs`, `metrics`, `database`, `performance`.

Angles are extensible only by framework version bump; users control *which* angles apply to their archetype, not *which angles exist*.

## How a run works

```
detect-stack              # what the app is built with (+ archetype)
  └─→ detect-use-cases    # what the app does (+ fixtures, paired atomically)
       └─→ create-sensors # one pass, generates all applicable-angle sensors
            └─→ validate-use-case
                 ├─→ run-sensor (assertion)         → Signals + AggregateSignal
                 ├─→ start-sensor / stop-sensor     → AggregateSignal at stop
                 └─→ aggregate per use case         → use case verdict
                      └─→ heal (on failure)         → fix proposal + re-validate
```

Each sensor terminates with exactly one `AggregateSignal`. The per-use-case aggregator gates on obligatory angles defined in the `ValidationPolicy`.

## Design principles

- **Determinism over prediction.** Anything that can be deterministic Go code is deterministic Go code. LLM inference is reserved for detection and sensor generation.
- **Schema-first.** All entities are YAML with a `schema_version` field. Filenames are unversioned; versioning is data, not filename metadata.
- **Grounded generation.** Sensors may only reference stack components the detector actually found. No phantom dependencies.
- **Self-healing as a first-class signal.** A `heal_hint` is required on every failing signal. The LLM eats structured instructions, not log dumps.
- **Dogfood.** The framework validates itself — its own use cases drive its own sensors.

## Implementation conventions

- **Language:** Go. Every package ships with tests; no untested code lands.
- **Skill size budget:** each skill is 200 lines maximum; synthesize past 100.
- **Skill scripts layout:** each skill carries its own `scripts/` folder; reusable logic lives in shared `lib/` packages.
- **Libraries:** prefer mature, widely-used Go libraries before writing bespoke implementations. Custom code, when required, is structured and reviewed.
- **Language of artifacts:** English (US), always — for code, docs, schemas, skills, commits, and comments.

## Repository layout (planned)

```
cmd/harness/           CLI entry point
schemas/               YAML schemas + enums + golden examples
internal/              Go packages (detect, sensors, runtime, lifecycle, schema, policy)
skills/                Slash-command primitives, each with scripts/
lib/                   Shared Go code reused across skill scripts
examples/              Sample repos exercised by the framework
docs/                  Design documents (plan, per-entity decomposition)
```

## Documentation

- [`docs/harness-framework/plan.md`](docs/harness-framework/plan.md) — the canonical specification. Read this first.
- [`docs/harness-framework/README.md`](docs/harness-framework/README.md) — parallel implementation decomposition.
- [`docs/harness-framework/00-schema-freeze.md`](docs/harness-framework/00-schema-freeze.md) — the sequential gate that must land before parallel entity work.
- `E1`…`E9` — per-entity brainstorming starters (enums, stack component, entry point, use case, fixture, sensor, signal, aggregate signal, validation policy).

## License

To be determined.
