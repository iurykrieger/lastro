import { ArrowRight, CheckCircle2, FileText, ListChecks, ShieldCheck, Sigma } from "lucide-react";
import { A, C, Callout, Code, CodeTabs, DataTable, H1, H2, Lead, Ol, P, Step, Steps, Ul } from "@/components/doc";
import { Badge } from "@/components/ui/badge";
import { Card, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { href } from "@/lib/router";
import { Pipeline } from "@/components/Pipeline";
import prose from "../../examples/shipping/prose.md?raw";
import model from "../../examples/shipping/model.json?raw";
import buggy from "../../examples/shipping/shipping-buggy.mjs?raw";
import extractAnswer from "../../examples/shipping/answers/extract.json?raw";
import { RECEIPT, OUT } from "@/content/outputs";

// ---------------------------------------------------------------------------
// Introduction
// ---------------------------------------------------------------------------

const CARDS = [
  { icon: ListChecks, title: "Quickstart", body: "Prove a real function from a paragraph of prose with the CLI.", to: "quickstart" },
  { icon: Sigma, title: "Concepts", body: "Intents, review policy, decisions, the formal spec and what each verdict means.", to: "concepts/pipeline" },
  { icon: FileText, title: "CLI", body: "Seven commands, JSON out, state in .lastro/, usable from any agent harness.", to: "cli/overview" },
  { icon: ShieldCheck, title: "SDKs", body: "TypeScript, Go and .NET: one data model, the same hashes and verdicts.", to: "sdks/typescript" },
];

export function Introduction() {
  return (
    <>
      <H1>Lastro</H1>
      <Lead>
        Lastro gives a plan written in prose a deterministic backing: intents a person reviewed, proof obligations derived from them, and a reproducible check of
        those obligations against what the code actually does.
      </Lead>
      <div className="mt-6 flex flex-wrap gap-1.5">
        <Badge variant="proved">Proved</Badge>
        <Badge variant="trusted">Proved under assumptions</Badge>
        <Badge variant="evidence">Evidenced</Badge>
        <Badge variant="fail">Violated</Badge>
        <Badge variant="beyond">Beyond proof</Badge>
      </div>
      <P>
        <em>Lastro</em> is Portuguese for "backing", as in the reserves behind a currency. It follows GitHub Next's{" "}
        <A to="https://githubnext.com/projects/pdd-kit/">PDD Kit</A> (Proof Driven Development): review moves up from code to intent, and every result says plainly
        whether it is proved, proved under trusted assumptions, evidenced to a stated confidence, or beyond proof.
      </P>

      <Pipeline />

      <div className="mt-8 grid gap-3 sm:grid-cols-2">
        {CARDS.map((c) => (
          <a key={c.to} href={href(c.to)} className="group">
            <Card className="h-full transition-colors group-hover:border-brass/60">
              <CardHeader className="p-5">
                <c.icon className="mb-2 h-5 w-5 text-brass" />
                <CardTitle className="flex items-center gap-1 text-base">
                  {c.title} <ArrowRight className="h-3.5 w-3.5 opacity-0 transition-opacity group-hover:opacity-100" />
                </CardTitle>
                <CardDescription>{c.body}</CardDescription>
              </CardHeader>
            </Card>
          </a>
        ))}
      </div>
      <Callout kind="tip" title="Try it without installing anything">
        The <A to="examples/overview">examples</A> run the real SDK and the Rust prover in your browser: fix a bug until the proof passes, plant one in a login,
        or review the intents yourself and watch which code passes.
      </Callout>

      <H2>What Lastro does</H2>
      <Ol>
        <li>
          <strong>Plan.</strong> An LLM reads your prose and proposes <A to="concepts/intents">intents</A>: one checkable promise each, anchored to the words it came
          from. Ambiguities come back as <A to="concepts/decisions">decision cards</A>, never as silent guesses.
        </li>
        <li>
          <strong>Review.</strong> People approve, amend or reject each intent. Your team's <A to="concepts/review">policy</A> sets how many approvals an intent needs.
          The reviewed intents are approved under a SHA-256 hash.
        </li>
        <li>
          <strong>Spec.</strong> An LLM derives a <A to="concepts/spec">formal spec</A> from the approved intents, in LSL (a JSON AST that is never evaluated as code).
          Lastro validates it and checks it for contradictions, which go back to the team as decisions.
        </li>
        <li>
          <strong>Prove.</strong> Lastro checks the spec against your code, deterministically, in two independent engines (TypeScript and Rust), and writes a{" "}
          <A to="concepts/receipts">receipt</A>. Failures come with counterexamples, as a brief for whoever writes the code.
        </li>
      </Ol>

      <H2>What Lastro is not</H2>
      <Ul>
        <li>
          <strong>Not a code generator.</strong> Lastro's only AI steps are extracting intents and deriving proofs. Any coding agent (Claude Code, Codex, …) or a person
          writes the implementation, and Lastro judges it.
        </li>
        <li>
          <strong>Not tied to a harness or a CI.</strong> It is a library (<C>createLastro</C>) and a CLI that prints JSON. Use it in a script, a CI job, an app or any
          agent plugin. The CLI can even hand its AI steps to the calling agent's own model.
        </li>
        <li>
          <strong>Not a test runner that calls samples "proof".</strong> "Proved" means checked for every input of a finite declared domain. Anything sampled is
          "evidenced", with the confidence it actually earned.
        </li>
      </Ul>

      <H2>Where to go next</H2>
      <P>
        New here? Do the <A to="quickstart">quickstart</A>: about ten minutes, no API key needed. Then read <A to="concepts/verdicts">Proved, trusted, evidenced</A>{" "}
        before you trust a green check.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Installation
// ---------------------------------------------------------------------------

export function Installation() {
  return (
    <>
      <H1>Installation</H1>
      <Lead>Lastro is at v0 and not on a package registry yet. Build what you need from the repository.</Lead>

      <H2>Prerequisites</H2>
      <DataTable
        head={["You want", "You need"]}
        rows={[
          ["The TypeScript SDK and the lastro CLI", "Node 20+ (CI uses 22)"],
          ["The Rust prover (recommended: the second engine)", <>Rust stable with the <C>wasm32-unknown-unknown</C> target</>],
          ["The Go SDK and lastro-go", "Go 1.25"],
          [".NET SDK and the lastro .NET tool", ".NET 8 SDK"],
        ]}
      />

      <H2>Clone and build</H2>
      <Code
        lang="bash"
        code={`$ git clone https://github.com/iurykrieger/lastro.git
$ cd lastro
$ rustup target add wasm32-unknown-unknown
$ npm install
$ npm test     # Rust tests, the WASM prover, then the SDK tests (with the Rust cross-check)`}
      />
      <P>
        <C>npm test</C> builds <C>sdks/typescript/wasm/lastro_core.wasm</C>. The TypeScript CLI loads it when present and runs every proof in both engines.
      </P>

      <H2>Per SDK</H2>
      <CodeTabs
        variants={[
          {
            label: "TypeScript",
            lang: "bash",
            title: "TypeScript SDK + lastro CLI",
            code: `$ npm run build -w lastro          # compiles to sdks/typescript/dist
$ npm link -w lastro               # puts \`lastro\` on your PATH
$ lastro                           # prints usage

# in your project
$ npm install /path/to/lastro/sdks/typescript
$ npm install @anthropic-ai/sdk    # optional: only for the Claude transport`,
          },
          {
            label: "Go",
            lang: "bash",
            title: "Go SDK + lastro-go",
            code: `$ go get github.com/iurykrieger/lastro/sdks/go
$ go install github.com/iurykrieger/lastro/sdks/go/cmd/lastro-go@latest

# the Rust prover binary, picked up from PATH by FindRustProver()
$ cargo build --release -p lastro-core --bin lastro-prove`,
          },
          {
            label: ".NET",
            lang: "bash",
            title: ".NET SDK + lastro tool",
            code: `$ dotnet pack sdks/dotnet -o ./nupkg
$ dotnet tool install --global --add-source ./nupkg Lastro.Cli   # the \`lastro\` command
$ dotnet add package Lastro --source ./nupkg
$ dotnet add package Lastro.Claude --source ./nupkg              # optional: the Claude transport

# the Rust prover binary, found by RustProver.Locate()
$ cargo build --release -p lastro-core --bin lastro-prove`,
          },
        ]}
      />

      <Callout kind="note" title="One workspace, three CLIs">
        The TypeScript <C>lastro</C>, the Go <C>lastro-go</C> and the .NET <C>lastro</C> read and write the same <C>.lastro/</C> files. Start with one and continue
        with another; see <A to="sdks/interop">Interoperability</A>.
      </Callout>

      <H2>The AI steps</H2>
      <P>
        Planning and deriving the spec need an LLM. You have two options, and neither needs a plugin for a specific agent:
      </P>
      <Ul>
        <li>
          Set <C>ANTHROPIC_API_KEY</C> and the SDKs call Claude through the Anthropic SDK for your language.
        </li>
        <li>
          Use the CLI's <A to="cli/llm">LLM handoff</A>: it writes each request to a file and the calling agent answers with its own model.
        </li>
      </Ul>
      <P>
        Everything after the spec (proving, receipts, feedback) is deterministic and never calls a model.
      </P>

      <H2>Try the sandbox</H2>
      <P>
        <C>npm run sandbox</C> opens an interactive sandbox on <C>http://localhost:5173</C>: a store login with its prose, intents, code and live proofs in both
        engines. Plant a bug and watch the clause that catches it fail.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Quickstart
// ---------------------------------------------------------------------------

export function Quickstart() {
  return (
    <>
      <H1>Quickstart</H1>
      <Lead>
        Take a shipping rule written in prose, turn it into reviewed intents, derive a formal spec, and prove a function against it. Everything below is real
        output of the TypeScript CLI.
      </Lead>
      <Callout kind="tip" title="No API key needed">
        This walkthrough uses <C>--llm handoff</C>: the CLI writes each AI request to a file and you (or your agent) answer it. The answers are in{" "}
        <C>apps/docs/examples/shipping/answers</C>, and a test runs this exact walkthrough in CI so the outputs here stay true.
      </Callout>

      <Steps>
        <Step title="Write the prose and the model">
          <P>The prose is the rule as the team wrote it:</P>
          <Code lang="markdown" title="prose.md" code={prose} />
          <P>
            The model is the vocabulary of the proofs: the inputs the code decides on and the outputs it returns, each with a domain. Here the input space has 201
            × 2 = 402 points, small enough to check every one.
          </P>
          <Code lang="json" title="model.json" code={model} />
        </Step>

        <Step title="Start a workspace">
          <Code lang="bash" code={`$ lastro init --model model.json`} />
          <Code lang="json" title="stdout · exit 0" code={OUT.init} />
          <P>
            <C>--approvals 2</C> would require two distinct reviewers per intent. The default policy is one.
          </P>
        </Step>

        <Step title="Extract intents">
          <Code lang="bash" code={`$ lastro plan prose.md`} />
          <P>
            Without <C>ANTHROPIC_API_KEY</C>, the CLI hands the step to you and exits with code 3:
          </P>
          <Code lang="json" title="stdout · exit 3" code={OUT.planNeedsLlm} />
          <P>
            The request file holds the system prompt, the prose and a JSON Schema. An agent answers it with its own model and writes the answer to the path given. Ours
            proposes four intents and one question the prose leaves open:
          </P>
          <Code lang="json" title=".lastro/llm/extract-6e77fe54fe3e.answer.json" code={extractAnswer} />
          <P>Run the same command again:</P>
          <Code lang="bash" code={`$ lastro plan prose.md`} />
          <Code lang="json" title="stdout · exit 1 (review pending)" code={OUT.planned} />
        </Step>

        <Step title="Review">
          <P>Answer the decision and approve the intents. Each command signs with the reviewer's name.</P>
          <Code
            lang="bash"
            code={`$ lastro review --as ana answer D1 1      # "After discounts"
$ lastro review --as ana approve-all`}
          />
          <Code lang="json" title="stdout · exit 0" code={OUT.reviewed} />
        </Step>

        <Step title="Derive the spec">
          <Code lang="bash" code={`$ lastro spec`} />
          <P>
            This is the second AI step, so it exits 3 with a <C>derive</C> request. Once answered, the CLI validates the plan (only approved intents, only model
            variables, every intent covered) and checks the clauses for contradictions:
          </P>
          <Code lang="json" title="stdout · exit 0" code={OUT.spec} />
        </Step>

        <Step title="Prove the code">
          <P>Someone (a coding agent, or you) wrote this implementation. It has an off-by-one:</P>
          <Code lang="js" title="shipping.mjs" code={buggy} />
          <Code lang="bash" code={`$ lastro prove ./shipping.mjs`} />
          <Code lang="json" title="stdout · exit 1" code={OUT.notProved} />
          <P>
            Lastro checked all 402 inputs in both engines and found the one that breaks C1. <C>lastro feedback</C> prints the same failures as a brief you can paste
            into any coding agent.
          </P>
        </Step>

        <Step title="Fix it and prove again">
          <Code lang="bash" code={`$ sed -i 's/subtotal > 100/subtotal >= 100/' shipping.mjs
$ lastro prove ./shipping.mjs`} />
          <Code lang="json" title="stdout · exit 0" code={OUT.proved} />
          <P>
            The receipt in <C>.lastro/receipt.md</C> is ready for the pull request:
          </P>
          <Code lang="markdown" title=".lastro/receipt.md" code={RECEIPT} wrap={false} />
        </Step>
      </Steps>

      <H2>What just happened</H2>
      <Ul>
        <li>
          <CheckCircle2 className="mr-1 inline h-4 w-4 text-proved" />
          I1 to I3 are <strong>proved</strong>: every one of the 402 inputs was checked, and the Rust prover agreed verdict by verdict.
        </li>
        <li>
          I4 ("instantly") is <strong>beyond proof</strong>: timing is not a property of the decision. The receipt says so instead of hiding it.
        </li>
        <li>
          The decision about discounts did not become code. It became a fact the team chose, recorded in the approved intents.
        </li>
      </Ul>
      <P>
        Next: the same flow <A to="sdks/typescript">as a library</A>, or <A to="cli/llm">wire the handoff into your agent</A>.
      </P>
    </>
  );
}
