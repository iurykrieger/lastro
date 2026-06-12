# Design: Declarative env-var injection for sensor steps

- **Date:** 2026-06-12
- **Issue:** [#49 — Sensors should declare env-var injection via an `env:` step property](https://github.com/iurykrieger/lastro/issues/49)
- **Status:** Approved (brainstorming session)

## Problem

Sensor steps spawn as standalone child processes whose environment contains only
`HARNESS_*` vars plus whatever the harness itself was launched with. They do not
inherit the target project's `.env`, and there is no declarative surface to supply
environment variables. Any recipe needing a secret or connection string
(`provision-auth` session minting needs `NEXTAUTH_SECRET`, `database-query` needs
`DATABASE_URL`, …) is unrunnable without a manual `set -a; . ./.env` before invoking
the harness — invisible to the sensor YAML, non-reproducible, and unsatisfiable by
`/validate-use-case` and CI, which therefore aggregate `inconclusive` even when the
application code is correct.

This is the same class of gap as #42 (services) and #45 (auth): the harness can
author the work but cannot fully set up the runtime preconditions a step needs.

## Decisions made

| Decision | Choice |
|---|---|
| Scope | Full proposal: per-step `env:`, `${{ env.NAME }}` namespace, manifest `env_file:`, primitive-declared env requirements, redaction |
| Precedence | Host env > `env_file`; per-step `env:` entries always win for that step |
| Missing required var | Typed `missing_env` signal **pre-spawn** → sensor aggregates `inconclusive` |
| Redaction | Mask **all** injected values (GitHub-Actions-style), ≥4 chars, inline YAML literals exempt |
| Architecture | Go-side env resolver in the executor (spawn-time resolution), not template-compiler-only or a shim binary |

## Design

### 1. Schema surface

- **`schemas/sensor.yaml`** — `SensorStep` gains an optional `env:` property: a map
  of `NAME: string` where names match `^[A-Z][A-Z0-9_]*$`. Valid on both `run:` and
  `uses:` steps.
- **`schemas/stack-manifest.yaml`** — optional root-level `env_file:` string
  (project-root-relative path). `/detect-stack` records `.env` when found, preferring
  `.env.local` when both exist (Next.js convention).
- **Core primitive declarations (`schemas/core-inputs/*.yaml`)** — new optional
  `env:` block declaring the variables a primitive's recipes need:

  ```yaml
  env:
    NEXTAUTH_SECRET:
      description: "Secret the app signs/verifies NextAuth JWTs with"
      required: true   # default
  ```

  `provision-auth` and `database` are populated immediately.

### 2. Resolution model (env resolver)

A deterministic component in `internal/runtime/executor` (`envresolver.go`,
promoted to its own package only if it grows):

- **Once per sensor run:** load `env_file` via `github.com/joho/godotenv`. Build the
  *ambient view*: env_file values overridden by the host environment (host wins, so
  CI stays in control).
- **Once per step:** the child environment is composed of, in order:
  1. `os.Environ()` (status quo),
  2. ambient env_file vars not already present in the host env,
  3. existing fixture / `HARNESS_INPUT_*` / `HARNESS_STEPOUT_*` maps (status quo),
  4. resolved per-step `env:` entries — these win unconditionally.

  The merge happens at the existing construction point in
  `internal/runtime/executor/step.go:87-96`.
- Step `env:` values support the full `${{ ... }}` expression surface
  (`env.`, `inputs.`, `fixtures.`, `steps.`). They compile through the existing
  template pipeline, then the resolver expands the compiled shell-style references
  in Go (`os.Expand` against the child env map). Secret values exist only in the
  spawned process's environment block — never in script text, never on disk.

### 3. Template namespace

`env` joins `inputs` / `fixtures` / `steps` in the parser
(`internal/usecase/template/parser.go`). `${{ env.NAME }}` compiles to a
safely-quoted `"${NAME}"`, consistent with every other namespace; it resolves
naturally in `run:` scripts because the ambient view is in the child env. The
compiler records each `env.*` reference so the pre-spawn requirement check knows
what the step needs.

### 4. Fail-fast: `missing_env` → inconclusive

Before spawning a step, the resolver checks three requirement sources:

1. `${{ env.* }}` references collected at compile time,
2. step `env:` entries whose expressions reference an absent variable,
3. the composed primitive's declared `required: true` env.

Any unset-or-empty variable → the runtime emits a typed `missing_env` signal
pre-spawn (evidence: variable names and the sources searched — host env, env_file
path) with remediation text ("export NEXTAUTH_SECRET or add it to .env"), skips the
step, and the sensor's terminal AggregateSignal is `inconclusive`. The application
is not proven broken; the environment is incomplete. No silent empty string ever
reaches a child process.

### 5. Redaction

Every value the feature injects — all env_file-sourced values plus all resolved
per-step `env:` values — is registered in a redaction set. A redacting writer wraps:

- the `raw.log` sink (`internal/runtime/executor/rawlog.go`),
- `signals.jsonl` serialization,
- aggregate evidence / `heal_hint` text.

Exact occurrences are replaced with `***`. Guards:

- values shorter than 4 characters are skipped (masking `"1"` corrupts output);
- values written as inline literals in sensor YAML (e.g. `NODE_ENV: test`) are
  exempt — they are already visible in the repo;
- host-env vars the harness never touched are out of scope (status quo).

### 6. Edge cases

| Case | Behavior |
|---|---|
| `env_file:` declared, file absent | Warning signal; continue with host-only view (requirement check catches real gaps) |
| Unparseable `.env` | Typed error signal; sensor `inconclusive` |
| Inline literal step `env:` value | Allowed; exempt from redaction |
| Same var in host and env_file with different values | Host wins; no warning (standard dotenv semantics) |

### 7. Testing & dogfooding

- **Unit (every touched package):** precedence matrix; dotenv loading; missing-var
  detection per requirement source; redaction writer (boundary lengths,
  multi-occurrence, literal exemption); `env` namespace parse/compile.
- **Integration:** an `examples/` sensor composing `provision-auth` with `env:`
  proves the child sees the variable, the log is masked, and removing the variable
  flips the run to `inconclusive` with a `missing_env` signal.
- **Acceptance (from the issue):** the NextAuth scenario
  (`s-create-project-success-e2e-test`) runs green via `/run-sensor` and
  `/validate-use-case` with no manual `export` / `set -a; . ./.env`.
- **Skills:** `/detect-stack` records `env_file`; `/create-sensors` emits per-step
  `env:` only for overrides/renames, since the ambient view covers the common case.

## Out of scope

- Encrypted secret stores / vault integration (env_file + host env only).
- Redacting encodings of secrets (base64/URL-encoded variants) — exact-match only
  for now; revisit if leaks are observed.
- Per-service env injection for shared services (`run-dev` already reads `.env`
  itself via the app's own tooling).
