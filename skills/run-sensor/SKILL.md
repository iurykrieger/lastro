---
name: run-sensor
description: Run a kind:assertion harness sensor synchronously. Returns the streamed signals followed by the terminal AggregateSignal. Exit non-zero on verdict=fail.
---

# /run-sensor

Synchronously run an assertion sensor and emit its signals + terminal
`AggregateSignal` as JSONL on stdout.

## Usage

```
/run-sensor <sensor-id>
```

`<sensor-id>` is the `id` field of a sensor YAML under `.harness/sensors/`.

## Output

- **stdout** — JSONL stream. One JSON object per line. Streamed individual
  `Signal`s first, then one terminal `AggregateSignal` as the final line.
- **stderr** — empty on success; a `ScriptError` envelope on script-level
  failure (sensor id not found, kind mismatch, runtime error).

## Exit codes

| Code | Meaning |
|---|---|
| 0 | `AggregateSignal.verdict == pass` |
| 1 | `AggregateSignal.verdict == fail` |
| 2 | `AggregateSignal.verdict == inconclusive` |
| 3 | Script-level error (bad argv, missing sensor, wrong kind, etc.) |

## Constraints

- The sensor's `kind` MUST be `assertion`. If it is `observational`, exit
  code 3 with `{"code":"wrong-kind","hint":"use /start-sensor"}` on stderr.
- The script blocks until the sensor terminates. For long-running
  observational sensors, use `/start-sensor` instead.

## Implementation

Wraps `lifecycle.RunSensor` via `skillruntime.RunSensorWithServices`: any
shared observational core service in the sensor's dependency closure
(`depends_on` edges and composed `uses:` primitives, e.g. `run-dev`) is
started first — blocking on its `ready` observation — and torn down once
the run returns, the same lifecycle `/validate-use-case` provides. After
the call returns, replays the per-run `signals.jsonl` to stdout, then
emits the terminal aggregate.

## How to invoke

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh run-sensor <sensor-id>
```

## Examples

Pass:

```
$ /run-sensor 01HMG12RX9N6Z8WJ3D6PNHVQXC
{"schema_version":"1.0.0","sensor_id":"01HMG…","verdict":"pass",…}
{"type":"aggregate","sensor_id":"01HMG…","verdict":"pass",…}
$ echo $?
0
```

Fail (with heal hint):

```
$ /run-sensor 01HMG12RXATAFM4N0F0X5Y4SGE
{"schema_version":"1.0.0","verdict":"fail",…,"heal_hint":{"summary":"…"}}
{"type":"aggregate","verdict":"fail","heal_hint":{…}}
$ echo $?
1
```
