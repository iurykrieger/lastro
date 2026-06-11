---
name: detect-use-cases
description: Scan the application's logic branches, condense them into journey-grouped use cases (success/failure/alternative), and report branch coverage. Run /detect-stack first if the manifest is missing.
---

# /detect-use-cases

You are detecting the use cases for the repository at the current working directory.
Detection is branch-driven: a deterministic engine inventories every logic branch in the
code; you condense those branches into journeys and write use cases that *cover* them;
a deterministic coverage metric scores the result.

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

## Prerequisite

`.harness/stack-manifest.yaml` must exist. If it is absent, stop and tell the user to run
`/detect-stack` first. Read the manifest to know the `archetype` before inspecting code.

## Step 1 — Scan branches (deterministic)

```bash
<plugin-root>/scripts/harness-tools.sh scan-branches --src . --harness-dir .harness
```

Then Read `.harness/branch-inventory.yaml`. Each entry is one decision point:
`{id, file, line, kind, condition, enclosing}`. Files marked `precision: heuristic`
have approximate counts; `ast` files are exact.

## Step 2 — Condense branches into journeys (inferential)

A **journey** is one user-meaningful flow through the application, anchored on a public
entry point (shaped by archetype: `http-api` → endpoint, `cli` → command,
`event-consumer` → listener, `worker`/`batch-job` → job handler). Group the inventory's
branches by the entry point whose handler/call-graph contains them — `enclosing` and
`file` tell you where each branch lives.

For each journey, plan its use cases from the branches:

- **success** (required) — the path where every validation passes; covers the
  happy-path branches (the `else`/fall-through of validation ifs, success cases).
- **failure** (required) — a rejected or erroring run; covers validation-failure
  branches (`if` guards that return errors, `catch`/error cases, `default` rejections).
- **alternative** — one per remaining *meaningful* branch cluster (a mode switch, an
  optional feature, a retry path). Don't force one per branch: condense branches that
  belong to the same observable behavior into a single use case.

A branch id names the *decision point*, not one side of it: a use case covers a
branch when its scenario **determines that decision's outcome** — the success use
case covers a validation `if` by driving its fall-through, the failure use case
covers the same id by triggering it. Never invent ids for "the happy side"; reuse
the decision point's id. A branch may be covered by multiple use cases; every
meaningful branch should be covered by at least one. Branches that are pure
defensive plumbing (unreachable panics, logging-only ifs) may stay uncovered —
you will justify them in Step 4.

## Step 3 — Emit fixtures and use cases

Write paired YAMLs per journey. **Write order is critical:** for each use case, write
ALL its fixtures before the use case itself, or persist fails with `fixture_binding`.

### Fixture YAML (`schemas/fixture.yaml`)

- `schema_version: 1.0.0`, `id` (`^[a-z][a-z0-9-]*$`), `use_case_id`, `role`
  (`input` | `expected-output` | `expected-side-effect`), `content_type`, `payload`,
  `binding`, `source_refs`. See `schemas/examples/fixture/*.yaml`.

```bash
<plugin-root>/scripts/harness-tools.sh detect-use-cases --type fixture --file /tmp/<fixture-id>.yaml --harness-dir .harness
```

### Use-case YAML (`schemas/use-case.yaml`)

- `schema_version: 2.0.0`, `id`, `title`, `archetype_scope: [<archetype>]`.
- `entry_points` — typed per archetype; see `schemas/examples/entry-point/<archetype>.yaml`.
- `given`/`when`/`then` — plain text with `${{fixtures.<id>}}` / `${{entry_points.<id>}}`.
- `fixture_ids: [...]` — every fixture referenced in the text.
- `journey: <journey-id>` — same id pattern (`^[a-z][a-z0-9-]*$`), kebab-cased from
  the entry point (e.g. `create-order`); the file lands in `.harness/use-cases/<journey>/`.
- `variation: success | failure | alternative`.
- `covers: [br-...]` — the branch ids from the inventory this scenario exercises.
  Every id must exist in the inventory or persist fails with `unknown_branch_ref`.
- `source_refs` — the implementation files behind the journey (sensors scan these).

```bash
<plugin-root>/scripts/harness-tools.sh detect-use-cases --type use-case --file /tmp/<use-case-id>.yaml --harness-dir .harness
```

## Step 4 — Measure coverage and iterate (deterministic)

```bash
<plugin-root>/scripts/harness-tools.sh coverage --harness-dir .harness
```

The JSON report (also written to `.harness/coverage.yaml`) gives `coverage_percent`,
per-journey rollups, and the `uncovered` branch list. For every uncovered branch either:

1. extend an existing use case's `covers` (re-emit it), or
2. add an `alternative`/`failure` use case for it, or
3. justify leaving it uncovered (defensive/unreachable/logging-only) — collect these.

Re-run `coverage` after each round. Stop when every remaining uncovered branch has a
justification. Report to the user: final `coverage_percent`, per-journey use-case
counts (each journey needs ≥1 success and ≥1 failure), and the justified-uncovered
list with one-line reasons.

## Exit code contract

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done with this write |
| 2 | Validation failure | Read JSON `persisterror.Error` from stdout; fix YAML; retry (cap 3) |
| 1 | Script-level error | Read stderr; surface to user; stop |

Common exit-2 `kind` values:

- `fixture_binding` — a referenced fixture is not on disk yet; write it, retry the use case.
- `unknown_branch_ref` — a `covers` id is not in the inventory; re-read the inventory
  (or re-run `scan-branches`) and fix the id.
- `missing_dependency` — `covers` is set but no `branch-inventory.yaml` exists; run Step 1.
- `unknown_enum_value` — bad `variation` (want `success`/`failure`/`alternative`).
- `schema_violation` — required field missing or wrong shape.

## Atomicity warning

Each write is per-entity. If a use-case write fails after some fixtures landed, fix and
retry, or remove the partial fixtures from `.harness/fixtures/` before giving up.
