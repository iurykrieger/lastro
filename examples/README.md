# examples/

Synthetic subject repositories used by the harness framework's
integration tests (Track 1), plus the `validator` helper package
consumed by both test tracks.

## Layout

| Path | Purpose |
|---|---|
| `validator/` | Shared `ValidateAll` Go package. Drives the `/validate-use-case` skill against a target directory and aggregates verdicts into a `Report`. Used by both tracks. |
| `http-api-sample/` | Canonical passing `http-api` subject. 1 GET + 1 POST. Three use cases, six sensors. |
| `http-api-sample-broken/` | Sibling of the passing twin with a seeded bug (missing 400 validation branch). Ships `heal-fixture/editplan.json`. |
| `cli-sample/` | Minimal Cobra CLI subject. Proves archetype branching produces different downstream sensors. |
| `integration_test.go` + `heal_test.go` | **Track 1** — plan §11 criteria 1-7. Build tag `integration`. |
| `dogfood_test.go` | **Track 2** — framework validates itself against its own committed `.harness/` (at the repo root). Build tag `dogfood`. |

## Running the tests

```bash
# Track 1 — synthetic samples, plan §11 criteria, heal acceptance.
go test -tags=integration -v -timeout 5m ./examples/...

# Track 2 — framework dogfood gate.
go test -tags=dogfood -v -timeout 5m ./examples/...

# Untagged — validator unit tests with fakes (also runs in normal `go test ./...`).
go test ./examples/validator/...
```

`-short` skips both tracks.

## Reports

Each `ValidateAll` invocation writes a structured report to
`<target>/.harness/reports/<run-id>/report.json`. Both
`.harness/reports/` and `.harness/runtime/` directories are gitignored.

## Adding a new sample

1. Create `examples/<name>/` with its own `go.mod` (module
   `example.com/<name>`).
2. Author the sample source.
3. Hand-curate `.harness/{stack-manifest,fixtures,use-cases,sensors,policy}/`
   following the schemas under `schemas/examples/`. Add a
   `policy/global.yaml` declaring the obligatory angles for the
   sample's archetype.
4. Sensors must emit a Signal JSON line on stdout — the `cmd/sensor-check`
   helper in the existing samples is a good starting point.
5. Add the new sample's directory to `sampleDirs` in
   `examples/integration_test.go`.
