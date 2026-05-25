---
name: detect-stack
description: Detect the project's archetype and stack components, then write the result to .harness/stack-manifest.yaml. Use when starting work on a repo the harness framework hasn't analyzed yet.
---

# /detect-stack

You are detecting the stack of the repository at the current working directory and producing a `stack-manifest.yaml`.

## What to inspect

- Dependency manifests: `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, `Gemfile`, `pom.xml`, etc.
- Directory structure for archetype hints:
  - `cmd/<x>/main.go` → likely `cli`
  - HTTP route handlers → likely `http-api`
  - Queue/topic listeners → `event-consumer`
  - Pre-rendered HTML/CSS in a build dir → `static-site`
- Framework conventions visible in source files (e.g., `gin.Default()`, `express()`, `FastAPI()`).

## What to emit

A single YAML file matching `schemas/stack-manifest.yaml`. Required fields:

- `schema_version: 1.0.0` — the script patch-bumps this on subsequent re-runs.
- `archetype` — one of the enum values in `schemas/enums/archetypes.yaml`:
  `http-api`, `event-consumer`, `event-producer`, `cli`, `sdk`, `library`, `worker`, `batch-job`, `static-site`.
- `components` — a non-empty list of `StackComponent` entries, each with:
  - `schema_version: 1.0.0`
  - `id` — lowercase, hyphenated (`^[a-z][a-z0-9-]*$`); e.g., `express`, `gin`, `gorm`.
  - `kind` — one of `library`, `runtime`, `framework`, `datastore`, `protocol`, `tool`.
  - `name`, `version`, `capabilities` (list of strings), `detection_evidence` (list of `{file, path}`).
  - See `schemas/examples/stack-component/*.yaml` for shape per kind.

**DO NOT emit `applicable_angles`** — the script injects it from the archetype.

## How to write

1. Use the Write tool to put your YAML at `/tmp/stack-manifest.yaml`.
2. Run:
   ```bash
   go run ./skills/detect-stack/scripts/ --file /tmp/stack-manifest.yaml --harness-dir .harness
   ```
3. **If exit code is 0:** the manifest has been written to `.harness/stack-manifest.yaml` with
   `applicable_angles` populated. You are done.
4. **If exit code is 2:** read the JSON error from stdout. It has shape:
   ```json
   {"kind":"schema_violation","entity_type":"stack-manifest","path":"...","value":"...","expected":"...","message":"..."}
   ```
   Common kinds you may encounter:
   - `schema_violation` — a required field is missing or a value is wrong shape.
   - `unknown_enum_value` — archetype, kind, or other enum is invalid.

   Fix the YAML at `/tmp/stack-manifest.yaml`, then re-run the script.
   **Stop after 3 attempts** and report the unresolved error to the user.
5. **If exit code is 1:** a script-level error (bad args, unreadable file) — read stderr and
   report to the user.
