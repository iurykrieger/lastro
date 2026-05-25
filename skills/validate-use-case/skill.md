---
name: validate-use-case
description: Run every sensor for one use case in DAG order, aggregate the verdicts, emit a UseCaseVerdict. Exit non-zero on fail/inconclusive.
---

# /validate-use-case

Run every sensor that validates the given use case (parallel within DAG
levels), aggregate their `AggregateSignal`s into a `UseCaseVerdict`, and
emit it on stdout. Writes `verdict.json` under `.harness/runtime/`.

## Usage

```
/validate-use-case <usecase-id>
```

`<usecase-id>` is the `id` field of a use-case YAML under
`.harness/use-cases/`.

## Output

- **stdout** — JSONL stream:
  - One line per sensor's terminal `AggregateSignal` (sensor-id order)
  - Final line: persisted verdict envelope (also written to `verdict.json`)
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

The exit code is the worst of (a) the `UseCaseVerdict.verdict` and (b)
the worst individual `AggregateSignal.verdict` across all sensors.
Promotion to the worst sensor verdict prevents an empty/vacuous policy
(no obligatory angles) from silently hiding sensor-level failures.

| Code | Meaning |
|---|---|
| 0 | All sensors passed AND `UseCaseVerdict.verdict == pass` |
| 1 | `UseCaseVerdict.verdict == fail` OR any sensor returned `verdict == fail` |
| 2 | `UseCaseVerdict.verdict == inconclusive` OR any sensor was inconclusive (and none failed) |
| 3 | Script-level error (use case not found, cycle, runner error) |

## Outputs

- `.harness/runtime/use-cases/<usecase-id>/<run-id>/verdict.json` — the
  full `UseCaseVerdict` plus `sensor_runs` traceability listing every
  sensor's id and verdict.

## Constraints

- The use case must have at least one entry in `archetype_scope`; the
  first entry is used to resolve the validation policy.
- Only sensors whose `use_case_id` matches participate. Cross-use-case
  `depends_on` is ignored.
- Policies are loaded best-effort from
  `.harness/policy/global.yaml` and
  `.harness/policy/local/<usecase-id>.yaml`. Missing or malformed files
  yield empty policies (no obligatory angles).
- Dependency-failed sensors get a synthetic `inconclusive`/`stopped`
  aggregate with `heal_hint.summary` = `"skipped: depends_on <id> failed"`.
- Cycles in `depends_on` exit 3 with `{"code":"scheduler-failed"}`.

## Examples

Pass:

```
$ /validate-use-case order-create
{"sensor_id":"order-create-build",…,"verdict":"pass"}
{"sensor_id":"order-create-e2e",…,"verdict":"pass"}
{"use_case_verdict":{"verdict":"pass","confidence":1.0,…},"use_case_run_id":"01HMG…","sensor_runs":[…]}
$ echo $?
0
```

Fail with dependency skipping:

```
$ /validate-use-case order-create
{"sensor_id":"order-create-build","verdict":"fail","heal_hint":{…}}
{"sensor_id":"order-create-e2e","verdict":"inconclusive","heal_hint":{"summary":"skipped: depends_on order-create-build failed"}}
{"use_case_verdict":{"verdict":"fail",…},"use_case_run_id":"…","sensor_runs":[…]}
$ echo $?
1
```
