# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project status

This repo currently contains **planning documents only** — no Go code, no `go.mod`, no skills implemented yet. The canonical source of truth is [`docs/harness-framework/plan.md`](docs/harness-framework/plan.md). Per-entity decomposition lives in [`docs/harness-framework/`](docs/harness-framework/) (`E1-enums.md` … `E9-validation-policy.md`), gated by [`00-schema-freeze.md`](docs/harness-framework/00-schema-freeze.md).

Read the plan before touching anything else. Every section below is a compression of it; the plan wins on disagreement.

## What this framework is

A **use-case-driven validation framework**. The flow:

1. Detect what the application *is* (`archetype` + stack) and *does* (`UseCases` with archetype-typed `EntryPoints` and `Fixtures`).
2. Synthesize `Sensors` per `(UseCase × applicable ValidationAngle)`, grounded in the detected stack.
3. Run sensors, collect `Signals` (JSON Lines), emit one terminal `AggregateSignal` per sensor run.
4. Aggregate signals per use case against a `ValidationPolicy` (obligatory/optional/disabled angles per archetype).
5. On failure, feed `heal_hint` blocks into `/heal` to propose fixes; re-validate affected sensors only.

Use cases describe *what* in `given/when/then` text with `${{fixtures.<id>}}` / `${{entry_points.<id>}}` interpolation. They contain **no code, no assertions, no orchestration**. Sensors do all the executing.

## Core invariants (do not violate)

- **Use case text is tech-agnostic.** No code, regex, library names, or wire-protocol details beyond what an archetype's surface naturally requires.
- **Sensors are grounded.** Top-level `uses:` may reference only `StackComponent` ids from the detected stack manifest.
- **Fixtures bind per-step.** Step `uses:` references `Fixture` ids owned by the sensor's use case. No sensor-wide fixture list.
- **Every sensor emits exactly one terminal `AggregateSignal`** regardless of `output_type` (single-shot or stream) or `kind` (assertion or observational).
- **`heal_hint` is required when `verdict = fail`** on any `Signal` or `AggregateSignal`. It is the contract with the LLM — structured, actionable, never raw logs.
- **Schema filenames are unversioned** (`signal.yaml`, not `signal-v1.yaml`). `schema_version` lives *inside* the schema.
- **One generation skill for sensors.** `create-sensors` (plural). It fans out across all applicable angles in one pass. No singular `create-sensor`.

## Framework rules (project-specific)

1. **Go only.** All framework code is written in Go. Every package has tests; no untested code lands.
2. **Determinism beats prediction.** If a behavior can be implemented deterministically, write deterministic Go for it and let the LLM only orchestrate. Reserve LLM inference for detection and generation (skills marked *inferential* in the plan's skill table). Runtime components — resolver, executor, fixture binder, signal collector, aggregator — are deterministic.
3. **Skill scripts layout.** Each skill directory contains a `scripts/` folder for skill-specific code. Code reused across two or more skills moves to a shared `lib/` (Go package) and is imported. Do not duplicate logic between skill scripts.
4. **Skill size budget.** A skill file is **200 lines max**. If it grows past **100 lines**, synthesize — sharpen instructions, drop redundant prose, push procedural detail into scripts under the skill's `scripts/` folder.
5. **Prefer mature Go libraries over bespoke code.** Pick well-maintained, widely-used libraries from the Go open-source ecosystem (YAML, JSON Schema, CLI scaffolding, structured logging, etc.) before writing custom implementations. When a custom implementation is genuinely required, craft it with clear separation of concerns and a documented design.
6. **English (US) only in artifacts.** All framework code, schemas, docs, skill text, commit messages, and inline comments are written in en-US — even when working sessions or external discussions happen in another language.
7. **Dogfood the framework.** Use the harness framework to detect its own use cases and synthesize sensors that validate its own behavior. Self-validation is a first-class deliverable, not an afterthought.

## Big-picture architecture

```
detect-stack          → stack-manifest.yaml (includes archetype)
detect-use-cases      → use-cases/*.yaml + fixtures/*.yaml   (paired, atomic)
create-sensors        → sensors/*.yaml                       (one per applicable angle)
run-sensor            → Signal stream + terminal AggregateSignal   (kind: assertion)
start-sensor / stop-sensor → AggregateSignal at stop          (kind: observational)
validate-use-case     → aggregated per-use-case verdict
heal                  → code edit proposal, then selective re-validate
```

Two aggregator layers: **per-sensor** (rolls up streamed signals into the terminal `AggregateSignal`) and **per-use-case** (computes use case verdict from each sensor's `AggregateSignal`, gated by `ValidationPolicy`).

## Planned repository layout

The target layout (from plan §8) is:

```
cmd/harness/           # CLI main package
schemas/               # YAML schemas + enums + golden examples
internal/
  detect/              # detect-stack, detect-use-cases logic
  sensors/             # create-sensors generation
  runtime/             # resolver, template, executor, fixtureBinder,
                       # signalCollector, aggregator, healLoop
  lifecycle/           # run-sensor, start-sensor, stop-sensor
  schema/              # schema loading + validation
  policy/              # ValidationPolicy resolution
skills/                # AI primitive definitions (slash commands),
                       # each with its own scripts/ folder
lib/                   # shared Go code reused across skill scripts
examples/              # sample repos exercised by the framework
```

**Test convention:** every `internal/` package has a sibling `_test.go`. No package lands without tests.

## Phased roadmap (current position: Phase 0)

- **Phase 0 — Foundations:** lock the entity model (the plan), write all schemas under `/schemas/` with one passing example each, define fixed enums as YAML.
- **Phase 1 — Detection:** `/detect-stack` (archetype + stack), `/detect-use-cases` (paired use cases + fixtures).
- **Phase 2 — Sensor generation:** `/create-sensors` with grounding and fixture-binding validation.
- **Phase 3 — Runtime (assertion path):** resolver, executor, fixture binder, signal collector, per-sensor + per-use-case aggregators, `/run-sensor`.
- **Phase 4 — Observational sensors:** `/start-sensor`, `/stop-sensor`, log-pattern derivation, completeness reporting.
- **Phase 5 — Heal loop:** `/heal`, selective re-validation.
- **Phase 6 — DX:** CLI (`harness detect`, `harness validate`, `harness heal`), reports, detection caching.

Open design questions are tracked in plan §10 (sensor/fixture regeneration policy, inferential confidence floor, heal loop termination, sensor identity hashing, in-place vs clean migration). Resolve before closing Phase 0.

## Acceptance criteria for "framework works"

See plan §11. The short version: on a fresh sample repo, the chain `detect-stack → detect-use-cases → create-sensors → validate-use-case` produces aggregated signals; a deliberately broken sample produces a `heal_hint` actionable enough that `/heal` fixes it on the first attempt; the same fixture is reused across at least two angles for the same use case.

## Working in this repo

- Before designing any Phase A entity, read both [`plan.md`](docs/harness-framework/plan.md) and the entity's decomposition doc (`E1`…`E9`).
- The schema-freeze gate (`00-schema-freeze.md`) must land **before** any entity Go code, so cross-entity field names and reference shapes don't drift across parallel work.
- When adding a skill, check whether logic belongs in the skill's `scripts/` (skill-local) or `lib/` (shared). When in doubt, start local; promote on the second caller.
- When a design choice could go either deterministic or LLM-driven, default to deterministic and document the rationale if you pick LLM.
