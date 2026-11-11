# lastro-board: the Lastro board in Claude Code

A Claude Code mod that shows, beside the chat, where a Lastro change stands:

- **Review**: each decision (answered or not) and each intent with its approvals out of the policy's count, like a
  pull request waiting for reviewers.
- **Proofs**: one row per proof, updated as the run under way reports it: `○` pending, `…` running, `✓` proved,
  `≈` evidenced, `✗` violated (with its counterexample), `!` weak or vacuous.
- A one-line summary above the prompt and in the status line, and a toast when every intent is approved and when a
  proof run ends.

It reads `.lastro/` once a second (the files the `lastro` CLI writes, and `.lastro/plugin/progress.jsonl`, which
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
