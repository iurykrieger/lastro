---
name: detect-environment
description: Detect how the application boots — every piece of software it depends on to execute — and write .harness/environment-model.yaml. Run /detect-stack first. No argument.
---

# /detect-environment

You are building the **operational dependency model** for the repo at the current
working directory: every piece of software the application needs to *execute*, as a
graph that `/create-core-sensors` turns into core sensors. You classify; the script
parses and validates.

## Prerequisites

- `.harness/stack-manifest.yaml` must exist (run `/detect-stack` first).

## Step 1 — gather raw facts (deterministic)

Run the parser and capture its output (it never interprets — just extracts):

```bash
<plugin-root>/scripts/harness-tools.sh detect-environment --mode facts --repo . > /tmp/raw-facts.yaml
```

`raw-facts.yaml` contains: `scripts` (package.json), `make_targets`, `procfile_entries`,
`compose_services` (image/ports/environment/depends_on/healthcheck), `compose_file`,
and `env_keys`. Read it.

## Step 2 — classify into the dependency model

Write `/tmp/environment-model.yaml` matching `schemas/environment-model.yaml`. The
**only** authored values are classifications and grounded pointers — never resolved
commands, env, or readiness (those live in the sensors generation emits).

- **`application`** — pick the script that *is* the app (`dev`, then `start`).
  `provided_by: {file: package.json, path: "scripts.<name>"}`. Its `depends_on` lists
  every backing dependency and setup node it needs before it can serve.
- **`dependencies.<name>`** — one per backing service in `compose_services`. Set
  `type` to `datastore` | `cache` | `broker`. `provided_by: {file: <compose_file>,
  path: "services.<name>"}`. Carry compose `depends_on` verbatim.
- **`setup[]`** — run-to-completion tasks. Map `db:migrate` → a `setup` node
  `depends_on` the datastore; map `db:seed*` → `setup` nodes with `optional: true`.
  `provided_by: {file: package.json, path: "scripts.<name>"}`.

Infer edges the infra files imply but don't state: the app and migrate both
`depends_on` the datastore; the app `depends_on` migrate.

**Grounding rule:** every `provided_by` must point at a `{file, path}` present in
`raw-facts.yaml` (a real `scripts.*` / `services.*` / Makefile target / Procfile
entry). The validator rejects anything else.

**No infra?** Emit just an `application` node (empty `dependencies`/`setup`). No run
script at all → there is no operational environment; tell the user and stop.

**DO NOT** put commands, env vars, or readiness in this file. Sensors own those.

## Step 3 — validate + persist

```bash
<plugin-root>/scripts/harness-tools.sh detect-environment --mode persist \
  --file /tmp/environment-model.yaml --facts /tmp/raw-facts.yaml --harness-dir .harness
```

- **Exit 0:** written to `.harness/environment-model.yaml`. Done.
- **Exit 2:** JSON `persisterror.Error` on stdout (schema, dangling edge, cycle, or an
  ungrounded `provided_by`). Fix `/tmp/environment-model.yaml` and re-run. **Stop after
  3 attempts** and report.
- **Exit 1:** script error on stderr — report to the user.

> **Plugin users:** `<plugin-root>` is two directories above this skill file.
