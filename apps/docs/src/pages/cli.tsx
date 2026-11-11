import { A, C, Callout, Code, CodeTabs, DataTable, H1, H2, H3, Lead, P, Ul } from "@/components/doc";
import { OUT, STREAM } from "@/content/outputs";

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

  init --model <model.json> [--approvals N]     start a .lastro/ workspace
  plan <prose-file>                             extract intents (keeps earlier review)
  status                                        intents, approvals, pending items
  review --as <name> <action> ...               approve <I1 I2..> | approve-all | amend <I> <text>
                                                reject <I> <note> | answer <D> <option-index>
  spec                                          derive the formal spec; contradictions come back as decisions
  prove <module.js> [--export name] [--traces file.json] [--stream]
                                                check the code against the spec; writes proof.json, receipt.md;
                                                --stream prints one JSON line per event (see "stream")
  feedback                                      the last proof's failures, as a brief for whoever writes the code

  --dir <path>          workspace (default .lastro)
  --llm anthropic|handoff`}
      />

      <H2>Exit codes</H2>
      <DataTable
        head={["Code", "Meaning"]}
        rows={[
          [<C>0</C>, "done, or ok"],
          [<C>1</C>, "not ok: review pending, contradictions, or failing proofs"],
          [<C>2</C>, "usage error or failure (the JSON has an error field)"],
          [<C>3</C>, <>an LLM answer is needed (see <A to="cli/llm">LLM handoff</A>)</>],
        ]}
      />

      <H2>init</H2>
      <P>
        Starts a workspace from a model file. <C>--approvals</C> sets the <A to="concepts/review">approval policy</A> (default 1).
      </P>
      <Code lang="bash" code={`$ lastro init --model model.json --approvals 2`} />
      <Code lang="json" title="stdout" code={OUT.init} />

      <H2>plan</H2>
      <P>
        Extracts intents from a prose file (AI step). If <C>.lastro/intents.json</C> exists, its review carries over to the new extraction. Exits 1 while anything is
        pending.
      </P>
      <Code lang="bash" code={`$ lastro plan docs/shipping.md`} />

      <H2>status</H2>
      <P>Prints the intents, their approvals, the decisions and what is pending. Exit 0 means the review is complete.</P>
      <Code lang="json" title="lastro status · stdout" code={OUT.status} />

      <H2>review</H2>
      <P>
        Reviews as one person. <C>--as</C> is required, so every approval is signed.
      </P>
      <Code
        lang="bash"
        code={`$ lastro review --as ana approve I1 I2
$ lastro review --as ana approve-all
$ lastro review --as ana amend I3 "A non-member order under 100 pays exactly 15."
$ lastro review --as ana reject I4 "Out of scope for this change"
$ lastro review --as ana answer D1 1          # option index, from 0`}
      />

      <H2>spec</H2>
      <P>
        Approves the reviewed intents, derives the formal spec (AI step), validates it, and checks it for contradictions. Writes <C>.lastro/spec.json</C>. With
        contradictions, it adds decisions to <C>intents.json</C> instead and exits 1 with status <C>contradictions</C>. Refuses to run while the review is pending
        (status <C>review-pending</C>).
      </P>

      <H2>prove</H2>
      <P>
        Checks the code against the spec. Writes <C>.lastro/proof.json</C> and <C>.lastro/receipt.md</C>, and exits 1 when anything fails. Its output includes the
        failures and the feedback brief.
      </P>
      <CodeTabs
        variants={[
          {
            label: "TypeScript",
            lang: "bash",
            title: "an ES module that exports the function",
            code: `$ lastro prove ./decide.js                    # the default export
$ lastro prove ./decide.js --export decideLogin
$ lastro prove ./decide.js --traces traces.json # also check recorded production runs`,
          },
          {
            label: "Go",
            lang: "bash",
            title: "any program, or a recorded table",
            code: `$ lastro-go prove --cmd "./decide"             # JSON lines: one input per line in, one output per line out
$ lastro-go prove --table rows.json             # a recorded input/output table
$ lastro-go prove --cmd "python3 decide.py" --prover ./target/release/lastro-prove`,
          },
          {
            label: ".NET",
            lang: "bash",
            title: "a public static method in an assembly",
            code: `$ lastro prove ./bin/Release/net8.0/Shop.dll --export Shop.Shipping.Fee`,
          },
        ]}
      />
      <P>
        The TypeScript CLI runs the Rust prover from <C>wasm/lastro_core.wasm</C> when it was built; the Go and .NET CLIs use the <C>lastro-prove</C> binary. Either
        way, <C>enginesAgree</C> in the output says whether both engines gave every clause the same verdict.
      </P>

      <H3>Streaming a run</H3>
      <P>
        With <C>--stream</C>, every CLI prints one JSON object per line as the run goes, instead of the single result: <C>start</C>, a <C>progress</C> event
        every 100 runs of the code under proof, one <C>clause</C> event per clause (with its counterexample when it fails), one <C>scenario</C> event per
        scenario, <C>crosscheck</C> when the Rust prover ran, and <C>result</C>, which carries the same body as without the flag. Exit codes and files are
        unchanged. The <A to="plugins/agents">agent plugins</A> use it to show each proof as it is checked.
      </P>
      <Code lang="json" title="lastro prove ./shipping-buggy.mjs --stream" code={STREAM} />

      <H2>feedback</H2>
      <P>Prints the failures of the last proof as a brief for whoever writes the code. Exit 1 while there are failures.</P>
      <Code lang="json" title="stdout" code={OUT.feedback} />

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
├── config.json          the model and the approval policy (init)
├── intents.json         the intent artifact: intents, approvals, decisions (plan, review, spec)
├── spec.json            the approved artifact + the proof plan + contradictions (spec)
├── proof.json           the verification report, Rust report, failures, spec gaps (prove)
├── receipt.md           the receipt for the pull request (prove)
└── llm/
    ├── extract-6e77fe54fe3e.request.json   what the AI step needs (handoff)
    ├── extract-6e77fe54fe3e.answer.json    the answer the calling agent wrote
    ├── derive-dca5b7ce966e.request.json
    └── derive-dca5b7ce966e.answer.json`}
      />
      <DataTable
        head={["File", "Written by", "Contents"]}
        rows={[
          [<C>config.json</C>, <C>init</C>, <><C>{`{ model, requiredApprovals }`}</C></>],
          [<C>intents.json</C>, <><C>plan</C>, <C>review</C>, <C>spec</C></>, <>an <A to="reference/data-model">IntentArtifact</A> (not approved)</>],
          [<C>spec.json</C>, <C>spec</C>, <><C>{`{ artifact, plan, contradictions, ok }`}</C>, with the artifact approved under its hash</>],
          [<C>proof.json</C>, <C>prove</C>, <><C>{`{ ok, report, rust?, enginesAgree?, failures, specGaps, receipt }`}</C></>],
          [<C>receipt.md</C>, <C>prove</C>, "the Markdown receipt"],
          [<C>llm/*.json</C>, <>handoff, and you</>, "requests and answers for the AI steps"],
        ]}
      />

      <H2>What to commit</H2>
      <P>
        Committing <C>intents.json</C> makes the review itself reviewable in a pull request. Committing the <C>llm/*.answer.json</C> files makes the AI steps
        replayable: a CI job can re-run <C>plan</C>, <C>spec</C> and <C>prove</C> with <C>--llm handoff</C> and no model, and get the same hashes, because a request whose answer file
        exists is answered from the file.
      </P>

      <H2>Several decision cores</H2>
      <P>
        Use one workspace per decision core with <C>--dir</C>:
      </P>
      <Code
        lang="bash"
        code={`$ lastro init --dir .lastro/login --model login/model.json
$ lastro plan --dir .lastro/login login/prose.md
$ lastro prove --dir .lastro/shipping ./shipping.js`}
      />

      <H2>One workspace, three CLIs</H2>
      <P>
        The TypeScript, Go and .NET CLIs share the format, the hashes and even the handoff request ids. One person can plan with <C>lastro</C>, another review and
        prove a Go service with <C>lastro-go</C>:
      </P>
      <Code
        lang="bash"
        code={`$ lastro plan prose.md && lastro review --as ana approve-all && lastro spec
$ lastro-go prove --cmd ./shipping     # continues the same .lastro/`}
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
      <Code lang="bash" code={`$ lastro plan prose.md`} />
      <Code lang="json" title="stdout · exit 3" code={OUT.planNeedsLlm} />
      <P>The request file has everything a model needs:</P>
      <DataTable
        head={["Field", "Contents"]}
        rows={[
          [<C>instructions</C>, "What to do: act as the system, read the user message, write only a JSON value matching the schema to the answer path, then run the same command again."],
          [<C>step</C>, <><C>extract</C> or <C>derive</C></>],
          [<C>system</C>, "The step's system prompt, identical in every SDK."],
          [<C>user</C>, "The data: the prose, or the model and the approved intents."],
          [<C>schema</C>, "The JSON Schema the answer must match."],
        ]}
      />
      <P>
        The id is the step plus the first 12 hex digits of a SHA-256 over the request, so the same question always gets the same file name, in every CLI. When the
        answer file exists, the CLI reads it instead of asking. Answers are never trusted blindly: quotes are anchored in the prose, and LSL is validated against
        the model.
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

run plan prose.md
# … people review …
run spec
run prove ./decide.js || lastro feedback`}
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
\`answer\` path. Then run the same command again. Never edit .lastro/intents.json by hand:
use \`lastro review\`.`}
      />
      <Callout kind="warning" title="The agent answers, people review">
        The handoff lets an agent do the AI steps, not the human one. Approvals still come from <C>lastro review --as &lt;name&gt;</C>, and your policy decides how
        many are needed.
      </Callout>
    </>
  );
}
