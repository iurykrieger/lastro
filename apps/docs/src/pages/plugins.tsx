import { A, C, Callout, Code, DataTable, H1, H2, Lead, P, Step, Steps } from "@/components/doc";
import { BOARDS } from "@/content/outputs";

// ---------------------------------------------------------------------------
// Claude Code and Codex
// ---------------------------------------------------------------------------

export function PluginAgents() {
  return (
    <>
      <H1>Claude Code and Codex</H1>
      <Lead>
        One plugin for both harnesses. Every implementation the agent proposes becomes intents you review like a pull request. Once you approve them, the proofs
        are derived into the repository, and the agent's code is checked against them before it stops.
      </Lead>

      <P>
        The plugin lives in <C>plugins/lastro</C>. It wraps the <A to="cli/overview">CLI</A>, so it decides nothing itself: every intent, decision, spec and proof
        comes from <C>lastro</C> and the files in <C>.lastro/</C>. It adds three things the CLI alone cannot: a gate that keeps code edits behind your review, a
        hook that runs the proofs when the agent stops, and a progress board.
      </P>

      <H2>Install</H2>
      <Code lang="text" title="Claude Code" code={`/plugin marketplace add iurykrieger/lastro
/plugin install lastro@lastro`} />
      <Code lang="bash" title="Codex" code={`$ codex plugin marketplace add iurykrieger/lastro`} />
      <P>
        To turn it on for everyone who works in a repository, commit a <C>.claude/settings.json</C> that declares the marketplace and enables the plugin; Claude
        Code offers to install it when someone trusts the folder. The Lastro repository does this for itself.
      </P>
      <Code
        lang="json"
        title=".claude/settings.json"
        code={`{
  "extraKnownMarketplaces": { "lastro": { "source": { "source": "github", "repo": "iurykrieger/lastro" } } },
  "enabledPlugins": { "lastro@lastro": true, "lastro-board@lastro": true }
}`}
      />
      <P>
        In Codex, enable <C>lastro</C> from <C>/plugins</C>. On Codex versions where hooks are still behind a flag, set <C>codex_hooks = true</C> under{" "}
        <C>[features]</C> in <C>~/.codex/config.toml</C>. The plugin needs Node 20 or later and a Lastro CLI (<C>npm i -D lastro</C>, or <C>lastro-go</C>).
      </P>

      <H2>The flow</H2>
      <Steps>
        <Step title="Set up the repository">
          <P>
            Ask the agent to set up Lastro (the <C>setup</C> skill). It drafts the <A to="concepts/spec">model</A> of the decision core with you, runs{" "}
            <C>lastro init</C>, and writes <C>.lastro/plugin.json</C>.
          </P>
        </Step>
        <Step title="The agent proposes, before its first edit">
          <P>
            Whatever the mode (plan mode, auto mode, an informal plan in the chat, or none), the first attempt to edit source code is the trigger. The edit is
            denied with the steps: write the change as prose in <C>.lastro/proposals/&lt;slug&gt;.md</C>, run <C>lastro plan</C>, start your review (the{" "}
            <C>propose</C> skill). The agent answers the <A to="cli/llm">LLM handoff</A> with its own model. When Claude Code leaves plan mode, the plan is
            saved there as a draft right away.
          </P>
        </Step>
        <Step title="You review, like a pull request">
          <P>
            The intents are the code review. The agent is the author and never approves. It asks you each open decision, then asks you to approve, request changes
            to (amend) or reject each intent, and records only your answers with <C>lastro review --as you</C>. In Claude Code every review command also shows a
            permission prompt: that click is your approval. With <C>--approvals 2</C>, the proposal waits for a second reviewer.
          </P>
          <Code lang="text" title="the board, review pending" code={BOARDS.review} />
        </Step>
        <Step title="The proofs are derived into the repository">
          <P>
            With every intent approved, <C>lastro spec</C> writes the proofs to <C>.lastro/spec.json</C>. The gate opens. The agent implements from the approved
            intents; there is no other brief.
          </P>
          <Code lang="text" title="the board, implementing" code={BOARDS.implementing} />
        </Step>
        <Step title="The harness checks the implementation">
          <P>
            When the agent stops, the <C>Stop</C> hook builds and runs <C>lastro prove</C>. Failing proofs block the stop and go back to the agent with their
            counterexamples, up to <C>maxIterations</C>; then you take over. The board shows each proof: <C>✓</C> proved, <C>≈</C> evidenced, <C>✗</C> violated,{" "}
            <C>!</C> weak or vacuous, <C>○</C> pending.
          </P>
          <Code lang="text" title="the board, a proof failed" code={BOARDS.failed} />
          <P>
            Once every proof passes, the change is done: the next source edit is a new change, and the flow starts again with its own intents.
          </P>
        </Step>
      </Steps>

      <H2>Hooks</H2>
      <DataTable
        head={["Event", "What the plugin does"]}
        rows={[
          [<C>SessionStart</C>, "Adds the board and the rules (the agent is the author, the user reviews) to the agent's context."],
          [<C>UserPromptSubmit</C>, "The same, on every prompt, so each request that changes behavior goes through a proposal."],
          [<C>PreToolUse</C>, "Denies the first source edit of a change, in any mode, with the steps to extract its intents; lets edits through only while the approved spec's proofs have not passed yet. Denies edits to files only the CLI writes. Asks you before each review command; refuses --as with an agent's name."],
          [<C>PostToolUse</C>, "Shows the board after each lastro command; while the review is open, hands the agent the review as Markdown (one list item per decision and intent, grouped, with its quote) to show the user as-is. Marks the last proof stale after a code edit. Saves Claude Code's plan (ExitPlanMode) as a proposal draft; reminds Codex after update_plan."],
          [<C>Stop</C>, "Builds, runs lastro prove --stream into .lastro/plugin/progress.jsonl, and sends failing proofs back to the agent; shows the board either way."],
        ]}
      />
      <P>
        Both harnesses send the same hook input and read the same answers, and Codex sets <C>CLAUDE_PLUGIN_ROOT</C> for plugin hooks, so one script serves both.
        Codex has no per-command permission prompt from hooks: there, review commands follow your Codex approval policy.
      </P>

      <H2>Configuration</H2>
      <Code
        lang="json"
        title=".lastro/plugin.json"
        code={`{
  "cli": "npx lastro",
  "build": "npm run build",
  "prove": "dist/shipping.js --export shippingFee",
  "gate": "on",
  "allow": ["docs/**", "**/*.md"],
  "maxIterations": 5
}`}
      />
      <DataTable
        head={["Key", "Default", "Meaning"]}
        rows={[
          [<C>cli</C>, <C>lastro</C>, <>How to run the CLI. The <C>LASTRO_CLI</C> environment variable overrides it.</>],
          [<C>dir</C>, <C>.lastro</C>, "The workspace."],
          [<C>build</C>, "none", "Runs before each proof."],
          [<C>prove</C>, "none", <>The arguments after <C>lastro prove</C>.</>],
          [<C>gate</C>, <C>on</C>, <><C>off</C> never blocks edits.</>],
          [<C>allow</C>, <C>[]</C>, "Paths (globs) the gate never blocks."],
          [<C>maxIterations</C>, <C>5</C>, "How many failing proofs the Stop hook sends back before handing over to you."],
        ]}
      />
      <Callout kind="note" title="What to commit">
        Commit <C>.lastro/</C> with the change: the approved intents and the proofs travel with the code. Ignore <C>.lastro/plugin/</C>, which holds local hook
        state.
      </Callout>

      <H2>The live board in Claude Code</H2>
      <P>
        The <C>lastro-board</C> mod draws the same board in a pane beside the chat: each decision, each intent with its approvals out of the policy's count, and
        each proof, updated as <C>lastro prove --stream</C> reports it (<C>…</C> while it runs). A one-line summary sits above the prompt, and a toast says when
        every intent is approved and when a run ends. It reads <C>.lastro/</C> and decides nothing. Mods are a Claude Code feature; Codex shows the text
        board.
      </P>
      <Code lang="text" title="Claude Code" code={`/plugin install lastro-board@lastro`} />

      <H2>A status line</H2>
      <P>In Claude Code, the board fits a status line:</P>
      <Code
        lang="json"
        title="~/.claude/settings.json"
        code={`{ "statusLine": { "type": "command", "command": "node /path/to/plugins/lastro/scripts/lastro-plugin.mjs board --line" } }`}
      />
      <Code lang="text" title="status line" code={`Lastro · proofs 2/3 passed · ✗ 1 failed · FAILED`} />

      <Callout kind="warning" title="Limits">
        The gate sees edit tools (<C>Edit</C>, <C>Write</C>, <C>MultiEdit</C>, <C>NotebookEdit</C>, <C>apply_patch</C>), not files a shell command changes. The
        Stop hook still proves whatever the code is. One workspace per repository for now.
      </Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Review app
// ---------------------------------------------------------------------------

export function PluginReviewApp() {
  return (
    <>
      <H1>Review app</H1>
      <Lead>
        A web app to browse the use cases, their proofs and the proposals, and to review intents together, the way a team reviews a pull request. Intent review
        replaces code review, so it gets the same tools: a diff per use case, approvals per reviewer, threads, and a merge box that says what still blocks.
      </Lead>

      <H2>Run it</H2>
      <Code lang="bash" title="from the repository" code={`$ npm install
$ npm run review                              # this repository's .lastro/, at http://localhost:5175
$ LASTRO_DIR=../shop/.lastro npm run review   # any other workspace`} />
      <P>
        The app lives in <C>apps/review</C>. It reads and writes the <A to="cli/workspace">workspace</A> through the TypeScript SDK (<C>reviewProposal</C>,{" "}
        <C>approveProposal</C>, <C>recombineAfter</C>), so every action means exactly what <C>lastro review</C> means. It runs on a small server (Vite's dev
        server, or <C>vite preview</C> after a build), never as a static page, because it writes files.
      </P>

      <H2>What you see</H2>
      <DataTable
        head={["Page", "What it shows"]}
        rows={[
          ["Proposals", "Every proposal with its status, how many intents are settled, what blocks it, and how many intents wait for you."],
          ["Conversation", "The prose, the decisions to answer, the threads, and what the proposal does not promise."],
          [
            "Use cases changed",
            "One block per touched use case, like a changed file: its description before and after as a word diff, then its intents. Each intent shows the quote it came from and the proofs approving it replaces. Filter by all, not settled, or waiting for me.",
          ],
          ["Proofs", "The proofs derived for the proposal, the ones it supersedes, and the last run."],
          ["Use cases", "The application today: every use case, its regression proofs, its history, and the changes open proposals make to it."],
        ]}
      />

      <H2>Reviewing</H2>
      <P>
        Approve, amend or reject each intent, answer each decision, and comment in threads. Rejecting or amending an intent makes its use case's after stale: write
        it by hand (<C>Edit after</C>) or ask the LLM to recombine it from the remaining intents; either way those intents need review again. When nothing blocks,
        the app approves the proposal under its policy, as the CLI does, and the approved intents become the brief for whoever writes the code.
      </P>
      <Callout kind="note" title="People review, agents author">
        The LLM is the author of the intents; every person is a reviewer. The app refuses agent names (<C>claude</C>, <C>codex</C>, <C>agent</C>,{" "}
        <C>assistant</C>, <C>ai</C>, <C>bot</C>) as reviewers, and the policy in <C>.lastro/config.json</C> decides how many approvals an intent needs and
        whether the submitter counts.
      </Callout>

      <H2>Reviewing together</H2>
      <P>
        Collaboration is file-backed, like the rest of Lastro. Who you are is the name in the header: <C>LASTRO_REVIEWER</C> or <C>git config user.name</C> by
        default, changeable per browser. Everyone who opens the same server reviews the same files: actions are applied one at a time to the latest file, and every
        open page updates as soon as a file in <C>.lastro/</C> changes, whether a reviewer, an agent running the CLI, or a <C>git pull</C> changed it. Reviewers on
        other machines run the app on their own checkout and share the proposal file through git, like any other change.
      </P>
      <P>
        Recombining an after asks the LLM: with <C>ANTHROPIC_API_KEY</C> set on the server it calls Claude; without it, it writes the same{" "}
        <A to="cli/llm">handoff</A> request as the CLI to <C>.lastro/llm/</C>, an agent answers it, and you recombine again.
      </P>
    </>
  );
}
