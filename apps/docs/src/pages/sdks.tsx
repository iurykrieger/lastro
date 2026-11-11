import { A, C, Callout, Code, DataTable, H1, H2, H3, Lead, P, Ul } from "@/components/doc";
import libraryTs from "../../examples/shipping/library.ts?raw";
import goLibrary from "../../examples/shipping/go-library/main.go?raw";
import goLines from "../../examples/shipping/go/main.go?raw";
import dotnetProgram from "../../examples/shipping/dotnet/Program.cs?raw";

const SAME_RESULT = (
  <P>
    Run against the <A to="quickstart">quickstart</A>'s answers, it produces the same intent hash (<C>sha256:c23c5326…</C>), plan hash (
    <C>sha256:e68a4212…</C>) and receipt as the CLI and the other SDKs.
  </P>
);

// ---------------------------------------------------------------------------
// TypeScript
// ---------------------------------------------------------------------------

const ASK = `import type { AskJson } from "lastro";

// Any model, any provider: return one JSON value matching request.schema.
const ask: AskJson = async ({ step, system, user, schema }) => {
  const reply = await myModel.complete({ system, user, jsonSchema: schema });
  return JSON.parse(reply.text);
};`;

const OFFLINE = `import { createArtifact, review, approve, validatePlan, verify, renderReceipt, loadRustProver, tabulate } from "lastro";

const artifact = approve(
  review(createArtifact(prose, drafts, decisions, notPromised), "ana").approveAll().answer("D1", 0).done(),
);
const plan = myPlan(artifact);                    // a ProofPlan you wrote, or StaticDeriver
const uncovered = validatePlan(artifact, plan);   // throws PlanError on invalid plans; returns uncovered intents
const report = await verify(artifact, plan, { subject: decide });
const rust = (await loadRustProver(wasmBytes)).prove(plan, await tabulate(plan.model, decide));
console.log(renderReceipt(report, plan, rust));`;

export function SdkTypeScript() {
  return (
    <>
      <H1>TypeScript SDK</H1>
      <Lead>
        The reference implementation: the <C>lastro</C> package (<C>sdks/typescript</C>) with <C>createLastro</C>, the building blocks underneath, the{" "}
        <C>lastro</C> CLI and a Claude transport at <C>lastro/claude</C>.
      </Lead>
      <Code lang="bash" code={`$ npm install /path/to/lastro/sdks/typescript\n$ npm install @anthropic-ai/sdk   # only for lastro/claude`} />

      <H2>The whole pipeline</H2>
      <Code lang="ts" title="apps/docs/examples/shipping/library.ts" code={libraryTs} />
      {SAME_RESULT}

      <H2>createLastro options</H2>
      <DataTable
        head={["Option", "Type", "Meaning"]}
        rows={[
          [<C>model</C>, <C>Model</C>, <>The vocabulary of the proofs (required). See <A to="concepts/spec">Model and formal spec</A>.</>],
          [<C>ask</C>, <C>AskJson</C>, "One LLM transport for both AI steps."],
          [<C>extractor</C>, <C>IntentExtractor</C>, <>Replaces the extraction step (for example <C>StaticExtractor</C> in tests).</>],
          [<C>deriver</C>, <C>ProofDeriver</C>, <>Replaces the derivation step (<C>StaticDeriver</C>).</>],
          [<C>requiredApprovals</C>, <C>number</C>, "Distinct reviewers per intent. Default 1."],
          [<C>rust</C>, <C>RustProver</C>, <>The independent prover from <C>loadRustProver(wasm)</C>. Recommended in CI.</>],
          [<C>verify</C>, <C>VerifyOptions</C>, <><C>exhaustiveLimit</C> (100,000), <C>samples</C> (2,000), <C>seed</C> (42), <C>mutation</C>, <C>consistencyLimit</C> (4,096).</>],
        ]}
      />

      <H2>Methods</H2>
      <DataTable
        head={["Method", "Returns", "AI?"]}
        rows={[
          [<C>plan(prose, previous?)</C>, <C>Promise&lt;IntentArtifact&gt;</C>, "yes"],
          [<C>review(artifact, reviewer)</C>, <><C>Review</C>: chain actions, then <C>done()</C></>, "no"],
          [<C>pending(artifact)</C>, <C>string[]</C>, "no"],
          [<C>spec(artifact)</C>, <C>Promise&lt;Spec&gt;</C>, "yes"],
          [<C>returnDecisions(spec)</C>, <C>IntentArtifact</C>, "no"],
          [<C>prove(spec, subject, {"{ traces? }"})</C>, <C>Promise&lt;Proof&gt;</C>, "no"],
          [<C>gapReview(spec, source)</C>, <><C>Promise&lt;Gap[]&gt;</C>: what the proofs cannot see. Evidence for humans, never proof.</>, "yes"],
        ]}
      />

      <H2>The subject</H2>
      <P>
        The code under proof is a function from the model's inputs to its outputs, sync or async: <C>{"(input: Env) => Env | Promise<Env>"}</C>. It should be the
        decision core of your feature, with effects behind the inputs (did the database find the row?) and the outputs (which status, which role).
      </P>

      <H2>LLM transports</H2>
      <H3>Claude</H3>
      <Code
        lang="ts"
        code={`import { claudeAsk } from "lastro/claude";

const ask = claudeAsk();                                   // ANTHROPIC_API_KEY, default model claude-opus-5-5
const tuned = claudeAsk({ model: "claude-opus-5-5", effort: "high", client: new Anthropic({ maxRetries: 4 }) });`}
      />
      <H3>Your own</H3>
      <P>
        <C>AskJson</C> is the whole contract: a request with <C>step</C>, <C>system</C>, <C>user</C> and <C>schema</C>, and one JSON value back. The CLI's handoff is
        just another <C>AskJson</C>.
      </P>
      <Code lang="ts" code={ASK} />

      <H2>Building blocks</H2>
      <P>
        Everything <C>createLastro</C> does is exported, for tools that need a single step: artifacts (<C>createArtifact</C>, <C>review</C>, <C>approve</C>,{" "}
        <C>reopen</C>, <C>isApproved</C>, <C>carryReview</C>, <C>raiseDecisions</C>, <C>hashOf</C>), plans (<C>validatePlan</C>, <C>checkPlan</C>), the engine (
        <C>verify</C>, <C>explainRun</C>, <C>checkConsistency</C>), LSL (<C>L</C>, <C>evaluate</C>, <C>show</C>, <C>toSmtLib</C>), domains (<C>D</C>,{" "}
        <C>enumerateSpace</C>), the prompts (<C>EXTRACT_SYSTEM</C>, <C>DERIVE_SCHEMA</C>, …), <C>renderReceipt</C> and the Rust bridge (<C>loadRustProver</C>,{" "}
        <C>tabulate</C>).
      </P>
      <Code lang="ts" title="the pipeline without an LLM" code={OFFLINE} />
      <P>
        <C>sdks/typescript/examples/login</C> does exactly this for a store login with six planted bugs: <C>npm run example -w lastro -- refuse-everyone</C>.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

export function SdkGo() {
  return (
    <>
      <H1>Go SDK</H1>
      <Lead>
        A port of the TypeScript engine (<C>sdks/go</C>) with the same data model, hashes and <C>.lastro/</C> workspace, plus the <C>lastro-go</C> CLI. Its subjects
        can be programs in any language.
      </Lead>
      <Code lang="bash" code={`$ go get github.com/iurykrieger/lastro/sdks/go`} />

      <H2>The whole pipeline</H2>
      <Code lang="go" title="apps/docs/examples/shipping/go-library/main.go" code={goLibrary} />
      {SAME_RESULT}
      <Callout kind="warning" title="Set Rust only when the prover was found">
        <C>FindRustProver()</C> returns a nil <C>*ExecProver</C> when <C>lastro-prove</C> is absent. Assigning it straight to <C>Options.Rust</C> makes a non-nil
        interface holding a nil pointer, so check it first as above.
      </Callout>

      <H2>Options</H2>
      <DataTable
        head={["Field", "Meaning"]}
        rows={[
          [<C>Model</C>, "The vocabulary of the proofs: Fields keep their declared order."],
          [<C>Ask</C>, <><C>lastro.AskJSON</C>: <C>claude.Ask(claude.Options{"{}"})</C> or your own.</>],
          [<><C>Extractor</C>, <C>Deriver</C></>, <>Replace a single AI step (<C>ExtractorFunc</C>, <C>DeriverFunc</C>).</>],
          [<C>RequiredApprovals</C>, "Distinct reviewers per intent. Default 1."],
          [<C>Rust</C>, <>A <C>RustProver</C>, usually from <C>FindRustProver()</C>.</>],
          [<C>Verify</C>, "Sampling and limits, as in the TypeScript SDK."],
        ]}
      />
      <P>
        Methods mirror the TypeScript SDK and take a <C>context.Context</C> where an LLM or the prover runs: <C>Plan</C>, <C>Review</C>, <C>Pending</C>,{" "}
        <C>Spec</C>, <C>ReturnDecisions</C>, <C>Prove</C>, <C>GapReview</C>. <C>Review(…).Done()</C> returns the artifact and the first error of the chain.
      </P>

      <H2>Subjects in any language</H2>
      <P>
        A subject is any <C>func(lastro.Env) (lastro.Env, error)</C>. Values arrive as JSON decodes them: numbers are <C>float64</C>. Two more ways to get one:
      </P>
      <Ul>
        <li>
          <C>lastro.ProcessSubject(ctx, name, args...)</C> starts a program once and speaks JSON lines with it: one input object per line on stdin, one output
          object per line on stdout. Python, Rust, a shell script: anything that can read a line.
        </li>
        <li>
          <C>lastro.TableSubject(rows)</C> answers from a recorded input/output table, for example one produced by <C>Tabulate</C> or by another tool.
        </li>
      </Ul>
      <Code lang="go" title="apps/docs/examples/shipping/go/main.go (a JSON-lines program)" code={goLines} />
      <Code
        lang="bash"
        code={`$ go build -o shipping ./go
$ lastro-go prove --cmd ./shipping     # continues the workspace the TypeScript CLI started`}
      />

      <H2>The lastro-go CLI</H2>
      <P>
        Same commands, JSON output, exit codes and workspace as the TypeScript CLI; only <C>prove</C> differs (<C>--cmd</C>, <C>--table</C>, <C>--prover</C>). See{" "}
        <A to="cli/overview">Commands</A>.
      </P>
      <Code lang="bash" code={`$ go install github.com/iurykrieger/lastro/sdks/go/cmd/lastro-go@latest`} />
      <P>
        After changing a prompt or schema in the TypeScript SDK, regenerate the Go copy with <C>go generate ./...</C> in <C>sdks/go</C>.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// .NET
// ---------------------------------------------------------------------------

export function SdkDotnet() {
  return (
    <>
      <H1>.NET SDK</H1>
      <Lead>
        A port of the TypeScript SDK (<C>sdks/dotnet</C>), not a wrapper: <C>Lastro</C> (no dependencies beyond .NET 8), <C>Lastro.Claude</C> (over the official
        Anthropic C# SDK) and <C>Lastro.Cli</C>, the <C>lastro</C> .NET tool.
      </Lead>
      <Code lang="bash" code={`$ dotnet pack sdks/dotnet -o ./nupkg\n$ dotnet add package Lastro --source ./nupkg\n$ dotnet add package Lastro.Claude --source ./nupkg`} />

      <H2>The whole pipeline</H2>
      <Code lang="csharp" title="apps/docs/examples/shipping/dotnet/Program.cs" code={dotnetProgram} />
      {SAME_RESULT}

      <H2>LastroClient</H2>
      <DataTable
        head={["Member", "Meaning"]}
        rows={[
          [<C>LastroOptions</C>, <><C>Model</C> (required), <C>Ask</C>, <C>Extractor</C>, <C>Deriver</C>, <C>RequiredApprovals</C> (default 1), <C>Rust</C>, <C>Verify</C></>],
          [<C>PlanAsync(prose, previous?)</C>, "Extract intents; carries a previous review over."],
          [<><C>Review(a, reviewer)</C> … <C>Done()</C></>, <><C>Approve</C>, <C>ApproveAll</C>, <C>Amend</C>, <C>Reject</C>, <C>Answer</C>, <C>Add</C>, <C>NotPromised</C></>],
          [<C>Pending(a)</C>, "What still blocks the spec."],
          [<C>SpecAsync(a)</C>, "Approve, derive, validate, check consistency."],
          [<C>ReturnDecisions(spec)</C>, "Contradictions back to the team."],
          [<C>ProveAsync(spec, subject, traces?)</C>, <>A <C>Subject</C> or a <C>{"Func<Env, Env>"}</C>.</>],
          [<C>GapReviewAsync(spec, source)</C>, "What the proofs cannot see. Evidence, never proof."],
        ]}
      />

      <H2>Values</H2>
      <P>
        <C>Env</C> is a dictionary of <C>Value</C>. Read with <C>AsBool</C>, <C>AsLong</C>, <C>AsInt</C>, <C>AsNumber</C> and <C>AsString</C>; write with plain
        literals, which convert implicitly: <C>{`new Env { ["fee"] = 0 }`}</C>. <C>Env.From</C> and <C>ToObjects()</C> convert from and to CLR dictionaries.
      </P>

      <H2>The lastro .NET tool</H2>
      <P>
        Same commands and workspace as the other CLIs. <C>prove</C> loads a public static method from an assembly: it takes one argument (<C>Lastro.Env</C> or a
        dictionary of string to object, where integers arrive as <C>long</C>) and returns the same kinds, optionally as a <C>Task</C>. The assembly does not need
        to reference Lastro. <C>init --subject Shop.dll#Shop.Shipping.Fee</C>
        records the subject so <C>prove</C> and <C>regress</C> need no arguments.
      </P>
      <Code
        lang="bash"
        code={`$ dotnet tool install --global --add-source ./nupkg Lastro.Cli
$ lastro prove P-001 ./bin/Release/net8.0/Shop.dll --export Shop.Shipping.Fee`}
      />
      <P>
        Proposals, intents, proofs and use cases are in <C>Proposals</C> (<C>FromExtraction</C>, <C>Review</C>, <C>Approve</C>, <C>DeriveAsync</C>,{" "}
        <C>ProveAsync</C>, <C>Apply</C>, <C>RegressAsync</C>, <C>RenderReceipt</C>). The building blocks are public too: <C>Artifacts</C>, <C>Plans</C>,{" "}
        <C>Engine</C>, <C>Lsl</C> and <C>L</C>, <C>D</C> and <C>Domains</C>, <C>Receipt</C>, <C>LlmPrompts</C> and <C>RustProver</C>.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Interop
// ---------------------------------------------------------------------------

export function SdkInterop() {
  return (
    <>
      <H1>Interoperability</H1>
      <Lead>Three SDKs, one format. Pick the language of your service; the review, the hashes and the verdicts do not change.</Lead>
      <DataTable
        head={["What is shared", "How it is kept equal"]}
        rows={[
          ["Wire format: camelCase JSON, kebab-case enums", "Each SDK reads the others' .lastro/ files in its tests."],
          ["Canonical JSON and SHA-256 hashes", "An artifact approved by one SDK is intact for the others; plans hash the same."],
          ["Verdicts, receipts, seeded sampling", "Cross-check tests run the TypeScript engine and compare reports field by field."],
          ["Prompts and JSON Schemas", "Generated from the TypeScript SDK, so handoff request ids match across CLIs."],
          ["The Rust prover", "Each SDK runs lastro-core (WASM in TypeScript, the native binary in Go and .NET) and requires agreement."],
        ]}
      />

      <H2>The cross-checks</H2>
      <DataTable
        head={["Test", "Checks"]}
        rows={[
          [<C>sdks/typescript/test/lastro.test.ts</C>, "the TypeScript engine and the Rust prover agree verdict by verdict"],
          [<C>sdks/go/crosscheck_test.go</C>, "identical reports and hashes with the TypeScript SDK; the Rust prover through lastro-prove"],
          [<C>sdks/go/cmd/lastro-go/main_test.go</C>, "lastro-go and lastro share a workspace"],
          [<C>sdks/dotnet/tests/Lastro.Tests/InteropTests.cs</C>, "hashes, verdicts, receipts, prompts and handoff ids match the TypeScript SDK"],
          [<C>apps/docs/test/walkthrough.test.mjs</C>, "the quickstart in these docs runs as shown"],
        ]}
      />

      <H2>Running them</H2>
      <Code
        lang="bash"
        code={`$ cargo test -p lastro-core && npm run wasm && npm test -w lastro
$ cargo build --release -p lastro-core && (cd sdks/go && go test ./...)
$ npx tsc -p sdks/typescript && dotnet test sdks/dotnet`}
      />
      <Callout kind="note" title="Changing semantics">
        A change to the engine, a prompt, a schema or the CLI workspace in one SDK must land in all of them in the same pull request; the cross-checks fail
        otherwise. In CI, <C>LASTRO_REQUIRE_CROSSCHECK=1</C> turns a skipped .NET cross-check into a failure.
      </Callout>
    </>
  );
}
