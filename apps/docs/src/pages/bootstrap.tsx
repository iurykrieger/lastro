import { A, C, Callout, Code, DataTable, H1, H2, Lead, P, Step, Steps, Ul } from "@/components/doc";
import answer from "../../examples/shipping/answers/bootstrap.json?raw";

// ---------------------------------------------------------------------------
// Existing repositories: lastro scan and lastro bootstrap
// ---------------------------------------------------------------------------

const REPORT = `# Bootstrap the checkout

The use cases the checkout implements today, read from its code.

Read from commit c0d1f9a7534d: 3 files, 0 tests, 1 of 1 commits.

## Charge shipping at checkout (charge-shipping, add, medium confidence)

An order with a subtotal of 100 or more pays no shipping, and a member never pays shipping, whatever the subtotal. Every other order pays a flat shipping fee of exactly 15.

- An order with a subtotal of 100 or more pays no shipping.
  Grounds: files \`shipping.mjs\`.
- A member never pays shipping, whatever the subtotal.
  Grounds: files \`shipping.mjs\`.
- A non-member order under 100 pays a shipping fee of exactly 15.
  Grounds: files \`shipping.mjs\`.

## Not covered

- \`shipping-buggy.mjs\`: a second copy of the fee with an off-by-one; ask which one ships`;

export function Bootstrap() {
  return (
    <>
      <H1>Existing repositories</H1>
      <Lead>
        A repository with thousands of commits already has behavior. Before the first proposal, Lastro needs the use cases the code implements today, each with
        proofs that pass against that code. <C>lastro bootstrap</C> reads the code, the tests and the commit history into a proposal you review like any other.
      </Lead>

      <P>
        Proposals change use cases, so they need use cases to read against. In a new repository the first proposal adds them. In an existing one, the behavior is
        already there, written in code, tests and commit messages, and nobody wants to retype it as prose. A bootstrap writes it down for you, as a{" "}
        <strong>bootstrap proposal</strong>: one intent per behavior the code shows, each grounded in the files, tests and commits it was read from. Nothing in it
        is trusted until you approved it, its proofs passed against today's code, and it was applied.
      </P>

      <H2>The flow</H2>
      <Steps>
        <Step title="Size the repository">
          <P>
            <C>lastro scan</C> reads the repository without any LLM: every tracked file with its role (source, test, doc, config) and its exported names, every
            test by name, the manifests (<C>package.json</C>, <C>go.mod</C>, <C>Cargo.toml</C>, <C>*.csproj</C>, <C>pyproject.toml</C>), the newest commits
            with the files they touched, and the areas a large repository can be split into.
          </P>
          <Code lang="bash" code={`$ lastro scan --max-commits 400`} />
        </Step>
        <Step title="Survey the code">
          <P>
            <C>lastro bootstrap</C> scans again and hands the digest, with source excerpts, to the <C>survey</C> step: the extract step reading code instead of
            prose. With <C>--llm handoff</C> the calling agent answers it with its own model and may open any file of the digest; with an API key, Claude
            answers it. It creates the workspace when there is none.
          </P>
          <Code lang="bash" code={`$ lastro bootstrap --as ana
# exit 3: needs-llm, step "survey": answer the request, then run it again
$ lastro bootstrap --as ana`} />
          <P>
            The answer is checked before anything is kept: every file, test and commit it cites must be in the digest, every behavior needs a file or a test, a
            use case that already has proofs cannot be touched, and a proposed model must be well formed. The CLI lists every problem at once.
          </P>
        </Step>
        <Step title="Review the reading">
          <P>
            The proposal's prose is a generated report: one section per use case, one line per behavior with its grounds. You review its intents like any{" "}
            <A to="concepts/review">review</A>: answer the decisions (where code, tests and commits disagree), approve what the code really does, amend a misreading,
            reject what you do not want recorded. Each intent carries its <C>grounds</C> and a confidence (high when tests or a commit confirm it).
          </P>
        </Step>
        <Step title="Prove against today's code">
          <P>
            <C>lastro derive</C> and <C>lastro prove</C> work as for any proposal. The proposal may bring the models and subjects its proofs need (its{" "}
            <C>setup</C>, reviewed with it). Here the code is the reference: a failing proof does not ask for a code change. Either the reading is wrong (amend the
            intent and derive again) or the code has a bug (reject the intent, or keep it and fix the code in a later proposal).
          </P>
        </Step>
        <Step title="Apply">
          <P>
            <C>lastro apply</C> writes the use cases to <C>.lastro/usecases/</C> and their proofs to <C>.lastro/proof/</C> as regression tests, and adds the proposal's models and subjects
            to <C>.lastro/config.json</C>. From then on, every proposal is read against them and every change re-runs their proofs.
          </P>
        </Step>
      </Steps>

      <H2>What the survey answers</H2>
      <P>The answer from the shipping example, as an agent would write it for the handoff request:</P>
      <Code lang="json" title="answers/bootstrap.json" code={answer} />
      <P>And the report it becomes, the prose of the bootstrap proposal:</P>
      <Code lang="markdown" title=".lastro/proposals/P-001-bootstrap-checkout.md" code={REPORT} />

      <H2>Large repositories</H2>
      <Ul>
        <li>
          <C>--scope a,b</C> bootstraps only those folders. <C>lastro scan</C> lists the <C>suggestedScopes</C>: the folder of each manifest, otherwise each
          top-level folder. One bootstrap proposal per area keeps each review small enough for one sitting.
        </li>
        <li>
          <C>--max-commits N</C> (default 400) and <C>--since &lt;date&gt;</C> bound the history; <C>--budget &lt;chars&gt;</C> bounds the source excerpts.
          When the digest was cut, the output says so (<C>truncated</C>).
        </li>
        <li>
          Run <C>lastro bootstrap --proposal &lt;P&gt;</C> to survey again after the code moved: intents that read the same keep their review.
        </li>
      </Ul>

      <DataTable
        head={["Field", "On", "What it holds"]}
        rows={[
          [<C>origin</C>, "proposal", <>{`{ kind: "bootstrap", head, digest, scope }`}: the commit and the digest hash the survey read</>],
          [<C>grounds</C>, "intent", <>files, tests (<C>file::name</C>) and full commit hashes, and a confidence: high, medium or low</>],
          [<C>setup</C>, "proposal", <>models and subjects added to <C>.lastro/config.json</C> at apply</>],
        ]}
      />

      <H2>Use cases without proofs</H2>
      <P>
        When the use cases are already written and reviewed (by a bootstrap, or by hand) but some have no proofs yet, derive them straight from the descriptions,
        with no proposal: <C>lastro derive --usecases</C>, then <C>lastro prove --usecases</C>. Each proof is numbered inside its use case (<C>UC-003.PF-01</C>) and
        kept in <C>.lastro/proof/</C>; once it passes, it is a regression test. As in a bootstrap, the code is the reference: a failing proof goes to you, never back
        to the code. See <A to="cli/overview">derive --usecases</A>.
      </P>
      <Code
        lang="bash"
        code={`$ lastro derive --usecases           # every use case with no passed proof; or --usecases UC-003,UC-016
$ lastro prove --usecases            # pending proofs that pass become regression tests`}
      />

      <Callout kind="note" title="Still two AI steps">
        The survey is extraction, from code instead of prose, and derivation is unchanged. Lastro never writes or changes the implementation, during a bootstrap
        least of all. With the agent plugin, the <A to="plugins/agents">bootstrap skill</A> runs this flow and the Stop hook never sends a bootstrap's failing proof
        back to the code.
      </Callout>
    </>
  );
}
