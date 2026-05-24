---
name: detect-use-cases
description: Read .harness/stack-manifest.yaml, identify public entry points, and write paired use-case + fixture YAML files to .harness/. Run /detect-stack first if the manifest is missing.
---

# /detect-use-cases

You are identifying the use cases for the repository at the current working directory and writing
paired use-case and fixture YAML files to `.harness/`.

## Prerequisite

`.harness/stack-manifest.yaml` must exist. If it is absent, stop and tell the user to run
`/detect-stack` first. Read the manifest to know the `archetype` before inspecting the code.

## What to identify

For each public entry point in the codebase — shaped by archetype:

- `http-api` → HTTP endpoints (routes, handlers)
- `cli` → top-level commands and subcommands
- `event-consumer` → queue/topic listeners
- `event-producer` → publish calls
- `worker` / `batch-job` → job handlers

Each entry point maps to one candidate use case with a `given/when/then` scenario.

## What to emit

### Fixture YAML (`schemas/fixture.yaml`)

- `schema_version: 1.0.0`
- `id` — lowercase, hyphenated (`^[a-z][a-z0-9-]*$`); e.g., `valid-login-payload`.
- `use_case_id` — must match the owning use case's `id`.
- `role` — one of `input`, `expected-output`, `expected-side-effect`.
- `content_type`, `payload`, `binding`, `source_refs`.
- See `schemas/examples/fixture/*.yaml` for shape.

### Use-case YAML (`schemas/use-case.yaml`)

- `schema_version: 2.0.0`
- `id` — lowercase, hyphenated (`^[a-z][a-z0-9-]*$`); e.g., `user-login`.
- `title`, `archetype_scope: [<archetype>]`.
- `entry_points` — typed per archetype; see `schemas/examples/entry-point/<archetype>.yaml`.
- `given`, `when`, `then` — plain text using `{{fixtures.<fixture-id>}}` and
  `{{entry_points.<entry-point-id>}}` for interpolation.
- `fixture_ids: [...]` — list every fixture referenced in the scenario text.

## Write order is critical

For **each** use case, write ALL its fixtures before writing the use case itself. Writing the
use case before its fixtures causes a `fixture_binding` validation failure.

**Per fixture:**
```bash
go run ./skills/detect-use-cases/scripts/ --type fixture --file /tmp/<fixture-id>.yaml --harness-dir .harness
```

**Per use case (after all its fixtures):**
```bash
go run ./skills/detect-use-cases/scripts/ --type use-case --file /tmp/<use-case-id>.yaml --harness-dir .harness
```

## Exit code contract

| Exit | Meaning | Action |
|------|---------|--------|
| 0 | Success | Done with this write |
| 2 | Validation failure | Read JSON `persisterror.Error` from stdout; fix YAML; retry (cap 3) |
| 1 | Script-level error | Read stderr; surface to user; stop |

**Exit 2 JSON shape:**
```json
{"kind":"fixture_binding","entity_type":"use-case","path":"...","value":"...","expected":"...","message":"..."}
```

If `kind` is `fixture_binding`, a referenced fixture was not written yet — write it and retry the
use case. Other common kinds: `schema_violation`, `unknown_enum_value`.

## Atomicity warning

Each write is per-entity. If a use-case write fails after some fixtures have already landed,
either fix and retry, or remove the partial fixture files from `.harness/fixtures/` to keep
state clean before giving up.
