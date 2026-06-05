---
name: stop-sensor
description: Terminate a running observational sensor identified by its handle. Emits the terminal AggregateSignal.
---

# /stop-sensor

Stop a previously-started observational sensor and emit its terminal
`AggregateSignal`.

## Usage

```
/stop-sensor <sensor-id>:<run-id>
```

The handle is the value `/start-sensor` printed under `"handle"`. The
sensor-id half is the sensor's slug id (kebab-case); the run-id half is
a 26-char Crockford base32 ULID.

## Output

- **stdout** — single-line JSON: the terminal `AggregateSignal`.
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Sensor terminated cleanly with `verdict == pass` |
| 1 | `verdict == fail` (e.g., expected observations missing) |
| 2 | `verdict == inconclusive` (timeout, partial completion) |
| 3 | Script-level error (bad handle, sensor not found) |

## Constraints

- Works whether the watcher is still running (sends SIGTERM, waits for
  aggregate.json) or already terminated (reads aggregate.json from disk).
- The handle must be well-formed: kebab-case sensor-id, then `:`, then
  26-char ULID run-id. Malformed handles exit 3 with `{"code":"bad-handle"}`.

## How to invoke

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh stop-sensor <sensor-id>:<run-id>
```

## Examples

```
$ /stop-sensor order-flow-watcher:01HMG12S4ABCDEFGHJKMNPQRSTV
{"type":"aggregate","verdict":"pass",…}
$ echo $?
0
```
