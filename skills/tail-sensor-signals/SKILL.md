---
name: tail-sensor-signals
description: Stream signals from a running or completed observational sensor. Supports --follow and --since=<n> for resumption.
---

# /tail-sensor-signals

Read the per-run `signals.jsonl` for a sensor and emit it to stdout.
Pure file reader — no runtime API call. Use this between `/start-sensor`
and `/stop-sensor` to watch what an observational sensor is observing.

## Usage

```
/tail-sensor-signals <sensor-id>:<run-id> [--follow] [--since=<n>]
```

The sensor-id half is a kebab-case slug; the run-id half is a 26-char
Crockford base32 ULID.

- `--follow`: block and stream new lines as the watcher emits them. Exits
  when the sensor leaves `.harness/runtime/running_sensors.json` AND no
  new bytes arrive for 1 second.
- `--since=<n>`: start at signal number `n` (1-indexed). Lets the LLM
  resume after a previous tail without re-reading.

## Output

- **stdout** — each `signals.jsonl` line, one per line.
- **stderr** — empty on success; `ScriptError` envelope on failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Graceful EOF (snapshot done, or follow exited cleanly) |
| 3 | Bad handle, unreadable file, or other script error |

This skill does not opine on the sensor's verdict — that's `/stop-sensor`'s job.

## How to invoke

> **Plugin users:** `<plugin-root>` is the directory two levels above this skill file.
> Typical path after marketplace install: `~/.claude/plugins/lastro-harness/`.

```bash
<plugin-root>/scripts/harness-tools.sh tail-sensor-signals <sensor-id>:<run-id> [--follow] [--since N]
```

## Examples

Snapshot:

```
$ /tail-sensor-signals order-flow-watcher:01HMG12S4ABCDEFGHJKMNPQRSTV
{"verdict":"pass",…}
{"verdict":"pass",…}
$ echo $?
0
```

Follow:

```
$ /tail-sensor-signals order-flow-watcher:01HMG12S4ABCDEFGHJKMNPQRSTV --follow
{"verdict":"pass",…}      ← existing line
{"verdict":"pass",…}      ← new line as watcher emits it
^C    ← or wait for /stop-sensor from a sibling process
$ echo $?
0
```

Resume:

```
$ /tail-sensor-signals order-flow-watcher:01HMG12S4ABCDEFGHJKMNPQRSTV --since=3
{"verdict":"pass",…}      ← 3rd line onward
```
