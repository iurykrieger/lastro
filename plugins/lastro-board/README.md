# lastro-board: the Lastro board in Claude Code

A Claude Code mod that shows, beside the chat, where a Lastro change stands:

- **Review** of the current proposal: each decision (answered or not) and each intent, with the use case it changes,
  its approvals out of the policy's count and the proofs it replaces, like a pull request waiting for reviewers.
- **Proofs**: one row per proof, updated as the run under way reports it: `○` pending, `…` running, `✓` proved,
  `≈` evidenced, `✗` violated (with its counterexample), `!` weak or vacuous, then the use cases' proofs the change
  broke (regressions).
- A one-line summary above the prompt and in the status line, and a toast when every intent is approved and when a
  proof run ends.

It reads `.lastro/` once a second (the current proposal in `.lastro/proposals/`, the latest one not applied or
closed, and `.lastro/plugin/progress.jsonl`, which
the [Lastro plugin](../lastro)'s Stop hook fills with `lastro prove --stream`). It decides nothing. `/lastro-board`
opens the pane.

```
/plugin marketplace add iurykrieger/lastro
/plugin install lastro-board@lastro
```

Mods are a Claude Code feature: in Codex, the Lastro plugin's text board is shown instead.

## Tests

`claude plugin test plugins/lastro-board` mounts the pane and the band on several surfaces over workspace files the
real CLI and plugin wrote (`tests/fixtures.ts`), and walks a change from review to a failed proof run.
