# Review proposals in a web app

Intent review replaces code review, so it needs a place where a team does it together, like a pull request.

A web app (`apps/review`, Vite, React and shadcn/ui) lists the proposals and the use cases of a `.lastro/` workspace,
and lets people review a proposal's intents. It reads and writes the workspace through the TypeScript SDK, so every
action means what `lastro review` means; it runs on a server (Vite's dev server or `vite preview`), picked with
`LASTRO_DIR`, and defaults to the repository's own `.lastro/`.

The proposals page shows each proposal with its status, how many intents are settled, how many items block its
approval and how many intents wait for the current reviewer.

A proposal has three tabs. The conversation shows the prose, the decisions to answer and the threads. The use cases
changed tab shows one block per touched use case: its description before and after as a word diff, then its intents,
each with the quote it came from, the existing proofs approving it replaces, its approvals out of the policy's count,
and its threads; it can be filtered to the intents that are not settled or that wait for the reviewer. The proofs tab
shows the derived proofs, the superseded ones and the last run. Next to the tabs, a merge box says what still blocks
approval under the proposal's policy and who approved what.

Reviewers approve, amend or reject intents, answer decisions, start threads, reply, and resolve threads (only the
thread's author or the submitter can). After a reject or an amend, a reviewer writes the use case's after by hand or
asks the LLM to recombine it: with `ANTHROPIC_API_KEY` the server calls Claude, otherwise it writes the same handoff
request as the CLI. When nothing blocks, the app approves the proposal, as the CLI does. The submitter's approval is
refused when the policy says the submitter cannot approve, and names an agent would use (claude, codex, agent,
assistant, ai, bot) cannot review at all. An approved, applied or closed proposal is read-only.

The reviewer's name defaults to `LASTRO_REVIEWER` or `git config user.name` and can be changed per browser. People on
the same server review the same files: actions are applied one at a time to the latest file, and every open page
reloads when a file in the workspace changes. People on other machines share the proposal files through git.

The use cases page lists every use case with its proofs and filters them by text; a use case's page shows its
behavior, its regression proofs, its history and the changes open proposals make to it.
