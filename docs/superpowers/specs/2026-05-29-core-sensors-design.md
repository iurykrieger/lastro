# Core Sensors (repo-level `scope`): Design Spec

> Source: GitHub issue [#24](https://github.com/iurykrieger/lastro/issues/24) (related: #21, #23, #22).
> Source plan: [`plan.md`](../../harness-framework/plan.md) §4.4, §6.1; [`E6-sensor.md`](../../harness-framework/E6-sensor.md) Q4; B5 plan [`2026-05-25-b5-validate-use-case.md`](../plans/2026-05-25-b5-validate-use-case.md).
> Status: drafted 2026-05-29, awaiting written-spec review.

## 1. Purpose

Introduce **core sensors** — repo-level, use-case-agnostic sensors that establish a runnable, validatable
environment (boot the app, connect the datastore, compile, run the suite) — and make per-use-case angle
sensors depend on them. This closes the structural gap behind #24: today every angle sensor that needs a
live environment re-implements (and mis-implements) the boot step, and the runtime's `depends_on` DAG —
already built and tested — has no roots to hang off.

The change is expressed through a single new discriminator on `Sensor`: **`scope: core | use-case`**.

## 2. Root cause (why this is structural, not a one-line fix)

Core/base sensors are impossible to express today across three layers:

1. **Schema** (`schemas/sensor.yaml`): `Sensor` requires both `use_case_id` and `angle`; `angle` is a closed
   enum of ten use-case-facing facets. A repo-level boot sensor has no use case and no matching angle.
2. **Generation** (`/create-sensors`): scoped strictly per `(use-case × angle)`, emits one sensor per angle,
   never reads the stack manifest for repo-level environment, never wires `depends_on`.
3. **Aggregation** (`/validate-use-case`): *"Only sensors whose `use_case_id` matches participate.
   Cross-use-case `depends_on` is ignored."* A shared core sensor would never be pulled into a use case's run.

The runtime machinery the issue wants (DAG roots, skip dependents on a failed prerequisite with a single
clear signal) **already exists** in `internal/sensor` (resolver) and `lib/skillruntime/scheduler.go`. It is
unreachable only because core sensors can't be modeled, generated, or gathered.

## 3. Scope

In:
- `schemas/sensor.yaml`: add `scope` discriminator; make `use_case_id` conditional on `scope`; add a new
  `environment` value to the angle enum.
- `schemas/enums/validation-angles.yaml`: add `environment` (canonical source for the new enum value).
- Update the `00-schema-freeze.md` gate to record the schema change **before** any Go code lands.
- `internal/enums/`: add `AngleEnvironment` constant and include it in `AllAngles()` (the `drift_test.go`
  guard at line 77 asserts `AllAngles()` equals the YAML enum — it breaks otherwise); add a `SensorScope`
  enum (`core | use-case`) with its own drift fixture if a `sensor-scopes.yaml` enum file is introduced
  (see §11 Q1). **Do NOT add `environment` to `ApplicableAngles`** (see decision #11).
- `internal/sensor/`: `Scope` field + type, conditional intrinsic validators, slug-uniqueness invariant,
  loader walking the new folder layout, store indexing by scope.
- New skill `/create-core-sensors`: emits core sensors from the stack manifest alone (one per applicable
  angle + `environment` primitives), wiring the intra-core DAG.
- `/create-sensors <uc>`: resolves and wires `depends_on` from each use-case angle sensor to the core sensor
  of the same angle (resolution **by angle**, not by name pattern).
- `/validate-use-case` gather — **two sites** filter by `use_case_id` today and both must change:
  `skills/validate-use-case/scripts/main.go` (~line 80, `b.Sensors.ForUseCase(useCaseID)`) and
  `cmd/harness/usecase_runner.go` (~line 170, `Sensors.All()` filtered by `s.UseCaseID == useCaseID`).
  Replace each with "use case's own sensors + transitive `depends_on` closure into `core` scope".
- Folder-layout migration: use-case sensors → `.harness/sensors/<usecase-id>/`, core sensors →
  `.harness/sensors/core/`.
- Tests for every touched `internal/` package; dogfood run.

Out:
- **Command-grounding quality** (`run-dev` resolving `make -C charge-api dev` not `make dev`; eliminating the
  catch-all `harness` command) — that is #21/#23. This spec delivers the *structure*; #24's boot fix *lands*
  on the `run-dev` core sensor, but full grounding correctness is tracked separately. See §9.
- Heal-loop behavior on setup failures (#22).
- Sensor `id` derivation strategy beyond the slug-uniqueness invariant (content-hash vs slug — plan §10.5).

## 4. Decisions captured from brainstorm

| # | Decision | Rationale |
|---|---|---|
| 1 | **Model core sensors via `scope: core \| use-case`**, default `core`→explicit, `use-case` as the schema default. | One discriminator, backward-compatible: every existing sensor omits `scope` and is treated as `use-case`. Avoids a separate `BaseSensor` entity (more surface) and avoids overloading the angle enum as the *only* signal. |
| 2 | **`scope` governs `use_case_id`; `angle` stays required for both scopes.** `scope: use-case` → `use_case_id` required. `scope: core` → `use_case_id` forbidden. | The user's model: "one core sensor for each angle available to the archetype." Core sensors carry an angle so the `depends_on` wiring is trivial and angle-driven. |
| 3 | **Add `environment` as an 11th angle value**, used by infra primitives (`run-dev`, `datastore`). | The boot/datastore primitives don't map to one of the ten graded facets. `environment` is the canonical home. It is added to the **angle enum only** — NOT to the ValidationPolicy `AngleList`. |
| 4 | **`environment` is not policy-graded.** | Environment primitives are DAG preconditions, not facets a policy marks obligatory/optional. Their failure skips dependents via `depends_on` and promotes the worst-verdict exit code — it never reads as a "missing obligatory angle". |
| 5 | **Core sensors form a two-level intra-`core` DAG.** `run-dev` (and `datastore`) are shared `environment` primitives; angle core sensors that need a live environment `depends_on` them. | Honors "one core sensor per angle" while killing the boot duplication that motivated #24 — boot is defined once in `run-dev`. |
| 6 | **Separate skill `/create-core-sensors`** (no argument; reads the stack manifest). `/create-sensors <uc>` only wires `depends_on`. | Core generation is repo-level and runs once; per-use-case generation runs per use case. Clean split, no re-derivation of core sensors on every use case. |
| 7 | **`depends_on` wiring resolves the core sensor by `angle`**, not by string-matching the id. "The use case's angle-X sensor depends on the core sensor whose `angle == X`." | Robust to the no-prefix id convention (#9); no brittle name patterns. |
| 8 | **`/validate-use-case` includes `core`-scope sensors reachable via the `depends_on` closure** (use-case→core and core→core edges). use-case→use-case edges stay out. | Reverts to the E6 Q4 *global-DAG* recommendation, bounded to `core` scope, so it enables core roots without re-introducing the cross-use-case coupling B5 deliberately avoided. The `internal/sensor` resolver already allows cross-edges (E6 spec decision #8); only the B5 *gather* rule changes. |
| 9 | **IDs are meaningful slugs, no mechanical prefix** (`s-`/`base-`). Generator must guarantee **global** slug uniqueness (depends_on references global ids). | User preference. Core: `run-dev`, `build`, `database-query`. Use-case: embed `<usecase-id>` in the slug for uniqueness, e.g. `create-card-charge-e2e-test`. |
| 10 | **Folder layout:** use-case sensors in `.harness/sensors/<usecase-id>/<slug>.yaml`; core sensors in `.harness/sensors/core/<slug>.yaml`. | Replaces today's flat `.harness/sensors/*.yaml`. Makes the `validate-use-case` gather trivial: read `<uc>/` + `core/`. Requires migrating persist/loader/gather. |
| 11 | **`environment` is core-only: it is NOT added to `ApplicableAngles`, and the angle-applicability check applies only to `scope: use-case` sensors.** | `internal/enums/archetype_angles.go` `ApplicableAngles` drives `/create-sensors`' `angle_not_applicable` rejection. Adding `environment` there would make `/create-sensors` emit unwanted per-use-case `environment` sensors. Keeping it out, **and** scoping the applicability check to `use-case` sensors, lets core sensors carry `environment` (and any applicable angle) without tripping the check. |

## 5. Schema changes

`schemas/sensor.yaml`:

```yaml
properties:
  scope:
    type: string
    description: "Canonical: core sensors are repo-level; use-case sensors bind to a use case."
    enum: [core, use-case]
    default: use-case
  angle:
    enum: [security, build, code-structure, unit-test, e2e-test,
           contracts, logs, metrics, database, performance, environment]   # + environment
  # ... existing fields unchanged ...

# Conditional requirement via if/then:
allOf:
  - if:
      properties: { scope: { const: use-case } }
    then:
      required: [use_case_id]
  - if:
      properties: { scope: { const: core } }
    then:
      not: { required: [use_case_id] }     # use_case_id forbidden for core scope
```

`required` at the top level drops `use_case_id` (now conditional) but keeps `schema_version, id, angle, kind,
nature, output_type, uses, steps`.

`schemas/enums/validation-angles.yaml`: append `environment` with purpose
*"Repo-level environment preconditions: app boot, datastore reachability."*

`schemas/validation-policy.yaml`: **unchanged** — `AngleList` keeps the ten graded angles; `environment` is
intentionally absent.

**Backward compatibility:** every existing sensor and golden example omits `scope` → defaults to `use-case`
→ `use_case_id` still required for them. No existing artifact breaks.

## 6. `internal/sensor` changes

- Add `Scope enums.SensorScope` (new enum `core | use-case`) to the `Sensor` struct; default to `use-case`
  on load when absent.
- Intrinsic validators (in `validate.go`):
  - `scope: use-case` && `use_case_id == ""` → error.
  - `scope: core` && `use_case_id != ""` → error.
  - existing self-dependency check unchanged.
- **Slug-uniqueness invariant** enforced at `Store` build time: duplicate global `id` → error (today ids were
  unique by the `s-<uc>-<angle>` convention; the no-prefix convention makes this an explicit check).
- Loader walks `.harness/sensors/**` (one level of subfolders: `<usecase-id>/` and `core/`) instead of a flat
  directory. `Store` indexes `byScope` in addition to `byUseCase`.
- `ResolveExecutionOrder` is unchanged (it already operates on the global slice and allows cross-edges —
  E6 design decision #8). The gather rule itself does **not** live in `internal/sensor`; it lives in the two
  call sites named in §3 (`skills/validate-use-case/scripts/main.go`, `cmd/harness/usecase_runner.go`). Note
  this change is **load-bearing**, not cosmetic: today, gathering only `ForUseCase` sensors while a use-case
  sensor carries a `depends_on` into a core sensor would make `ResolveExecutionOrder`'s dangling-edge
  pre-check fail and exit 3 (`scheduler-failed`). The closure-based gather is what keeps the edge resolvable.

## 7. Generation

**`/create-core-sensors`** (new, no argument):
1. Read `.harness/stack-manifest.yaml`: `applicable_angles`, `components[*].id`, datastore/boot components.
2. Emit one core sensor per applicable angle into `.harness/sensors/core/<slug>.yaml`, plus `environment`
   primitives (`run-dev`, and `datastore` when the datastore is not booted by `run-dev`).
3. Wire the intra-core DAG: `e2e-test`, `logs`, `metrics`, `performance` → `depends_on: [run-dev]`;
   `database` → `depends_on: [datastore]` (or `[run-dev]` if the datastore boots with the app); `build`,
   `unit-test`, `code-structure`, `security`, `contracts` are self-contained.
4. Each core sensor's top-level `uses:` references real `stack-manifest` component ids (grounding invariant).
5. Run commands derive from detected stack (Makefile targets incl. submodule paths, health endpoint, test
   command). Command-grounding *correctness* is bounded by #21/#23 — see §9.

**`/create-sensors <uc>`** (extended): after emitting per-use-case angle sensors, for each one resolve the
core sensor whose `angle` matches and append it to `depends_on`. If no core sensor exists for that angle
(e.g. core sensors not yet generated), emit the use-case sensor without the edge and warn the user to run
`/create-core-sensors`.

## 8. `/validate-use-case` inclusion rule

Replace the constraint *"Only sensors whose `use_case_id` matches participate; cross-use-case `depends_on` is
ignored"* with:

> Gather = sensors in `.harness/sensors/<uc>/` **plus** the transitive `depends_on` closure of those sensors
> restricted to `scope: core` (i.e. follow use-case→core and core→core edges; do not follow
> use-case→use-case edges).

Then `ResolveExecutionOrder` over the gathered slice, schedule honoring `depends_on`. A failed core root
(e.g. `run-dev`) yields one `fail` `AggregateSignal` and its dependents get the existing synthetic
`inconclusive`/`stopped` aggregate with `heal_hint.summary = "skipped: depends_on <id> failed"`. The
per-use-case aggregator groups all gathered sensors by angle; `environment`-angle sensors are present in the
run but not graded by policy (their effect is via dependent-skipping + worst-verdict exit promotion).

## 9. Boundary with #21 / #23 / #22

- **#24 (this spec):** the *structure* — `scope`, core sensors, the intra-core DAG, the gather rule.
- **#21/#23:** the *command quality* — grounding generated `run:` commands in real stack commands and
  removing the inert `harness` catch-all. The `run-dev` core sensor is where #24's boot fix lands, but its
  command must be correctly grounded by the #21/#23 work to actually boot `charge-api`.
- **#22:** heal triggering on setup failures. Out of scope here.

A green dogfood for #24 means *the DAG wires and skips correctly*; a fully green real-repo run additionally
needs #21/#23.

## 10. Testing

- `internal/sensor`: positive + negative for each new invariant (`core` + `use_case_id` present → error;
  `use-case` + missing `use_case_id` → error; duplicate global slug → error; loader reads both subfolder
  shapes; store `byScope` index). Golden examples gain a `core` sensor per archetype.
- Generation scripts: `/create-core-sensors` emits the expected core set + DAG from a fixture stack manifest;
  `/create-sensors` appends the correct `depends_on` edge resolved by angle.
- `/validate-use-case` gather: a use case with an `e2e-test` sensor depending on a `core/run-dev` that fails
  → e2e sensor is skipped with the expected `heal_hint.summary`; cycle in the core DAG → exit 3.
- **Dogfood:** `/detect-stack → /create-core-sensors → /create-sensors → /validate-use-case` on this repo and
  on a sample, asserting core sensors are the DAG roots and a single failed root skips its dependents with one
  signal.

## 11. Open questions for the plan / freeze gate

1. **Scope enum location.** New `schemas/enums/sensor-scopes.yaml` (mirroring `sensor-kinds.yaml`) vs inline
   enum in `sensor.yaml` only. Recommendation: dedicated enum file for consistency with the other sensor
   facets.
2. **`datastore` primitive vs folding into `run-dev`.** Whether the datastore is a separate `environment`
   primitive or part of `run-dev` is stack-dependent (docker-compose brings both up vs external datastore).
   Recommendation: generator decides per stack manifest; both shapes supported by the schema.
3. **Does the schema-freeze gate treat adding an enum value + a conditional-required field as a freeze
   change requiring sign-off?** It touches a frozen schema → yes, record in `00-schema-freeze.md` first.
4. **Migration of existing `.harness/sensors/*.yaml` into per-use-case folders — not hypothetical.** Real
   flat files exist on disk today: `.harness/sensors/uc-harness-validate-use-case-build.yaml` and
   `.harness/sensors/uc-harness-validate-use-case-unit-test.yaml` (the dogfood sensors). The plan must pick a
   concrete handling — in-place move into `.harness/sensors/<usecase-id>/` vs regenerate — and the loader must
   tolerate the transition. Tie to plan §10's clean-vs-in-place migration question.

## 12. Acceptance criteria

- A sensor with `scope: core` and no `use_case_id` loads and validates; with a `use_case_id` it is rejected.
- An existing sensor with no `scope` still loads as `use-case` and still requires `use_case_id`.
- `/create-core-sensors` produces `.harness/sensors/core/run-dev.yaml` (+ the angle core set) with a correct
  intra-core DAG, grounded `uses:`.
- `/create-sensors <uc>` wires each use-case angle sensor to the matching core sensor by angle.
- `/validate-use-case <uc>` gathers core roots, and a failed `run-dev` skips e2e/logs/metrics/performance with
  one clear signal instead of N independent boot failures.
- All touched `internal/` packages have passing tests; the dogfood chain runs end-to-end.
