import { A, C, Callout, Code, CodeTabs, DataTable, H1, H2, H3, Lead, P, Ul } from "@/components/doc";
import { OUT, STREAM, USECASE } from "@/content/outputs";

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

export function CliOverview() {
  return (
    <>
      <H1>Commands</H1>
      <Lead>
        The CLI is the SDK as a command line, for people and for any agent harness. Every command prints one JSON object on stdout and keeps its state as JSON
        files in <C>.lastro/</C>.
      </Lead>

      <P>
        There are three CLIs with the same commands, output and workspace: <C>lastro</C> (TypeScript), <C>lastro-go</C> (Go) and <C>lastro</C> (.NET tool). They
        differ only in how <C>prove</C> loads the code under proof.
      </P>

      <H2>Usage</H2>
      <Code
        lang="text"
        title="lastro (no arguments)"
        code={`lastro <command> [options]

  init --model <model.json> [--subject <module.js>[#export]] [--approvals N] [--submitter-cannot-approve]
                                                start a .lastro/ workspace (or add a model to it)
  propose <prose-file> [--section <heading>] [--as <submitter>] [--proposal <P>]
                                                create a proposal and extract its intents against the use cases;
                                                --proposal re-extracts an existing one and keeps its review
  scan [--scope <paths>] [--max-commits N] [--since <date>]
                                                read the repository (files, tests, commits) without an LLM:
                                                its size, areas and the scopes a bootstrap can be split into
  bootstrap [--scope <paths>] [--max-commits N] [--since <date>] [--budget <chars>] [--as <name>] [--proposal <P>]
                                                for an existing repository: survey the code and its history into
                                                a bootstrap proposal of the use cases it implements today, to
                                                review, derive, prove against the code as it is, and apply
  status [<P>]                                  proposals, or one proposal's intents and pending items
  review <P> --as <name> <action> ...           approve <I..> | approve-all | amend <I> <text> | reject <I> <note>
                                                answer <D> <option> | comment <I|PF|UC|prose> <text> | reply <T> <text>
                                                resolve <T> | rewrite <UC> <after> | recombine <UC> [instructions]
  derive <P>                                    derive the pending proofs of the approved intents
  prove <P> [<module.js>] [--export name] [--stream]
                                                run its proofs and every use case's proofs (regression);
                                                writes the receipt next to the proposal
  feedback <P>                                  the last run's failures, as a brief for whoever writes the code
  apply <P>                                     attach the proofs to their use cases and sync .lastro/usecases/
  regress [<module.js>] [--export name]         run every use case's proofs alone

  <P> is a proposal id (P-001, or 1) or slug; without it, the latest open proposal.
  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`}
      />

      <P>
        Everything lives in the workspace: <C>.lastro/config.json</C> (models, subjects, review policy), one file per use case in <C>.lastro/usecases/</C>, and
        one file per proposal in <C>.lastro/proposals/</C> with its prose (<C>.md</C>) and its last receipt (<C>.receipt.md</C>). See{" "}
        <A to="concepts/proposals">use cases and proposals</A>. A workspace from an older version (<C>intents.json</C>, <C>spec.json</C>, <C>proof.json</C>) is
        refused: delete those files and run <C>propose</C> again.
      </P>

      <H2>Exit codes</H2>
      <DataTable
        head={["Code", "Meaning"]}
        rows={[
          [<C>0</C>, "done, or ok"],
          [<C>1</C>, "not ok: review pending, decisions raised, refused (problems listed), or failing proofs"],
          [<C>2</C>, "usage error or failure (the JSON has an error field)"],
          [<C>3</C>, <>an LLM answer is needed (see <A to="cli/llm">LLM handoff</A>)</>],
        ]}
      />

      <H2>init</H2>
      <P>
        Starts a workspace from a model file, or adds a model to it. <C>--subject</C> declares the code under proof (a module and its export, relative to the
        folder that holds <C>.lastro/</C>) so <C>prove</C> and <C>regress</C> need no arguments. <C>--approvals</C> and <C>--submitter-cannot-approve</C> set the{" "}
        <A to="concepts/review">review policy</A> (default: one approval, and the submitter counts).
      </P>
      <Code lang="bash" code={`$ lastro init --model model.json --subject dist/shipping.js#shippingFee --approvals 2`} />
      <Code lang="json" title="lastro init --model model.json · stdout" code={OUT.init} />

      <H2>propose</H2>
      <P>
        Creates the next proposal (<C>P-001</C>, <C>P-002</C>...) from a prose file and extracts its intents against the current use cases and their proofs (AI
        step). <C>--as</C> records who submitted the prose. Each intent adds, changes or removes one use case and lists the existing proofs it replaces on purpose.
        Exits 1 while the review is pending. To change the prose, edit <C>.lastro/proposals/P-001-&lt;slug&gt;.md</C> and run <C>propose</C> on it again (or pass{" "}
        <C>--proposal P-001</C>): intents that did not change keep their review.
      </P>
      <Code lang="bash" code={`$ lastro propose docs/shipping.md --as ana`} />

      <H2>status</H2>
      <P>Without a proposal: the use cases, their proofs, the unproved ones and every proposal with its status. With one: its intents, approvals, decisions, threads and what is pending. Exit 0 means nothing is pending.</P>
      <Code lang="json" title="lastro status · stdout" code={OUT.status} />

      <H2>review</H2>
      <P>
        Reviews a proposal as one person. <C>--as</C> is required, so every approval is signed. Intent ids can be short (<C>I-01</C>). Amending or rejecting an
        intent makes its use case's description stale: <C>rewrite</C> it by hand or <C>recombine</C> it (AI, part of the extract step); either way that use
        case's intents need review again. When nothing is pending, the proposal is approved, bound to the current use cases.
      </P>
      <Code
        lang="bash"
        code={`$ lastro review P-001 --as ana approve I-01 I-02
$ lastro review P-001 --as ana approve-all
$ lastro review P-001 --as ana amend I-03 "A non-member order under 100 pays exactly 15."
$ lastro review P-001 --as ana reject I-04 "Out of scope for this change"
$ lastro review P-001 --as ana answer D1 1          # option index, from 0
$ lastro review P-001 --as bob comment I-02 "Gold members only?"
$ lastro review P-001 --as bob resolve T1
$ lastro review P-001 --as ana recombine UC-001 "keep the wording short"`}
      />

      <H2>derive</H2>
      <P>
        Derives the pending proofs of the approved and amended intents (AI step), each a clause or a scenario naming its model and subject. Refuses while the
        review is pending (status <C>review-pending</C>). Contradictions, a removal the model cannot express, or an existing proof the change breaks without an
        intent replacing it come back as decisions: the proposal goes back to review (status <C>back-to-review</C>, exit 1).
      </P>

      <H2>prove</H2>
      <P>
        Runs the proposal's proofs and every use case's proofs (a failing one is a regression) against the code, and writes the receipt next to the proposal.
        Exits 1 when anything fails. Its output includes the failures, the regressions, the replaced proofs that still pass, the unproved use cases and the
        feedback brief.
      </P>
      <CodeTabs
        variants={[
          {
            label: "TypeScript",
            lang: "bash",
            title: "an ES module that exports the function",
            code: `$ lastro prove P-001                            # the subjects declared in .lastro/config.json
$ lastro prove P-001 ./decide.js                # the default export
$ lastro prove P-001 ./decide.js --export decideLogin`,
          },
          {
            label: "Go",
            lang: "bash",
            title: "any program, or a recorded table",
            code: `$ lastro-go prove P-001 --cmd "./decide"       # JSON lines: one input per line in, one output per line out
$ lastro-go prove P-001 --table rows.json       # a recorded input/output table
$ lastro-go prove P-001 --cmd "python3 decide.py" --prover ./target/release/lastro-prove`,
          },
          {
            label: ".NET",
            lang: "bash",
            title: "a public static method in an assembly",
            code: `$ lastro prove P-001 ./bin/Release/net8.0/Shop.dll --export Shop.Shipping.Fee`,
          },
        ]}
      />
      <Code lang="json" title="lastro prove P-001 ./shipping-buggy.mjs · stdout · exit 1" code={OUT.notProved} />
      <P>
        The TypeScript CLI runs the Rust prover from <C>wasm/lastro_core.wasm</C> when it was built; the Go and .NET CLIs use the <C>lastro-prove</C> binary. Either
        way, <C>enginesAgree</C> in the output says whether both engines gave every proof the same verdict.
      </P>

      <H3>Streaming a run</H3>
      <P>
        With <C>--stream</C>, every CLI prints one JSON object per line as the run goes, instead of the single result: <C>start</C> (the proposal and its proof
        ids), a <C>progress</C> event every 100 runs of the code under proof, one <C>proof</C> event per proof (its role: <C>proposal</C>, <C>regression</C> or{" "}
        <C>superseded</C>, its verdict, and its counterexample when it fails), <C>crosscheck</C> when the Rust prover ran, and <C>result</C>, which carries the
        same body as without the flag. Exit codes and files are unchanged. The <A to="plugins/agents">agent plugins</A> use it to show each proof as it is
        checked.
      </P>
      <Code lang="json" title="lastro prove P-001 ./shipping-buggy.mjs --stream" code={STREAM} />

      <H2>feedback</H2>
      <P>Prints the failures of the proposal's last run (its proofs and the regressions) as a brief for whoever writes the code. Exit 1 while there are failures.</P>
      <Code lang="json" title="stdout" code={OUT.feedback} />

      <H2>apply</H2>
      <P>
        Runs only when the proposal is approved, its baseline still matches the use cases, and its last run passed every proof, new and regression. It writes
        each use case the proposal adds or changes, deletes the removed ones, moves each proof to its use case as a regression test, drops the superseded proofs
        and records the proposal in each use case's <C>history</C>. Use cases still without proofs are reported as <C>unproved</C>; they do not block it.
      </P>
      <Code lang="json" title="lastro apply P-001 · stdout" code={OUT.applied} />

      <H2>scan and bootstrap</H2>
      <P>
        For a repository that already has code: <C>scan</C> reads its files, tests and commits without an LLM, and <C>bootstrap</C> turns them into a bootstrap
        proposal through the <C>survey</C> step, reviewed, derived, proved and applied like any other. See <A to="bootstrap">Existing repositories</A>.
      </P>

      <H2>regress</H2>
      <P>Runs every use case's proofs alone, against the declared subjects or one module. Exit 1 on any regression.</P>
      <Code lang="json" title="lastro regress ./shipping.mjs · stdout" code={OUT.regress} />

      <H2>Global flags</H2>
      <DataTable
        head={["Flag", "Default", "Meaning"]}
        rows={[
          [<C>--dir &lt;path&gt;</C>, <C>.lastro</C>, "The workspace directory. Keep one per decision core."],
          [<C>--llm anthropic|handoff</C>, <>anthropic if <C>ANTHROPIC_API_KEY</C> is set, else handoff</>, <>Where the AI steps run. See <A to="cli/llm">LLM handoff</A>.</>],
        ]}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// Workspace
// ---------------------------------------------------------------------------

export function CliWorkspace() {
  return (
    <>
      <H1>The .lastro/ workspace</H1>
      <Lead>Plain JSON files, in the same format the SDKs use in memory. Commit them, diff them in review, or hand them to another CLI.</Lead>
      <Code
        lang="text"
        title=".lastro/"
        code={`.lastro/
├── config.json                         models, subjects and the review policy (init)
├── usecases/
│   └── UC-001-charge-shipping.json     one use case: its description, its proofs, its history (apply)
├── proposals/
│   ├── P-001-shipping-fee.md           the prose (propose)
│   ├── P-001-shipping-fee.json         the proposal: intents, review, proofs, last run (propose … prove)
│   ├── P-001-shipping-fee.receipt.md   the receipt for the pull request (prove, apply)
│   └── P-001-shipping-fee.runs.jsonl   every prove run, one JSON line each (prove)
└── llm/
    ├── extract-<id>.request.json       what an AI step needs (handoff)
    ├── extract-<id>.answer.json        the answer the calling agent wrote
    ├── derive-<id>.request.json
    └── derive-<id>.answer.json`}
      />
      <DataTable
        head={["File", "Written by", "Contents"]}
        rows={[
          [<C>config.json</C>, <C>init</C>, <><C>{`{ models, subjects, review: { approvals, submitterCanApprove } }`}</C> (the older <C>model</C> and <C>requiredApprovals</C> keys are still read)</>],
          [<C>usecases/&lt;id&gt;-&lt;slug&gt;.json</C>, <C>apply</C>, <><C>{`{ id, slug, name, description, proofs, history }`}</C>: the application, one use case per file</>],
          [<C>proposals/&lt;id&gt;-&lt;slug&gt;.json</C>, <><C>propose</C>, <C>review</C>, <C>derive</C>, <C>prove</C>, <C>apply</C></>, <>a <A to="concepts/proposals">proposal</A>: prose, intents, use-case changes, decisions, threads, policy, proofs, last run</>],
          [<C>proposals/&lt;id&gt;-&lt;slug&gt;.md</C>, <><C>propose</C>, or you</>, "the prose; edit it and run propose on it again"],
          [<C>proposals/&lt;id&gt;-&lt;slug&gt;.receipt.md</C>, <><C>prove</C>, <C>apply</C></>, "the Markdown receipt"],
          [<C>proposals/&lt;id&gt;-&lt;slug&gt;.runs.jsonl</C>, <C>prove</C>, <>one line per run: <C>{`{ run, at, commit?, dirty?, ok, outcomes, regressions, replacedStillPass, unproved, enginesAgree? }`}</C>. The only file with timestamps; nothing hashes it</>],
          [<C>llm/*.json</C>, <>handoff, and the calling agent</>, "AI requests and their answers"],
        ]}
      />
      <P>A use case after <C>apply</C>, with the proofs that now guard it:</P>
      <Code lang="json" title=".lastro/usecases/UC-001-charge-shipping.json" code={USECASE} />

      <H2>What to commit</H2>
      <P>
        Commit the whole workspace. The proposals make the review itself reviewable in a pull request, and the use cases carry the proofs every later change is
        checked against. Committing the <C>llm/*.answer.json</C> files makes the AI steps replayable: a CI job can re-run <C>propose</C>, <C>derive</C> and{" "}
        <C>prove</C> with <C>--llm handoff</C> and no model, and get the same hashes, because a request whose answer file exists is answered from the file.
      </P>

      <H2>Several decision cores</H2>
      <P>
        Use one workspace per decision core with <C>--dir</C>:
      </P>
      <Code
        lang="bash"
        code={`$ lastro init --dir .lastro/login --model login/model.json
$ lastro propose --dir .lastro/login login/prose.md
$ lastro prove P-001 --dir .lastro/shipping ./shipping.js`}
      />

      <H2>One workspace, three CLIs</H2>
      <P>
        The TypeScript, Go and .NET CLIs share the format, the hashes and even the handoff request ids. One person can propose with <C>lastro</C>, another
        review and prove a Go service with <C>lastro-go</C>:
      </P>
      <Code
        lang="bash"
        code={`$ lastro propose prose.md && lastro review P-001 --as ana approve-all && lastro derive P-001
$ lastro-go prove P-001 --cmd ./shipping     # continues the same .lastro/`}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// LLM handoff
// ---------------------------------------------------------------------------

export function CliLlm() {
  return (
    <>
      <H1>LLM handoff</H1>
      <Lead>
        The two AI steps can run on the Anthropic API, or be handed to the agent that is calling the CLI, which answers with its own model. No API key, no plugin
        for a specific harness.
      </Lead>

      <H2>Choosing a mode</H2>
      <DataTable
        head={["Mode", "When", "How"]}
        rows={[
          [<C>--llm anthropic</C>, <>default when <C>ANTHROPIC_API_KEY</C> is set</>, "Calls Claude through the Anthropic SDK of the CLI's language, with structured output."],
          [<C>--llm handoff</C>, "default otherwise", "Writes the request to a file and exits 3. The caller answers and runs the same command again."],
        ]}
      />
      <Callout kind="note" title="TypeScript CLI and the Anthropic SDK">
        <C>@anthropic-ai/sdk</C> is an optional peer dependency of the TypeScript SDK. Install it to use <C>--llm anthropic</C>; the handoff needs nothing.
      </Callout>

      <H2>The protocol</H2>
      <Code lang="bash" code={`$ lastro propose prose.md --as ana`} />
      <Code lang="json" title="stdout · exit 3" code={OUT.proposeNeedsLlm} />
      <P>The request file has everything a model needs:</P>
      <DataTable
        head={["Field", "Contents"]}
        rows={[
          [<C>instructions</C>, "What to do: act as the system, read the user message, write only a JSON value matching the schema to the answer path, then run the same command again."],
          [<C>step</C>, <><C>extract</C> (intents from prose, or recombining a use case's description) or <C>derive</C> (proofs)</>],
          [<C>system</C>, "The step's system prompt, identical in every SDK."],
          [<C>user</C>, "The data: the prose and the use cases with their proofs, or the models and the approved intents."],
          [<C>schema</C>, "The JSON Schema the answer must match."],
        ]}
      />
      <P>
        The id is the step plus the first 12 hex digits of a SHA-256 over the request, so the same question always gets the same file name, in every CLI. When the
        answer file exists, the CLI reads it instead of asking. Answers are never trusted blindly: quotes are anchored in the prose, and LSL is validated against
        the model.
        Use-case references, replaced proofs and coverage are checked too, and every problem comes back at once.
      </P>

      <H2>Wiring it into an agent</H2>
      <P>Any agent that can run commands and write files can drive Lastro. The whole loop, in pseudo-shell:</P>
      <Code
        lang="bash"
        title="a harness-agnostic driver"
        code={`run() {
  out=$(lastro "$@"); code=$?
  while [ $code -eq 3 ]; do
    req=$(echo "$out" | jq -r .request); ans=$(echo "$out" | jq -r .answer)
    your-model --system "$(jq -r .system "$req")" --user "$(jq -r .user "$req")" \\
               --schema "$(jq -c .schema "$req")" > "$ans"
    out=$(lastro "$@"); code=$?
  done
  echo "$out"; return $code
}

run propose prose.md --as ana
# … people review: lastro review P-001 --as <name> …
run derive P-001
run prove P-001 ./decide.js || lastro feedback P-001
# … when every proof passes: lastro apply P-001`}
      />
      <H3>Telling your agent</H3>
      <P>
        For Claude Code or Codex, the <A to="plugins/agents">Lastro plugin</A> does this and more: it gates code edits on your review and proves the code when
        the agent stops. Without the plugin, a short instruction in the agent's project notes is enough:
      </P>
      <Code
        lang="markdown"
        title="AGENTS.md / CLAUDE.md"
        code={`When a \`lastro\` command exits with code 3, open the \`request\` file it printed. Act as the
system prompt in \`system\`, read \`user\`, and write ONLY a JSON value matching \`schema\` to the
\`answer\` path. Then run the same command again. Never edit .lastro/usecases/ or
.lastro/proposals/*.json by hand: use \`lastro review\`.`}
      />
      <Callout kind="warning" title="The agent answers, people review">
        The handoff lets an agent do the AI steps, not the human one. Approvals still come from <C>lastro review --as &lt;name&gt;</C>, and your policy decides how
        many are needed.
      </Callout>
    </>
  );
}
