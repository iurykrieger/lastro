# Design: Operational environment dependency model for sensor generation

- **Date:** 2026-06-14
- **Issue:** [#52 — Sensor generation is blind to the project's runtime environment & dependency graph](https://github.com/iurykrieger/lastro/issues/52)
- **Status:** Approved (brainstorming session)

## Problem

Sensor generation (`/create-core-sensors`, `/create-sensors`) authors commands from
the **static stack manifest** — a list of detected components and versions — but
never models the project's **operational environment**: how the application is
actually started, which backing services it depends on, in what order they must come
up, what readiness each tier requires, and what setup (migrations, seeds) must run
first.

As a result, generated sensors encode commands that presuppose a runtime nobody
provisions. The commands are individually valid and grounded in real tools, but the
environment they assume is never established, so they fail for reasons unrelated to
the application code under test.

This is the root cause behind a string of point-fixes, each patched in isolation:

- **#42** — `depends_on` services weren't started for single-sensor runs
- **#45** — auth wasn't provisioned, so session-gated flows were unsatisfiable
- **#49 / #50** — `.env` wasn't injected, so secret-reading recipes couldn't run
- **#51** *(still open)* — `run-dev` boots the app but not its datastore, so
  DB-backed flows 500

Verified against the code: the manifest's only top-level keys are `schema_version`,
`applicable_angles`, `env_file`, `archetype`, `components`. It records *what the app
is*, never *how it boots*. `depends_on` is hand-authored by the LLM during
generation; the resolver only topo-sorts edges it is given, never discovers them. The
convergent runtime infrastructure (`servicemgr`, `envview`/`redact`, baseline floors)
all landed *below* the generation layer, so each new environmental concern requires
its own patch. Five fixes in, the failure class is still live — an architectural gap,
not another bug.

## Decisions made

| Decision | Choice |
|---|---|
| Scope | Full operational model now (not a vertical slice) |
| Detection mechanism | Deterministic Go parser + LLM classifier + Go validator |
| Storage | Sibling file `.harness/environment-model.yaml` via new `/detect-environment` skill |
| Model shape | A **dependency model** of software the app needs to execute; references sensors, never duplicates commands or env |
| Command linkage | A node carries a grounded `provided_by` pointer (file+path), not a resolved command string |
| Env vars | Reuse the existing sensor `env:` EnvSpec (#49); the model declares no env |
| Readiness | Not in the model; generation derives it from node `type` (or a compose `healthcheck`) |
| Honest verdicts | Typed precondition-signal family generalizing `missing_env`; all → `inconclusive`, never `fail` |
| `#22` (heal on setup failure) | Out of scope; enabled-but-separate |

## Design

### 1. The dependency model (`.harness/environment-model.yaml`)

Every piece of software the application depends on to execute is a node. The file is
a *cause*, not an index: `/create-core-sensors` reads the graph and emits one core
sensor per node. It references sensors and never restates what a sensor already owns
(command, env, readiness, edges).

```yaml
schema_version: 1.0.0

application:
  provided_by: { file: package.json, path: "scripts.dev" }     # resolves to `next dev`
  depends_on: [postgres, migrate]

dependencies:
  postgres:
    type: datastore                                            # → datastore-style readiness (pg_isready)
    provided_by: { file: docker-compose.yml, path: services.postgres }
    depends_on: []

setup:
  - id: migrate
    type: setup                                                # → single-shot, success = exit 0
    provided_by: { file: package.json, path: "scripts.db:migrate" }
    depends_on: [postgres]
```

Key properties:

- **`provided_by` is a grounded pointer, not a command.** It names a `{file, path}`
  the deterministic parser verified exists. Generation resolves it to the sensor's
  `run` step. The command string lives in exactly one authored place
  (`package.json` / `docker-compose.yml`); the model references it, the sensor
  materializes it.
- **`type` drives sensor *shape*.** `datastore`/`cache`/`broker` → observational
  service with a type-appropriate readiness probe; `setup` → single-shot assertion;
  the `application` → the `run-dev` observational service with log-pattern readiness.
- **`depends_on` is the discovered graph.** Edges reference node names (the same
  names compose uses), parsed in verbatim where compose declares them and inferred by
  the classifier where it does not.
- **No env block, no readiness, no command duplication.** Env lives in the generated
  sensors' `env:` EnvSpec (#49); readiness is derived at generation; commands are
  resolved from pointers.
- **Compose facts (image/ports) are not re-encoded** — they live in
  `docker-compose.yml`, which the datastore sensor's `run` invokes.

The model preserves rather than reconciles conflicting facts. (In the reference repo,
`docker-compose.yml` provisions `reliable/reliable` while `.env`'s `DATABASE_URL`
points at `hooky/hooky`; a real mismatch surfaces as an honest `inconclusive` at run
time, not a silent wrong assumption.)

#### Schema validation rules

- Every `provided_by` `{file, path}` resolves to a fact the parser extracted (no
  hallucinated scripts/services).
- Every `depends_on` names a node declared in the file; the graph is acyclic.
- Node ids/names are unique; `type` is a known enum value.
- A repo with no infra files yields a valid model with only an `application` node (or
  an empty model when there are no run scripts at all).

### 2. The `/detect-environment` pipeline

Follows the `detect-stack` shape (LLM authors, Go validates/persists) with a
deterministic parser front-end so the classifier reasons over facts, not file text.

**Stage 1 — deterministic parser (`internal/environment/`, Go).** Extracts raw facts
verbatim:
- `package.json` → `scripts{}`; `Makefile` → targets; `Procfile` → process entries
- `docker-compose.yml` / `compose.yaml` → services with `image`, `ports`,
  `environment`, `depends_on`, and `healthcheck` if declared
- `.env` / `.env.example` → key names (values ignored)
- known config throws (drizzle's `if (!process.env.DATABASE_URL) throw`) →
  required-key hints

Output: a `RawFacts` struct serialized to a temp `raw-facts.yaml`.

**Stage 2 — classifier (the `/detect-environment` skill, LLM).** Reads
`raw-facts.yaml` + `stack-manifest.yaml` and produces the dependency model:
- pick the **application** node (which script is the app: `dev`/`start`)
- classify each compose service's **`type`** (`datastore`/`cache`/`broker`)
- map setup scripts (`db:migrate`, `db:seed-*`) to **`setup`** nodes, marking seeds
  `optional: true`
- **infer edges** compose did not state (app → datastore + migrate; migrate →
  datastore); use compose `depends_on` verbatim when present

**Stage 3 — Go validator + persist (`internal/environment/load.go`, mirrors
`stack.Persist`).** Enforces the §1 validation rules against
`schemas/environment-model.yaml`, then writes `.harness/environment-model.yaml`.

**Graceful degradation.** No compose, bare `next dev` → an `application` node with
empty `dependencies`/`setup`. Zero run scripts → empty model and a "no operational
environment detected" report. Nothing regresses relative to today.

New artifacts:
- `internal/environment/` — parser (compose/scripts/dotenv), `facts.go`, `model.go`,
  `load.go`, `persist.go` (+ `_test.go` siblings)
- `schemas/environment-model.yaml` + `schemas/examples/environment-model/*.yaml`
- `skills/detect-environment/SKILL.md` + `scripts/main.go` (parser-facts and
  validate-persist subcommands)

### 3. `/create-core-sensors` consuming the model

One node → one core sensor. The command is resolved from `provided_by`; the edges are
carried onto the sensor's own `depends_on`. The LLM stops inferring "what command
boots this stack."

| Model node | Core sensor | Shape | `run` from | Readiness |
|---|---|---|---|---|
| `application` | `core-run-dev` | observational / stream, `scope: core` | `provided_by` → `next dev` | `key: ready` log matcher (`"Ready in\|compiled"`) |
| `dependencies.<svc>` | `core-datastore-<svc>` | observational / stream, `scope: core` | `docker compose up -d <svc>` | compose `healthcheck` if declared, else `type`-derived probe (`pg_isready`) as `key: ready` |
| `setup[]` | `core-<id>` | assertion / single-shot | `provided_by` → `npm run db:migrate` | `exit_zero` |

Discovered (no longer hand-authored):
1. **Commands** — resolved from `provided_by` pointers.
2. **`depends_on` edges** — model node-name graph translated node→sensor-id onto each
   sensor's `depends_on`. The existing resolver topo-sorts `postgres → migrate → app`
   — #42's closure machinery, fed discovered edges.
3. **Env needs** — generation discovers each node's required keys and declares them in
   the sensor's `env:` EnvSpec (#49), so the pre-spawn `missing_env` check covers them.

**Use-case sensors get the closure for free.** A DB-backed use-case sensor declares
only `depends_on: [core-run-dev]` (or composes the `e2e-test` primitive, which already
does). The resolver walks the model-derived edges and pulls in `core-migrate` and
`core-datastore-postgres` transitively — in order, readiness-gated, with symmetric
teardown. This is the #52 acceptance criterion.

**Stable ids** derive deterministically from node identity
(`core-datastore-postgres`, `core-migrate`), so re-detection + regeneration rewrites
the same sensors rather than duplicating them.

**Integration dependency:** the datastore sensor's top-level `uses:` must ground in a
`StackComponent`. `/detect-stack` must record the container tooling (`docker` /
`compose`) as a component — it already sees `docker-compose.yml` as detection
evidence, so this is a small in-scope extension.

### 4. Honest `inconclusive` verdicts

Generalize `missing_env` (#49) into a typed precondition-signal family. All aggregate
to **`inconclusive`, never `fail`**: if the harness could not establish the world, it
cannot claim the application is broken.

| Signal key | Fires when | Names | Verdict |
|---|---|---|---|
| `missing_env` *(exists)* | a required `env:` key is absent/empty pre-spawn | the key | inconclusive |
| `missing_service` | a dependency node's `bring_up` fails (no Docker daemon, image pull fails, port taken) | the service + node | inconclusive |
| `unready_service` | bring-up ran but readiness never passed within `timeout` (no `key: ready`) | the service + probe | inconclusive |
| `setup_unavailable` | a `setup` node's command is absent/unresolvable | the setup step | inconclusive |

Each carries a structured payload — `precondition` (category), `node`, and a
`remediation` line aimed at the **human** ("start the Docker daemon", "set
`DATABASE_URL` in `.env`"). It is a remediation hint, not a `heal_hint`: these are not
code defects, so they do not enter the heal loop.

**Aggregation.** The rollup already orders error-termination before completeness (the
#45/#49 precedent). We extend that classification: a core sensor terminating with a
precondition signal marks its use case `inconclusive` and propagates the typed signal
upward so the verdict names the unmet piece. A use-case sensor whose `depends_on`
closure hit a precondition is skipped and inherits `inconclusive` — never a behavioral
`fail`.

**Boundary held explicitly.** A precondition that *can't be established* (daemon down,
absent service, missing secret) → `inconclusive`, no heal. A setup command that *runs
and exits non-zero* (e.g. a migration failing on a genuine schema bug) is a code
problem, and routing it to heal is the open **#22**. The typed signals make this
distinction representable, but wiring heal-on-setup-failure is #22's work, not this
spec's.

Implementation surface (all extends existing code):
- typed keys in `schemas/enums/precondition-signals.yaml`, reusing the `missing_env`
  signal shape
- `servicemgr` / `executor`: emit `missing_service` on bring-up failure,
  `unready_service` on readiness timeout
- `internal/aggregate/rollup.go`: classify precondition signals → `inconclusive`,
  propagate, ordered before fail/completeness

### 5. Testing & dogfooding

**Deterministic unit tests** (table-driven `_test.go` per `internal/environment/`
package):
- **parser** — compose services, `package.json` scripts, `.env` keys, Procfile/
  Makefile, drizzle `DATABASE_URL`-throw, and the no-infra → empty facts path. Golden
  `raw-facts` fixtures.
- **validator/persist** — rejects dangling `provided_by` (grounding), dangling
  `depends_on` (edges), cycles, schema-invalid models; accepts the reference model and
  the no-op model.
- **schema goldens** — `schemas/examples/environment-model/{nextjs-drizzle,no-op}.yaml`,
  each asserted valid.
- **rollup** — `missing_service` / `unready_service` → `inconclusive`, propagate to
  dependents, ordered before `fail`.

**Integration (offline, deterministic).** Capture the reference repo's infra files
(`package.json`, `docker-compose.yml`, `.env.example`, `drizzle.config.ts`) into
`examples/nextjs-drizzle-sample/`. The integration test runs the full chain and
asserts:
- `/detect-environment` produces the expected model (`postgres` → `migrate` →
  `application`, edges and pointers intact);
- `/create-core-sensors` emits `core-datastore-postgres`, `core-migrate`,
  `core-run-dev` with commands resolved from pointers and the discovered closure;
- a use-case sensor declaring only `depends_on: [core-run-dev]` resolves to the full
  ordered tier.

**Live acceptance (manual, against `~/Workspace/reliable-events/dashboard`):**
- Docker daemon stopped → `missing_service`, verdict `inconclusive`;
- `DATABASE_URL` unset → `missing_env`, verdict `inconclusive`.

**Self-dogfood:** `/detect-environment` on the lastro repo itself (CLI archetype, no
compose) → a valid no-op model.

## Out of scope

- **#22** — triggering `/heal` on setup-command failures. The typed signals make the
  inconclusive-vs-code-defect distinction representable; wiring heal is separate.
- Reconciling conflicting infra facts (compose vs `.env`). The model records facts
  faithfully; mismatches surface as runtime `inconclusive`.
- Non-`type`-derivable readiness beyond compose `healthcheck` + framework log
  patterns (e.g. a custom `/health` route classifier) — future extension.

## Acceptance criteria (from #52)

- Generation reads run scripts, compose topology, and setup steps and records them as
  a structured dependency model. ✔ §1–2
- A generated DB-backed sensor brings up its full dependency tier (datastore +
  migrations + app), in order, gated on readiness, with symmetric teardown, no manual
  setup. ✔ §3
- An unmeetable precondition yields `inconclusive` with a typed signal naming the
  missing piece, not a misleading `fail`. ✔ §4
