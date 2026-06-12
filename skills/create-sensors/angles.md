# Per-angle sensor approach

One section per validation angle. Before writing a sensor, read the section
for its angle: what the angle validates, the shape to declare
(`kind` / `nature` / `output_type`), what to compose or run, and how to grade
the outcome with `signal_matches`.

Two contracts apply across every angle:

- **Exit-status grading only covers binary outcomes.** A step that exits
  non-zero without emitting any signal is graded as a crash (`inconclusive`),
  not a failure. Any tool that reports *findings* through a non-zero exit
  (scanners, linters, validators, test runners, `grep`) must tolerate that
  exit (pipe into a parser, or `|| true`), end by echoing one normalized
  summary line, and be graded by `signal_matches`: one pass matcher on the
  report line with `expected: true` (no report ⇒ completeness fail, never a
  silent pass), plus fail/warn matchers with `heal_hint`s for findings.
- **Scope to the use case.** Wherever a tool accepts paths, point it at the
  use case's `source_refs` (written literally at generation time), not the
  whole repo — the verdict must speak about *this* use case.

Matcher precedence: every matcher that matches a line emits its own signal,
and the rollup takes the worst severity (`fail` > `warn` > `inconclusive` >
`pass`). A pass report matcher and a fail findings matcher may safely match
the same line — the fail wins.

## build

- **Validates:** the code paths the use case touches compile and package.
- **Shape:** `assertion` / `computational` / `single-shot`.
- **Command:** stack-native compiler — `go build ./...`, `tsc --noEmit`,
  `npm run build`.
- **Grading:** exit 0 with no signals is a pass. Add a fail matcher on the
  compiler's error lines (e.g. `error TS\d+`, `cannot find`, `undefined:`)
  with a `heal_hint`, so a broken build grades `fail` with the offending
  symbol as evidence instead of crashing to `inconclusive`.

## unit-test

- **Validates:** unit tests covering the use case exist and pass.
- **Shape:** `assertion` / `computational` / `single-shot`.
- **Command:** the project's test runner, scoped to the packages owning the
  use case's `source_refs` — `go test ./internal/orders/...`,
  `npx --yes jest src/handlers`, `pytest -q tests/orders`.
- **Grading:** runners exit non-zero on failing tests — add a fail matcher on
  the failure summary (`FAIL|\d+ (failed|failing)`) with a `heal_hint`, and
  an `expected: true` pass matcher on the completion summary (`^ok |passed`)
  so "no tests ran" is a completeness fail, not a silent pass. Treat a
  "no test files" report as `warn`: the use case is unprotected.

## code-structure

- **Validates:** the use case's implementation conforms to lint and
  architecture rules.
- **Shape:** `assertion` / `computational` / `single-shot`.
- **Command:** the stack's lint tool over the `source_refs` paths —
  `golangci-lint run <dirs>`, `npx --yes eslint <files>`.
- **Grading:** linters report findings via non-zero exit — apply the scanner
  contract: tolerate the exit, echo `lint errors=N warnings=M`, grade
  errors ≥ 1 as `fail`, warnings ≥ 1 as `warn`, with an `expected: true`
  matcher on the report line.

## e2e-test

- **Validates:** end-to-end behavior matches the use case's `when`/`then`
  against its real entry points.
- **Shape:** `assertion` / `computational` / `single-shot`. Compose the
  `e2e-test` core primitive: `{ uses: e2e-test, with: { method, path, ... } }`,
  binding the entry point literally and payloads via `${{ fixtures.<id> }}`.
  The primitive already `depends_on` the shared `run-dev` service.
- **Inputs:** the primitive's floor (`schemas/core-inputs/e2e-test.yaml`) is
  `base_url, method, path, query, headers, body, expect_status, timeout`. Bind
  `expect_status` to the variation's expectation — `"422"` for a failure use
  case — and `body` to a fixture path. The primitive grades the status itself
  (no `curl --fail`) and emits `status=` / `body=` lines.
- **Grading:** layer matchers over those normalized lines for the `then`
  clauses: an `expected: true` pass matcher on the expected status/body shape,
  fail matchers with `heal_hint`s for wrong status or missing fields. One
  sensor covers one use case; bind each variation to its own use case.
- **Auth-gated entry points:** when the asserted outcome sits behind an auth
  gate (a success 2xx, or a validation 4xx that only fires *after* auth), an
  unauthenticated probe can never reach it — it observes the rejection branch
  instead. Compose `provision-auth` as the step BEFORE the request and feed
  its credential into the request's headers:

  ```yaml
  steps:
    - id: auth
      uses: provision-auth
      with: { kind: session, persona: seeded-owner }   # or bearer | api-key | basic
    - id: request
      uses: e2e-test
      with:
        headers: "${{ steps.auth.outputs.header }}"
        # method/path/body/expect_status as usual
  ```

  Pick `kind` from the scheme the route accepts: `session` (cookie),
  `bearer` (JWT/opaque token), `api-key`, or `basic` — routes accepting
  several get one sensor per asserted scheme, not a merged one. If
  `.harness/sensors/core/provision-auth.yaml` does not exist,
  do NOT emit a sensor that asserts through the gate — it would fail (or
  coincidentally pass) for the wrong reason. Emit only the reachable
  variation (e.g. `failure-unauthenticated`) and report the others as
  blocked on auth provisioning. A failed provisioning recipe aborts the run
  and aggregates `inconclusive`, never a behavioral fail.
  Full example: `schemas/examples/sensor/uc-authenticated-consumer.yaml`.

## contracts

- **Validates:** the API/schema/SDK contract artifacts the use case's entry
  points belong to are well-formed and conformant.
- **Shape:** `assertion` / `computational` / `single-shot`.
- **Command:** the spec's validator — `npx --yes @redocly/cli lint <spec>`,
  `protoc --descriptor_set_out=/dev/null <proto>`, JSON Schema validation via
  a stack tool.
- **Grading:** validators exit non-zero on violations — scanner contract:
  tolerate the exit, echo `contracts errors=N`, fail matcher with a
  `heal_hint` naming the artifact and the first violation.

## logs

- **Validates:** log shape, redaction, and semantic correctness while the use
  case is exercised.
- **Shape:** `observational` / `computational` / `stream`, **attached to the
  shared service** (mechanics below). Only tail directly
  (`docker logs --follow`, `tail -f`) when no observational core service exists.
  A parameterized `logs` primitive (floor: `pattern, anti_pattern, within,
  service`) may also exist for one-shot grep-style assertions over the
  shared stream.
- **Grading:** `expected: true` pass matchers on the lines that must appear
  for this use case; fail matchers with `heal_hint`s on error lines
  (`5\d\d|unhandled|panic`) and on sensitive data surfacing in logs
  (credential/PII-shaped patterns derived from the use case's fixtures) —
  redaction failures are log defects.

### Attaching to the shared observational service

When `.harness/sensors/core/` contains a `kind: observational` +
`scope: core` service (e.g. `run-dev`), a `logs`-angle sensor (or any stream
consumer) **attaches** to it rather than spawning its own server or
`docker logs`. The runtime starts the service once, reference-counts it, and
feeds its live signal stream to every attaching sensor. Emit:

- `kind: observational`, `output_type: stream`.
- `depends_on: [<service-id>]` — this pulls the shared service into the use
  case's scheduled set; without it the service is never started.
- a step `{ id: watch, uses: <service-id> }` — the attach itself. No `run:`
  server boot, no second `docker logs`.
- `signal_matches:` applied to the service's emitted `matched_line`; mark
  must-appear lines `expected: true`.
- optional `observe_window: <duration>` (e.g. `45s`) bounding the watch;
  omit for the runtime default.

```yaml
id: s-checkout-logs
use_case_id: uc-checkout
angle: logs
kind: observational
nature: computational
output_type: stream
uses: []
depends_on: [run-dev]
observe_window: 45s
signal_matches:
  - { key: served, pattern: "GET /checkout .* 200", verdict: pass, expected: true }
  - { key: error,  pattern: "5\\d\\d|unhandled",     verdict: fail, heal_hint: { summary: "Server error during checkout", rationale: "A 5xx surfaced while exercising the use case." } }
steps:
  - id: watch
    uses: run-dev
```

## metrics

- **Validates:** the use case emits its telemetry with the right shape.
- **Shape:** `assertion` / `computational` / `single-shot` (scrape after
  exercising the use case); attach to the shared service instead when
  metrics flow through its log stream. Scraping a local endpoint needs
  `depends_on: [run-dev]`.
- **Command:** compose the `metrics` core primitive (floor: `metrics_url,
  name, labels, predicate, within`; it grades the predicate and emits the
  scraped value), or fall back to
  `curl -sS http://localhost:9090/metrics | grep <key>`.
- **Grading:** `grep` exits 1 when the metric is absent — tolerate it and
  grade with an `expected: true` pass matcher on the metric line; the
  completeness fail (with a `heal_hint` naming the missing key) is the
  "metric never emitted" verdict.

## database

- **Validates:** the writes/migrations promised by the use case's `then`
  actually happened.
- **Shape:** `assertion` / `computational` / `single-shot`. Compose the
  `database-query` core primitive (floor: `query, params, expect_rows,
  timeout`) with the query and `${{ fixtures.<id> }}`-bound identifiers —
  bind `expect_rows: "0"` to assert an ABSENT write for failure variations;
  fall back to a stack CLI probe (`psql -c`, `aws dynamodb get-item`).
- **Grading:** pass matcher (`expected: true`) on the expected row/state in
  the query output; fail matcher with a `heal_hint` describing the expected
  vs. observed state when it is missing or mismatched.

## performance

- **Validates:** latency, throughput, and resource ceilings for the use
  case's entry points.
- **Shape:** `assertion` / `computational` / `single-shot`. Compose the
  `performance` core primitive (floor: `base_url, method, path, headers,
  body, duration, rate, p95_budget_ms`; it emits `p95_ms=` and grades the
  budget itself), or fall back to a load tool from the stack
  (`hey -n 100 <url>`, `k6 run script.js`); needs `depends_on: [run-dev]`.
- **Grading:** normalize to one summary line (`perf p95_ms=N error_rate=P`),
  then threshold via matchers: `warn` approaching the ceiling, `fail` above
  it, each `heal_hint` stating the threshold and the measured value.

## security

A security sensor covers two facets — emit both as separate steps of one
sensor:

1. **dep-audit** — stack-native dependency audit (`npm audit --json`,
   `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`, `pip-audit`).
2. **code-scan** — scan the files in the use case's `source_refs` for
   hardcoded credentials and injection anti-patterns (request data
   concatenated or interpolated into queries/commands). Derive `grep -E`
   patterns from the stack's language and libraries; write the
   `source_refs` paths literally into the step at generation time.

- **Shape:** `assertion` / `computational` / `single-shot`.
- **Grading:** scanners report findings through a non-zero exit — that is a
  result, not a crash. Apply the scanner contract to every step: tolerate
  the exit, echo one normalized summary line (`dep-audit critical=2 high=5`,
  `code-scan secrets=0 injection=1`), one `expected: true` pass matcher per
  report line, fail matchers with `heal_hint`s for non-zero finding counts.

Full shape: `schemas/examples/sensor/assertion-security-single.yaml`.

## environment

Core sensors only (`run-dev`, `datastore`) — owned by `/create-core-sensors`.
Never emit a use-case sensor for this angle; use-case sensors *consume* the
environment via `depends_on` and attach steps.
