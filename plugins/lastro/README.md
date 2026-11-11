# Lastro plugin for Claude Code and Codex

One plugin, two harnesses. It wraps the `lastro` CLI so that every implementation an agent proposes goes through
Lastro before any code is written:

1. **Before the first source edit, in any mode** (plan mode, auto mode, an informal plan or none), the agent writes
   the change as prose in `.lastro/proposals/<slug>.md` and runs `lastro plan`. The gate enforces it: the first edit
   is denied with these steps. Leaving Claude Code's plan mode saves the plan there as a draft.
2. **The user reviews the intents like a pull request**: answers every decision, then approves, requests changes
   (amend) or rejects each intent. The agent is the author and never approves; the policy
   (`lastro init --approvals N`) sets how many reviewers each intent needs.
3. Once every intent is approved, `lastro spec` derives the proofs into `.lastro/spec.json`, in the repository
   being changed.
4. The agent implements from the approved intents. When it stops, the plugin builds and runs `lastro prove`; if
   a proof fails, the failures go back to the agent and it keeps going (up to `maxIterations`).
5. A progress board shows where the change stands: review, approvals, proofs `✓` passed, `✗` failed, `○` pending.
   In Claude Code, the [lastro-board](../lastro-board) mod shows it in a pane beside the chat, with each proof
   updating live as `lastro prove --stream` reports it.
6. Once every proof passes, the next source edit is a new change and starts again at step 1.

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
| `skills/propose` | Proposal → intents → the user's review (decisions, approve / request changes / reject) → `lastro spec`. |
| `skills/verify` | Prove on demand, read counterexamples, fix the code, never touch intents or proofs. |
| `hooks/hooks.json` | `SessionStart` adds the board and the rules; `UserPromptSubmit` adds one short line, only when the board changed; `PreToolUse` gates code edits and review commands; `PostToolUse` shows the board after each `lastro` command and marks proofs stale after an edit; `Stop` proves and sends failures back. |
| `scripts/lastro-plugin.mjs` | The hook entry point, plus `board [--line]` and `prove` for people and status lines. |

## The hooks

| Event | What happens |
|---|---|
| `PreToolUse` on edits | The first source edit of a change, in any mode, is denied with the steps to extract its intents and start the review. Edits go through only while the approved spec's proofs have not passed yet; after they pass, the next edit is a new change. `.lastro/` proposals and handoff answers, and the `allow` globs, are always allowed. `.lastro/intents.json`, `spec.json`, `proof.json`, `receipt.md` and `config.json` are written only by the CLI. |
| `PreToolUse` on `lastro review` | Claude Code shows a permission prompt: that click is the user's review. `--as claude` (or any agent name) is refused in both harnesses. |
| `PostToolUse` | After a `lastro` command, the board is shown to the user. After a code edit, the last proof is marked stale. After `ExitPlanMode` (Claude Code), the plan is saved as `.lastro/proposals/<slug>.md`; after `update_plan` (Codex), a reminder. |
| `Stop` | With a current spec: build, then `lastro prove --stream` into `.lastro/plugin/progress.jsonl` (what the mod draws). Failing proofs block the stop and the feedback goes back to the agent, up to `maxIterations`; then the user takes over. The board is shown either way. |

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
  "gate": "on",
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
| `gate` | `on` | `off` never blocks edits. |
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
