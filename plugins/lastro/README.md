# Lastro plugin for Claude Code and Codex

One plugin, two harnesses. It wraps the `lastro` CLI so that every implementation an agent proposes goes through
Lastro before any code is written:

1. **Before the first source edit, in any mode** (plan mode, auto mode, an informal plan or none), the agent writes
   the change as prose in `.lastro/proposals/<slug>.md` and runs `lastro propose`: a new proposal whose intents add,
   change or remove the application's use cases (`.lastro/usecases/`). The gate enforces it: the first edit is denied
   with these steps. Leaving Claude Code's plan mode saves the plan as the prose of the next proposal.
2. **The user reviews the intents like a pull request**: answers every decision, then approves, requests changes
   (amend) or rejects each intent, seeing the existing proofs each one replaces on purpose. The agent is the author
   and never approves; the policy (`review` in `.lastro/config.json`) sets how many reviewers each intent needs and
   whether the submitter counts.
3. Once the proposal is approved, `lastro derive` writes its pending proofs into the proposal, in the repository
   being changed.
4. The agent implements from the approved intents. When it stops, the plugin builds and runs `lastro prove` on the
   proposal and every use case's proofs; if a proof fails (or a use case regresses), the failures go back to the
   agent and it keeps going (up to `maxIterations`).
5. A progress board shows where the change stands: review, approvals, proofs `✓` passed, `✗` failed, `○` pending,
   and regressions. In Claude Code, the [lastro-board](../lastro-board) mod shows it in a pane beside the chat, with
   each proof updating live as `lastro prove --stream` reports it.
6. Once every proof passes, the user runs `lastro apply`: the proofs join their use cases. The next source edit is
   a new change and starts again at step 1.

```
Lastro · shipping_fee · proofs FAILED

  ✓ Intents      4 extracted
  ✓ Decisions    1/1 answered
  ✓ Approval     4/4 intents approved
  ✓ Proofs       3 derived
  ▸ Verification 2/3 passed, 1 failed

  Proofs
    ✗ C1 violated  A subtotal of 100 or more ships free. [I1]
        counterexample {"member":false,"subtotal":100} → {"fee":15}
    ✓ C2 proved    Members always ship free. [I2]
    ✓ C3 proved    A non-member under 100 pays exactly 15. [I3]
    – I4 beyond proof (termination-timing): needs a human sign-off
```

## Install

Claude Code:

```
/plugin marketplace add iurykrieger/lastro
/plugin install lastro@lastro
```

Codex (hooks need `[features] codex_hooks = true` in `~/.codex/config.toml` on versions where they are still
behind the flag):

```
codex plugin marketplace add iurykrieger/lastro
```

then enable `lastro` from `/plugins`. The plugin needs Node 20+ and the Lastro CLI (`npm i -D lastro`, or
`lastro-go`).

To enable it for everyone in a repository, commit a `.claude/settings.json` that declares the marketplace
(`"extraKnownMarketplaces": { "lastro": { "source": { "source": "github", "repo": "iurykrieger/lastro" } } }`) and
`"enabledPlugins": { "lastro@lastro": true }`. This repository does that for itself.

Then ask the agent to "set up Lastro" (the `setup` skill), or write `.lastro/plugin.json` yourself.

## What is in it

| Path | What it does |
|---|---|
| `.claude-plugin/plugin.json`, `.codex-plugin/plugin.json` | The manifests. Both harnesses load the same skills and hooks. |
| `skills/setup` | Draft the model with the user, `lastro init`, write `.lastro/plugin.json`. |
| `skills/bootstrap` | For a repository that already has code: `lastro scan` sizes it, `lastro bootstrap` reads its code, tests and commits into a bootstrap proposal of the use cases it implements today, the user reviews it, its proofs are checked against the code as it is, and `lastro apply` records them as the baseline. |
| `skills/propose` | Prose → proposal and its intents → the user's review (decisions, approve / request changes / reject, threads, the proofs each intent replaces) → `lastro derive`. |
| `skills/verify` | Prove on demand, read counterexamples and regressions, fix the code, never touch proposals, use cases or proofs. |
| `hooks/hooks.json` | `SessionStart` adds the board and the rules; `UserPromptSubmit` adds one short line, only when the board changed; `PreToolUse` gates code edits and review commands; `PostToolUse` shows the board after each `lastro` command and marks proofs stale after an edit; `Stop` proves and sends failures back. |
| `scripts/lastro-plugin.mjs` | The hook entry point, plus `board [--line]`, `review` and `prove` for people and status lines. |

## The hooks

| Event | What happens |
|---|---|
| `PreToolUse` on edits | The first source edit of a change, in any mode, is denied with the steps to write a proposal and start the review. Edits go through only while the current proposal is approved, its proofs are derived and its last run has not passed yet; after it passes, the next edit is a new change. Proposal prose (`.lastro/proposals/*.md`), handoff answers and the `allow` globs are always allowed. `.lastro/usecases/`, the proposal files (`.json`, `.receipt.md`) and `config.json` are written only by the CLI. |
| `PreToolUse` on `lastro review` | Claude Code shows a permission prompt: that click is the user's review. `--as claude` (or any agent name) is refused in both harnesses. |
| `PostToolUse` | After a `lastro` command, the board is shown to the user. After a code edit, the last proof is marked stale. After `ExitPlanMode` (Claude Code), the plan is saved as the next proposal's prose, `.lastro/proposals/<id>-<slug>.md`; after `update_plan` (Codex), a reminder. |
| `Stop` | With derived proofs: build, then `lastro prove <proposal> --stream` (the proposal's proofs and every use case's) into `.lastro/plugin/progress.jsonl` (what the mod draws). Failing proofs block the stop and the feedback goes back to the agent, up to `maxIterations`; then the user takes over. The board is shown either way. |

Both harnesses send the same hook input and read the same output (`permissionDecision`, `additionalContext`,
`decision: "block"`, `systemMessage`), and Codex sets `CLAUDE_PLUGIN_ROOT` for plugin hooks, so one script serves
both. Codex has no per-command permission prompt from hooks, so there the review commands follow your Codex
approval policy.

## `.lastro/plugin.json`

```json
{
  "cli": "npx lastro",
  "build": "npm run build",
  "prove": "dist/shipping.js --export shippingFee",
  "enabled": true,
  "allow": ["docs/**", "**/*.md"],
  "maxIterations": 5
}
```

| Key | Default | Meaning |
|---|---|---|
| `cli` | `lastro` | How to run the CLI (`npx lastro`, `lastro-go`, `node path/to/cli.js`). `LASTRO_CLI` overrides it. |
| `dir` | `.lastro` | The workspace. |
| `build` | none | Runs before each proof. |
| `prove` | none | The arguments after `lastro prove`. |
| `enabled` | `true` | `false` turns the plugin off in this repository: no gate, no proofs on Stop, no board. (An older `"gate": "off"` still means `false`.) |
| `allow` | `[]` | Globs the gate never blocks. |
| `maxIterations` | `5` | How many failing proofs the Stop hook sends back before handing over. |

Local hook state and the last run's events live in `.lastro/plugin/`; add it to `.gitignore`. Commit the rest of `.lastro/`: the approved
intents and the proofs travel with the code.

## Status line (Claude Code)

```json
{ "statusLine": { "type": "command", "command": "node /path/to/plugins/lastro/scripts/lastro-plugin.mjs board --line" } }
```

prints `Lastro · proofs 2/3 passed · ✗ 1 failed · FAILED`.

## Limits

- The gate sees edit tools (Claude Code's `Edit`/`Write`/`MultiEdit`/`NotebookEdit`, Codex's `apply_patch`), not
  files changed from a shell command. The `Stop` hook still proves whatever the code is.
- One workspace (one decision core) per repository for now.

## Tests

`npm test -w @lastro/plugin` drives every hook through the real CLI on the docs' shipping example (build the TS
SDK first).
