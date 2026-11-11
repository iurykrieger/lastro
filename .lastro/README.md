# Lastro on Lastro

This folder holds Lastro's own workspaces: the framework checks itself with its own CLI. The prose is
[`USE-CASES.md`](../USE-CASES.md) at the repository root; each section there is one decision core with its
own workspace here.

| Workspace | Section of `USE-CASES.md` | Code it will judge |
|---|---|---|
| `review-gate/` | Review gate | `Review`, `pendingReview`, `approvedIntents`, `freeze` (`sdks/typescript/src/artifact.ts`) |
| `clause-verdicts/` | Clause verdicts | `verify` (`sdks/typescript/src/verify/engine.ts`) |
| `re-extraction/` | Re-extraction keeps the review | `carryReview` (`sdks/typescript/src/artifact.ts`) |

Each workspace has the usual CLI state: `config.json` (model and review policy), `intents.json` (the extracted
intents and their review) and `llm/` (each AI request and the answer it got, so every step can be replayed
without a model). `model.json` is the proof vocabulary, used once the intents are approved.

## Status

Intents are extracted and **waiting for review**. No proof has been derived yet.

## Reviewing

Build the CLI once (`npm install && npm run build -w lastro`), then, per workspace:

```sh
lastro() { node sdks/typescript/dist/src/cli.js "$@"; }

lastro status --dir .lastro/review-gate                                  # intents, decisions, what is pending
lastro review --dir .lastro/review-gate --as <you> approve I1 I2          # or approve-all
lastro review --dir .lastro/review-gate --as <you> amend I5 "new wording"
lastro review --dir .lastro/review-gate --as <you> reject I3 "why"
lastro review --dir .lastro/review-gate --as <you> answer D2 1            # option index, from 0
```

## Changing a use case

Edit `USE-CASES.md`, then extract that section again:

```sh
lastro plan USE-CASES.md --section "Review gate" --dir .lastro/review-gate
```

Without `ANTHROPIC_API_KEY` the CLI hands the step to you: it writes the request to `llm/`, exits 3, and you (or
your agent) write the JSON answer to the named file and run the command again. Intents whose statement did not
change keep their review; only the new ones need a look.
