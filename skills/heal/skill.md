---
name: heal
description: Apply a Claude-supplied EditPlan, re-validate the use case, and either keep the edit (verdict=pass) or revert it (verdict=fail). Single-shot.
---

# /heal

Apply one edit-and-revalidate cycle. The loop iteration itself is driven
by a Claude Code `PostToolUse` hook on `/validate-use-case` and
`/run-sensor` — this script is single-shot.

## Usage

```
echo '<edit-plan-json>' | /heal <usecase-id>
```

`<usecase-id>` is the use case to re-validate after applying the edit.
The skill reads the `EditPlan` JSON from stdin.

### EditPlan JSON shape

```json
{
  "files": [
    {"path": "src/handlers/orders.ts", "op": "write", "content": "<full file contents>"},
    {"path": "src/handlers/old.ts",     "op": "delete"}
  ],
  "rationale": "<one-paragraph explanation of why this fixes the failing signal>"
}
```

`op` is `write` (replace/create) or `delete`. `path` is repo-root-relative
and must not contain `..` segments.

## Output

- **stdout** — JSONL stream:
  - One line per re-validated sensor's `AggregateSignal`
  - Final line: heal envelope:
    `{"status":"healed|reverted","iteration":N,"max_iterations":M,"verdict":{…},"applied_files":[…]}`
- **stderr** — empty on success; `ScriptError` envelope on script-level failure.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | `verdict == pass` after applying the edit (edit kept) |
| 1 | `verdict == fail` (edit reverted; heal-state incremented) |
| 2 | `verdict == inconclusive` (edit reverted) |
| 3 | Script-level error (bad argv, bad stdin, heal-exhausted, etc.) |

## Persistent state

`.harness/runtime/heal-state.json` tracks how many heal attempts the current
cycle has spent. Counter resets to 0 on `verdict=pass`. When the counter
reaches `max_iterations` (default 3), the script refuses to apply further
edits and exits 3 with `{"code":"heal-exhausted"}`.

## Constraints

- Edits must be repo-root-relative; absolute paths or `..` segments are
  rejected with `{"code":"bad-edit-plan"}`.
- Snapshot/restore is in-memory file backup (no `git stash` yet). When
  B3's `DefaultTransactor` lands, this skill will switch to it.
- Re-validation reuses the `/validate-use-case` DAG scheduler.

## Examples

Heal succeeds:

```
$ echo '{"files":[{"path":"src/order.ts","op":"write","content":"…fixed…"}],"rationale":"…"}' | /heal order-create
{"sensor_id":"order-create-build","verdict":"pass",…}
{"status":"healed","iteration":1,"max_iterations":3,"verdict":{…},"applied_files":["src/order.ts"]}
$ echo $?
0
```

Heal fails → edit reverted, iteration incremented:

```
$ echo '{"files":[…],"rationale":"…"}' | /heal order-create
{"sensor_id":"order-create-build","verdict":"fail","heal_hint":{…}}
{"status":"reverted","iteration":2,"max_iterations":3,"verdict":{…},"applied_files":[]}
$ echo $?
1
```

Heal exhausted:

```
$ echo '…' | /heal order-create
{"code":"heal-exhausted","details":{"iteration":3,"max_iterations":3}}
$ echo $?
3
```
