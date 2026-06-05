---
name: start-sensor
description: Spawn a kind:observational harness sensor as a long-running watcher and return its handle. Pair with /stop-sensor.
---

# /start-sensor

Spawn an observational sensor's watcher process and return immediately
with the handle the caller passes to `/stop-sensor` later.

## Usage

```
/start-sensor <sensor-id>
```

`<sensor-id>` is the `id` field of a sensor YAML under `.harness/sensors/`.
The sensor's `kind` MUST be `observational`.

## Output

- **stdout** — single-line JSON object:
  `{"handle":"<sensor-id>:<run-id>","run_dir":"<path>","pid":<int>}`
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Watcher spawned; handle written to stdout |
| 3 | Script-level error (sensor not found, wrong kind, spawn failed) |

The watcher process is detached from this skill's process — exiting the
skill does not kill the watcher. Use `/stop-sensor <handle>` to terminate.

## Constraints

- The sensor's `kind` MUST be `observational`. For `assertion` sensors,
  use `/run-sensor`.
- Expected observations are read from the sensor's YAML
  (`expected_observations` field; see B4). If absent, the script passes
  `nil` and the watcher reports `completeness` as best-effort.

## How to invoke

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh start-sensor <sensor-id>
```

## Examples

```
$ /start-sensor 01HMG12RX9N6Z8WJ3D6PNHVQXC
{"handle":"01HMG12RX9N6Z8WJ3D6PNHVQXC:01HMG12S4ABCDEFGH...","run_dir":"…","pid":12345}
$ echo $?
0
```
