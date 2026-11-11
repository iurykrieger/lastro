import { A, C, Callout, Code, CodeTabs, DataTable, H1, H2, H3, Lead, Ol, P, Ul } from "@/components/doc";
import { Badge } from "@/components/ui/badge";
import { Pipeline } from "@/components/Pipeline";
import { OUT, USECASE } from "@/content/outputs";
import { ClauseAnatomy, SamplingPlayground } from "@/live/shipping";

// ---------------------------------------------------------------------------
// Pipeline
// ---------------------------------------------------------------------------

export function ConceptPipeline() {
  return (
    <>
      <H1>The pipeline</H1>
      <Lead>
        Every change is a <A to="concepts/proposals">proposal</A> that moves through five stages. Two of them use an LLM, one is human, and the rest is fully
        deterministic.
      </Lead>
      <Pipeline />

      <DataTable
        head={["Stage", "Takes", "Returns", "CLI"]}
        rows={[
          ["Propose", "prose and the current use cases", "a proposal: intents that add, change or remove use cases, and decisions", <C>lastro propose prose.md</C>],
          ["Review", "the proposal, a reviewer name", "the proposal, with verdicts; approved under a hash once nothing is pending", <C>lastro review P-001 --as ana …</C>],
          ["Derive", "an approved proposal", "proofs for the approved intents, or decisions when they contradict", <C>lastro derive P-001</C>],
          ["Prove", "the proofs, the use cases' proofs and the code", "a verdict per proof, regressions, a receipt and feedback", <C>lastro prove P-001 ./code.js</C>],
          ["Apply", "a proved proposal", "the use cases with their new descriptions and proofs", <C>lastro apply P-001</C>],
        ]}
      />
      <P>
        Under the CLI sits the engine the library exposes: <C>createLastro</C> runs the same steps (<C>plan</C>, <C>review</C>, <C>spec</C>, <C>prove</C>) over one
        intent artifact in memory, without use cases or a workspace. The rest of these concept pages describe that engine; the proposal layer reuses it for every
        proof it runs.
      </P>

      <H2>Every stage is plain data</H2>
      <P>
        Each stage takes and returns JSON-serializable objects. That is what lets the same pipeline run as a library call, as a CLI command that keeps state in{" "}
        <C>.lastro/</C>, or across SDKs: a proposal approved by the TypeScript CLI is intact for the Go and .NET CLIs, and all three compute the same hashes.
      </P>

      <H2>Only two AI steps</H2>
      <P>
        Lastro uses an LLM to <strong>extract intents</strong> from prose and to <strong>derive the proofs</strong> from approved intents. Neither answer is trusted
        as is:
      </P>
      <Ul>
        <li>every intent's quote is anchored in the prose, so the reviewer sees where it came from;</li>
        <li>every LSL expression is validated against the model (only declared variables and definitions, the right arity for every operator);</li>
        <li>the plan is checked to cite only approved intents and to cover every approved intent with an obligation or a reason it is beyond proof.</li>
      </Ul>
      <P>
        Proving is deterministic: fixed seeds, canonical JSON, and hashes over approved artifacts. Run it twice and you get the same receipt.
      </P>

      <H2>Who writes the code</H2>
      <P>
        Not Lastro. The implementation comes from any coding agent or person. Lastro's job ends at a verdict and, when the code fails, a list of failures with
        counterexamples that any agent can act on. The loop "write, prove, read the feedback, fix" lives in your harness:
      </P>
      <Code
        lang="text"
        title="the loop, outside Lastro"
        code={`while true; do
  your-agent "implement the approved intents of P-001"   # Claude Code, Codex, a person…
  lastro prove P-001 ./decide.js && break                # exit 0: every proof passes
  lastro feedback P-001                                  # failures + counterexamples → back to the agent
done
lastro apply P-001                                       # the proofs join their use cases`}
      />
      <Callout kind="note" title="Harness-agnostic by design">
        Nothing in the SDKs depends on a specific agent product. New surfaces (an MCP server, an IDE panel, a CI step) wrap <C>createLastro</C> or call the CLI.
      </Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Use cases and proposals
// ---------------------------------------------------------------------------

const PROPOSAL_INTENT = `{
  "id": "P-002.I-01",
  "change": "modify",
  "useCase": "UC-001",
  "name": "Free shipping from 80",
  "description": "An order with a subtotal of 80 or more pays no shipping.",
  "provability": "formal",
  "source": { "quote": "Lower the free-shipping threshold to 80.", "start": 0, "end": 40 },
  "replaces": [{ "proof": "P-001.PF-01", "reason": "The threshold moves from 100 to 80." }],
  "status": "approved",
  "approvals": ["ana"]
}`;

export function ConceptProposals() {
  return (
    <>
      <H1>Use cases and proposals</H1>
      <Lead>
        Your application is a list of use cases, each guarded by proofs. Every change is a proposal whose intents add, change or remove use cases, and whose proofs
        join them once they pass.
      </Lead>

      <H2>Four entities</H2>
      <P>Every entity has an <C>id</C>, a stable kebab-case <C>slug</C>, a <C>name</C> and a <C>description</C>.</P>
      <DataTable
        head={["Entity", "Id", "Where it lives", "What it is"]}
        rows={[
          ["Use case", <C>UC-001</C>, <C>.lastro/usecases/UC-001-&lt;slug&gt;.json</C>, "One behavior of the application, described as the code implements it, with the proofs that guard it."],
          ["Proposal", <C>P-001</C>, <C>.lastro/proposals/P-001-&lt;slug&gt;.json</C>, "One change: its prose, the intents extracted from it, decisions, review threads and proofs."],
          ["Intent", <C>P-001.I-01</C>, "inside its proposal", "Adds, modifies or removes one use case, in natural language. People approve, amend or reject it."],
          ["Proof", <C>P-001.PF-01</C>, "in its proposal, then in its use case", "A clause or a scenario derived from one approved intent. Once it passes and the proposal is applied, it is a regression test."],
        ]}
      />
      <P>Ids are never reused. A proof keeps the id it was born with, so a use case's history reads straight from its proofs.</P>

      <H2>A use case</H2>
      <P>
        After the quickstart's <C>lastro apply P-001</C>, the shipping rule is one use case with four proofs. Every later proposal runs them as regression tests.
      </P>
      <Code lang="json" title=".lastro/usecases/UC-001-charge-shipping.json" code={USECASE} />

      <H2>Intents change use cases</H2>
      <P>
        <C>lastro propose</C> sends the prose together with every use case (its description and its proofs) to the extractor. Each intent it returns says what it does
        to which use case:
      </P>
      <DataTable
        head={["change", "Meaning", "replaces"]}
        rows={[
          [<C>add</C>, "A new use case. The proposal gives it a name, a slug and its description after the change.", "empty"],
          [<C>modify</C>, "Changes what an existing use case does. The proposal shows its description before and after.", "the proofs that stop holding, each with a reason"],
          [<C>remove</C>, "The use case goes away.", "every proof of the use case, filled in by Lastro"],
        ]}
      />
      <Code lang="json" title="an intent that moves the threshold" code={PROPOSAL_INTENT} />
      <P>
        <C>replaces</C> is the only way a proof stops running. Reviewers see it next to the intent (<C>⚠ replaces P-001.PF-01</C>), so dropping a regression test is
        something people approve, never something an LLM decides. Every other proof of the use case keeps running.
      </P>

      <H2>Review, like a pull request</H2>
      <P>
        The LLM is the author of the intents and people are the reviewers. Besides <A to="concepts/review">approve, amend and reject</A>, a reviewer can open a thread on
        an intent or a decision (<C>comment</C>), others can <C>reply</C>, and the one who opened it or the submitter can <C>resolve</C> it. The policy in{" "}
        <C>.lastro/config.json</C> sets how many approvals an intent needs and whether the person who submitted the proposal can approve it.
      </P>
      <Code
        lang="bash"
        code={`$ lastro review P-002 --as bruno comment I-01 "Does 80 include tax?"
$ lastro review P-002 --as ana reply T1 "No, the subtotal is before tax."
$ lastro review P-002 --as bruno resolve T1
$ lastro review P-002 --as bruno approve I-01`}
      />
      <P>
        Amending an intent can make the use case's new description stale: <C>review rewrite</C> writes it again, and <C>review recombine</C> asks the LLM to recombine
        the approved intents into one description.
      </P>

      <H2>Approval is bound to a baseline</H2>
      <P>
        A proposal is read against the use cases as they were: their ids, slugs, names and descriptions hash to a <strong>baseline</strong>. Once nothing is pending,
        the proposal is approved under a SHA-256 hash of its reviewed content, which includes that baseline. If another proposal is applied first and changes those
        use cases, Lastro refuses to approve or apply it: run <C>lastro propose</C> on it again, and the review carries over to every intent the new reading
        keeps.
      </P>

      <H2>Status</H2>
      <DataTable
        head={["status", "When"]}
        rows={[
          [<C>open</C>, "Intents or decisions still need review."],
          [<C>changes-requested</C>, "A use case description is stale, or derivation raised a decision."],
          [<C>approved</C>, "The review is done and the approval hash holds; no proofs yet."],
          [<C>proving</C>, "Proofs are derived; they have not all passed yet."],
          [<C>proved</C>, "The last run passed: every proof of the proposal and every regression proof."],
          [<C>applied</C>, "The proofs joined their use cases."],
          [<C>closed</C>, "Withdrawn without applying."],
        ]}
      />

      <H2>Proving and applying</H2>
      <P>
        <C>lastro prove P-001 ./code.js</C> runs the proposal's proofs and every proof of every use case, except the ones an intent replaces. A replaced proof still
        runs, only to report when it keeps passing (the change may not have happened). A failing proof of an existing use case is a <strong>regression</strong>, and it
        blocks the proposal like any other failure.
      </P>
      <P>
        <C>lastro apply P-001</C> refuses unless the proposal is proved and its baseline still holds. It then writes each use case's new description, adds the passing
        proofs, drops the replaced ones, deletes removed use cases, and records the proposal in each use case's <C>history</C>. <C>lastro regress</C> runs every use
        case's proofs on their own, for CI.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Intents
// ---------------------------------------------------------------------------

const INTENT_JSON = `{
  "id": "I1",
  "statement": "An order with a subtotal of 100 or more pays no shipping.",
  "kind": "behavior",
  "provability": "formal",
  "source": { "quote": "Orders of 100 or more ship free.", "start": 0, "end": 32 },
  "status": "approved",
  "approvals": ["ana"]
}`;

export function ConceptIntents() {
  return (
    <>
      <H1>Prose and intents</H1>
      <Lead>
        An intent is one checkable promise, restated precisely and tied to the exact words of the prose it came from. The list of intents, not the code and not the
        proof, is what people review.
      </Lead>

      <H2>From prose to intents</H2>
      <P>
        <C>plan</C> sends the prose to the extractor with a fixed system prompt and a JSON Schema. The rules it follows are part of the SDK (and identical in every
        SDK):
      </P>
      <Ul>
        <li>One intent per promise: "only X and always Y" becomes two intents.</li>
        <li>The quote is copied verbatim from the prose, never paraphrased. It anchors the intent map.</li>
        <li>When the prose is ambiguous, the extractor must not guess. It adds a <A to="concepts/decisions">decision</A> instead.</li>
        <li>Things a reader might assume but the prose does not promise (timing, rate limits…) go to <C>notPromised</C>.</li>
      </Ul>
      <Code lang="json" title="one intent of the engine's artifact" code={INTENT_JSON} />
      <Callout kind="note" title="Intents in a proposal">
        The CLI keeps intents in a <A to="concepts/proposals">proposal</A>, where each one also names the use case it adds, changes or removes and the existing
        proofs it replaces. The rules below hold for both.
      </Callout>

      <H2>The intent map</H2>
      <P>
        <C>source.start</C> and <C>source.end</C> are offsets into the prose, counted in UTF-16 code units in every SDK, so a UI can highlight the words behind each
        intent. Matching tolerates differences in case and whitespace, but an extraction whose quote is not in the prose at all is rejected with an error.
      </P>

      <H2>Kind and provability</H2>
      <DataTable
        head={["kind", "Meaning"]}
        rows={[
          [<C>behavior</C>, "what must happen"],
          [<C>invariant</C>, "what must never happen"],
          [<C>preserve</C>, "existing behavior that must keep working"],
          [<C>non-functional</C>, "performance, UX and the like"],
        ]}
      />
      <DataTable
        head={["provability", "When"]}
        rows={[
          [<C>formal</C>, "a pure input → output fact about the decision logic"],
          [<C>empirical</C>, "can only be checked by running examples"],
          [<C>beyond-proof</C>, <>falls where proof stops; needs a <C>beyondProofReason</C> (see <A to="concepts/verdicts">verdicts</A>)</>],
        ]}
      />
      <P>The extractor proposes both; the reviewer can change them when amending.</P>

      <H2>Editing the prose</H2>
      <P>
        Prose changes. Pass the previous artifact to <C>plan</C> (the CLI does it for you) and the review carries over: an intent whose statement matches one already
        reviewed keeps its verdict and approvals, an amendment is reapplied when the AI repeats its old reading of the same words, and decisions with the same
        question keep their answer. Everything else comes back as <C>proposed</C>, so people only look at what the edit changed.
      </P>
      <CodeTabs
        variants={[
          { label: "TypeScript", lang: "ts", code: `const next = await lastro.plan(editedProse, previous); // carryReview(previous, fresh)` },
          { label: "Go", lang: "go", code: `next, err := l.Plan(ctx, editedProse, &previous) // CarryReview(previous, fresh)` },
          { label: ".NET", lang: "csharp", code: `var next = await lastro.PlanAsync(editedProse, previous); // Artifacts.CarryReview(previous, fresh)` },
          { label: "CLI", lang: "bash", code: `$ lastro propose .lastro/proposals/P-001-shipping-fee.md   # re-extracts P-001 and carries its review` },
        ]}
      />
    </>
  );
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

export function ConceptReview() {
  return (
    <>
      <H1>Review and approval policy</H1>
      <Lead>Review is where "the AI misunderstood" gets caught, before anyone writes a proof or a line of code.</Lead>

      <H2>Review actions</H2>
      <DataTable
        head={["Action", "Effect"]}
        rows={[
          [<C>approve(…ids)</C>, "Signs the intents with the reviewer's name."],
          [<C>approveAll()</C>, "Approves every intent this reviewer has not signed yet. Rejected intents stay rejected."],
          [<C>amend(id, statement)</C>, "Rewrites the statement (the original is kept). Amended intents count as approved, and the new wording needs fresh approvals from everyone else."],
          [<C>reject(id, note)</C>, "The intent should not be built at all. A note is required."],
          [<C>answer(decisionId, option)</C>, <>Answers a <A to="concepts/decisions">decision card</A>.</>],
          [<C>add(draft)</C>, "Adds an intent the extractor missed: the unstated requirement made explicit."],
          [<C>notPromised(…items)</C>, "Records what is explicitly not promised."],
        ]}
      />

      <H2>The approval policy</H2>
      <P>
        The team decides how many distinct reviewers must approve each intent when it creates the SDK. The default is one. The policy is stored in the artifact, so
        every later step and every SDK enforces the same rule.
      </P>
      <CodeTabs
        variants={[
          {
            label: "TypeScript",
            lang: "ts",
            code: `const lastro = createLastro({ model, ask, requiredApprovals: 2 });

let intents = await lastro.plan(prose);
intents = lastro.review(intents, "ana").approveAll().amend("I5", "If both are sent, the email wins.").done();
intents = lastro.review(intents, "bruno").approveAll().done();
lastro.pending(intents); // [] once every intent has two approvals and every decision an answer`,
          },
          {
            label: "Go",
            lang: "go",
            code: `l := lastro.New(lastro.Options{Model: model, Ask: ask, RequiredApprovals: 2})

intents, _ := l.Plan(ctx, prose, nil)
intents, _ = l.Review(intents, "ana").ApproveAll().Amend("I5", "If both are sent, the email wins.").Done()
intents, _ = l.Review(intents, "bruno").ApproveAll().Done()
l.Pending(intents) // [] once the policy is met`,
          },
          {
            label: ".NET",
            lang: "csharp",
            code: `var lastro = new LastroClient(new LastroOptions { Model = model, Ask = ask, RequiredApprovals = 2 });

var intents = await lastro.PlanAsync(prose);
intents = lastro.Review(intents, "ana").ApproveAll().Amend("I5", "If both are sent, the email wins.").Done();
intents = lastro.Review(intents, "bruno").ApproveAll().Done();
lastro.Pending(intents); // [] once the policy is met`,
          },
          {
            label: "CLI",
            lang: "bash",
            code: `$ lastro init --model model.json --approvals 2
$ lastro propose prose.md --as ana
$ lastro review P-001 --as ana approve-all
$ lastro review P-001 --as ana amend I-05 "If both are sent, the email wins."
$ lastro review P-001 --as bruno approve-all
$ lastro status P-001          # pending is empty once the policy is met`,
          },
        ]}
      />
      <P>
        <C>pending</C> lists what still blocks the spec, in words a person can act on: <C>I3 not reviewed</C>, <C>I5 needs 1 more approval(s)</C>,{" "}
        <C>D1 unanswered: …</C>.
      </P>

      <H2>Approval</H2>
      <P>
        <C>spec</C> approves the artifact first: it refuses while anything is pending, then computes a SHA-256 hash over the artifact's canonical JSON (sorted keys,
        no whitespace). Everything after (the plan, the report, the receipt) is tied to that hash, and <C>isApproved</C> recomputes it to show nothing changed after
        review. To review an approved artifact again, work on a copy from <C>reopen</C>; the library does this for you.
      </P>
      <Callout kind="warning" title="Approve what you mean">
        A green proof means the code does what the approved intents say. If the AI misread the prose and you approved it, the proof will faithfully prove the
        misreading. The <A to="installation">sandbox</A>'s login example shows exactly this with intent I5.
      </Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Decisions
// ---------------------------------------------------------------------------

const CARD = `{
  "id": "D1",
  "intentIds": ["I1"],
  "question": "Is the 100 threshold measured before or after discounts?",
  "options": [
    { "label": "Before discounts", "consequence": "A 110 order with a 20 discount still ships free." },
    { "label": "After discounts", "consequence": "The same order pays 15; the model input must be the discounted subtotal." }
  ],
  "recommended": 1,
  "chosen": 1,
  "origin": "extraction"
}`;

export function ConceptDecisions() {
  return (
    <>
      <H1>Decisions</H1>
      <Lead>Ambiguity becomes a question with consequences, never a silent guess. Lastro asks in two places: when it reads the prose, and when the spec contradicts itself.</Lead>

      <H2>The decision card</H2>
      <P>One question, two to four options each with its consequence, and one recommended option.</P>
      <Code lang="json" title="a decision card" code={CARD} />
      <DataTable
        head={["origin", "Raised by", "What an answer does"]}
        rows={[
          [<C>extraction</C>, "plan, when the prose is ambiguous", "Records the choice. The derivation sees every decision and its answer."],
          [<C>consistency-check</C>, "spec, when two clauses cannot both hold", "Adds an intent that records the decision. It must be approved like any other."],
          [<C>verification</C>, "the verification report", "Same as a contradiction."],
        ]}
      />

      <H2>Contradictions go back to the team</H2>
      <P>
        After derivation, a consistency check looks for inputs where the clauses cannot all hold. Each contradiction becomes a decision card on a reopened copy of
        the artifact, so it cannot be approved again until someone answers. The answer adds an intent like{" "}
        <em>"&lt;question&gt; Decided: &lt;option&gt; (&lt;consequence&gt;)."</em>, which goes through review before the spec is derived again.
      </P>
      <CodeTabs
        variants={[
          {
            label: "TypeScript",
            lang: "ts",
            code: `let spec = await lastro.spec(intents);
if (!spec.ok) {
  intents = lastro.returnDecisions(spec);                 // contradictions → unanswered decisions
  intents = lastro.review(intents, "ana").answer("D3", 0).approveAll().done();
  spec = await lastro.spec(intents);                      // derive again
}`,
          },
          {
            label: "Go",
            lang: "go",
            code: `spec, _ := l.Spec(ctx, intents)
if !spec.OK {
	intents = l.ReturnDecisions(spec)
	intents, _ = l.Review(intents, "ana").Answer("D3", 0).ApproveAll().Done()
	spec, _ = l.Spec(ctx, intents)
}`,
          },
          {
            label: ".NET",
            lang: "csharp",
            code: `var spec = await lastro.SpecAsync(intents);
if (!spec.Ok)
{
    intents = lastro.ReturnDecisions(spec);
    intents = lastro.Review(intents, "ana").Answer("D3", 0).ApproveAll().Done();
    spec = await lastro.SpecAsync(intents);
}`,
          },
          {
            label: "CLI",
            lang: "bash",
            code: `$ lastro derive P-001                          # exit 1, status "back-to-review"
$ lastro review P-001 --as ana answer D3 0     # records the choice on the intents involved
$ lastro review P-001 --as ana approve-all
$ lastro derive P-001`,
          },
        ]}
      />
      <P>
        The CLI says the same in its output: <em>"Derivation raised decisions. Answer them with <C>lastro review P-001 --as &lt;name&gt; answer &lt;D&gt; &lt;option&gt;</C>, review the intents they reopened, and run <C>lastro derive P-001</C> again."</em>{" "}
        In a proposal, a derivation decision can also say that an approved intent breaks an existing proof of its use case: the proof is added to the intent's{" "}
        <C>replaces</C> and the intent goes back to review, so nobody drops a regression test without approving it.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Model and spec
// ---------------------------------------------------------------------------

const LOGIN_MODEL = `export const MODEL: Model = {
  name: "login_flow",
  inputs: {
    sends: D.enum("email", "username", "both", "neither"), // request fields
    accountFound: D.bool(),                                 // T1: find_by_email / find_by_username
    password: D.enum("right", "wrong", "bad-hash"),         // T2: check_password (bcrypt)
    infra: D.enum("ok", "db-fails", "signer-fails"),        // T1 db errors, T3 signer
  },
  outputs: {
    status: D.enum(200, 400, 401, 500),
    token: D.bool(),
    role: D.enum("none", "customer", "admin"),
  },
  defs: {
    named: {
      params: [],
      description: "the request names an account that exists and the database answered",
      body: L.and(L.neq(L.in("sends"), "neither"), L.in("accountFound"), L.neq(L.in("infra"), "db-fails")),
    },
  },
};`;

const CLAUSE = `{
  id: "C5",
  intentIds: ["I1"],
  description: "A wrong password for a readable account gets 401.",
  requires: L.and(L.call("named"), L.eq(L.in("password"), "wrong")),
  ensures: L.and(L.eq(L.out("status"), 401), L.not(L.out("token")), L.eq(L.out("role"), "none")),
  assumes: ["T2"],
}
// receipt: named() ∧ (in.password = "wrong") ⇒ (out.status = 401) ∧ ¬out.token ∧ (out.role = "none")`;

const TRUST = `{
  id: "T2",
  intentIds: ["I1"],
  piece: "check_password (bcrypt)",
  contract: "password=right iff the password matches the hash; bad-hash iff the hash is unreadable",
  youAccept: "bcrypt compares correctly",
  ifWrong: "any password may count as right: every password guarantee falls",
  checkedBy: ["S3"],
}`;

export function ConceptSpec() {
  return (
    <>
      <H1>Model and formal spec</H1>
      <Lead>
        The spec talks about a <em>decision core</em>: a pure function from the model's inputs to its outputs. Side effects and external code sit behind trusted
        pieces that the receipt names one by one.
      </Lead>

      <H2>The model</H2>
      <P>
        The model is the vocabulary of the proofs. You write it once per decision core. Inputs include facts that external pieces provide (did the database find the
        account? did bcrypt match?), like uninterpreted functions in a verifier.
      </P>
      <Code lang="ts" title="sdks/typescript/examples/login/login.ts" code={LOGIN_MODEL} />
      <DataTable
        head={["Domain", "JSON", "Size"]}
        rows={[
          [<C>D.bool()</C>, <C>{`{"type":"bool"}`}</C>, "2"],
          [<C>D.enum(…values)</C>, <C>{`{"type":"enum","values":[…]}`}</C>, "number of values"],
          [<C>D.int(min, max)</C>, <C>{`{"type":"int","min":0,"max":200}`}</C>, "max − min + 1; unbounded without both"],
          [<C>D.string(…examples)</C>, <C>{`{"type":"string","examples":[…]}`}</C>, "unbounded: sampled, never proved"],
        ]}
      />
      <P>
        When the product of the input domains is finite and at most 100,000 points, every input is checked. Otherwise the engine samples (2,000 inputs, seed 42) and
        the verdict can only be <Badge variant="evidence">evidenced</Badge>.
      </P>

      <H2>The proof plan</H2>
      <P>The derived spec is a proof plan with four kinds of obligation.</P>
      <H3>Clauses (formal)</H3>
      <P>
        "For every input: if <C>requires</C> holds, then <C>ensures</C> holds", written in <A to="reference/lsl">LSL</A>. One sentence of intent may need several
        clauses; the derivation prompt prefers both-ways clauses (the right password always gets a token <em>and</em> a wrong one never does), because a one-sided
        clause lets a program that refuses everyone pass.
      </P>
      <Code lang="ts" title="a clause" code={CLAUSE} />
      <ClauseAnatomy />
      <H3>Trusted pieces (assumptions)</H3>
      <P>The boundary of the proof, written as a trust card: what you accept, what breaks if it is wrong, and which scenarios check it.</P>
      <Code lang="ts" title="a trusted piece" code={TRUST} />
      <H3>Scenarios (empirical)</H3>
      <P>
        Given/When/Then examples with a concrete input for every model input and an LSL <C>expect</C>. They are evidence, never proof, and they are how trusted
        pieces get exercised.
      </P>
      <H3>Beyond proof</H3>
      <P>
        Intents (or parts of them) where proof stops, each with a reason: <C>external-code</C>, <C>world-facts</C>, <C>termination-timing</C>, <C>human-judgment</C>{" "}
        or <C>unstated</C>. See <A to="concepts/verdicts">where proof stops</A>.
      </P>

      <H2>Validation</H2>
      <P>Before anything runs, <C>validatePlan</C> checks the plan deterministically and rejects it when:</P>
      <Ul>
        <li>a clause, scenario or trusted piece cites an intent that was not approved;</li>
        <li>an expression uses a variable or definition the model does not declare, or an operator with the wrong number of arguments;</li>
        <li>an approved intent has neither an obligation nor a beyond-proof reason (reported as a <C>gap</C>).</li>
      </Ul>
      <P>
        Then the consistency check asks, for every input, whether at least one output satisfies every clause at once (when the output space has at most 4,096
        points). An input with no such output is a contradiction, and it goes back to the team as a <A to="concepts/decisions">decision</A> whose options are
        "Relax C<em>n</em>" for each clause involved.
      </P>
      <Code lang="json" title="lastro derive P-001 · stdout" code={OUT.derived} />
    </>
  );
}

// ---------------------------------------------------------------------------
// Verdicts
// ---------------------------------------------------------------------------

export function ConceptVerdicts() {
  return (
    <>
      <H1>Proved, trusted, evidenced</H1>
      <Lead>Every result says how much it can be trusted. A green check that rests on samples is never called a proof.</Lead>

      <H2>Intent verdicts</H2>
      <DataTable
        head={["Verdict", "When", "Evidence"]}
        rows={[
          [<Badge variant="proved">proved</Badge>, <>Every clause of the intent was checked for <strong>every</strong> input of the declared domain.</>, "formal"],
          [<Badge variant="trusted">proved-under-assumptions</Badge>, "Proved, but it relies on trusted pieces (for example, that bcrypt compares correctly).", "formal + trusted"],
          [<Badge variant="evidence">evidenced</Badge>, "The domain is too large or infinite: deterministic sampling found no failure, or only scenarios back it.", "empirical"],
          [<Badge variant="fail">violated</Badge>, "A counterexample was found, or a scenario failed. It is in the receipt.", "formal or empirical"],
          [<Badge variant="beyond">beyond-proof</Badge>, "The intent falls where proof stops and needs human sign-off.", "none"],
          [<Badge variant="outline">gap</Badge>, "An approved intent with no obligation.", "none"],
        ]}
      />
      <P>
        Each intent result also carries a <C>confidence</C>: 1 for proved, <C>1 − bound</C> for evidenced, 0 for violated and gaps.
      </P>

      <H2>Proved means exhaustive</H2>
      <P>
        In v0, "proved" is proof by exhaustion over a finite declared domain. It is a real proof, not a sample, but it holds <em>for the model</em>. The bridge
        between the model and the world is the trusted pieces, listed one by one in the receipt. An SMT backend (the spec already compiles to SMT-LIB2 with{" "}
        <C>toSmtLib</C>) is the next step.
      </P>

      <H2>Evidenced means sampled, with a bound</H2>
      <P>
        When the input space is too large, the engine checks a deterministic sample (fixed seed). With <em>N</em> failure-free samples, the failure rate is at most{" "}
        <C>3/N</C> at 95% confidence (the rule of three), and the receipt says exactly that.
      </P>
      <SamplingPlayground />

      <H2>Clauses that prove nothing</H2>
      <P>Passing is not enough. Each clause is also checked for being useful:</P>
      <DataTable
        head={["Clause verdict", "Meaning"]}
        rows={[
          [<C>vacuous</C>, "No input makes the if-part true. The clause holds and proves nothing."],
          [<C>weak</C>, "No planted mutant was rejected: the clause verifies but rules out no bug (\"vibe verification\")."],
        ]}
      />
      <P>
        Mutants are wrong versions of the program's output: every boolean output flipped, every output of a small domain (16 values or fewer) forced to each of
        its values, every integer output moved by one. A clause that no mutant can break is checking nothing the program decides. Vacuous and weak clauses are reported as <strong>spec gaps</strong>: work for the team, not for whoever writes the code.
      </P>

      <H2>Production traces</H2>
      <P>
        You can also pass recorded real executions (<C>traces</C>). A model proof can hold while reality disagrees: if a trace violates a proved clause, the clause
        becomes <C>violated</C> with the note "proved on the model, but a real trace violates it: a trusted piece (assumption) does not hold".
 Try it on <A to="examples/trust">Trusted pieces and traces</A>.
      </P>

      <H2>Where proof stops</H2>
      <DataTable
        head={["Reason", "Example", "What Lastro does"]}
        rows={[
          [<C>external-code</C>, "bcrypt, the database driver, the JWT library", "becomes a trusted piece backed by scenarios"],
          [<C>world-facts</C>, "database contents, the clock", "becomes a model input plus a trusted piece"],
          [<C>termination-timing</C>, "\"must answer in 200 ms\", a hung query", "beyond proof; may get an empirical scenario"],
          [<C>human-judgment</C>, "\"the error message must be clear\"", "beyond proof; human sign-off in the receipt"],
          [<C>unstated</C>, "a requirement nobody wrote down", "listed under Not promised"],
        ]}
      />

      <H2>When is a proof ok?</H2>
      <P>
        <C>proof.ok</C> is true when no intent is violated, no approved intent is a gap, and the spec has no open contradictions. With the Rust prover, it also needs
        the Rust prover's own <C>ok</C> (which requires a table covering the whole input space) and both engines to agree on every clause.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Receipts
// ---------------------------------------------------------------------------

export function ConceptReceipts() {
  return (
    <>
      <H1>Receipts and feedback</H1>
      <Lead>A proof produces two things for two audiences: a receipt for the people who approve the change, and a brief for whoever writes the code.</Lead>

      <H2>The receipt</H2>
      <P>Markdown, ready to paste into a pull request. In order, it shows:</P>
      <Ol>
        <li>the result and a count per verdict;</li>
        <li>the hashes of the approved intents and of the plan, and the Rust prover's coverage and table hash;</li>
        <li>every intent with its verdict, evidence, confidence and what backs it;</li>
        <li>every clause with its verdict, method, mutants killed, and its formal statement;</li>
        <li>scenarios, trusted pieces (with their trust cards), where proof stops, and what is not promised.</li>
      </Ol>
      <P>
        See the full receipt of the shipping example in the <A to="quickstart">quickstart</A>. Every SDK renders the same receipt, byte for byte, from the same
        report.
      </P>

      <H2>Two engines</H2>
      <P>
        The TypeScript engine (also ported to Go and .NET) and the Rust prover <C>lastro-core</C> are independent implementations of the LSL semantics. The Rust
        prover does not trust the caller: it gets a table of input/output rows, enumerates the declared input space itself, and reports coverage. When both run, the
        receipt says whether their verdicts agree, clause by clause.
      </P>
      <Code lang="markdown" code={`Independent check: \`lastro-core 0.1.0\` · coverage 402/402 inputs · table \`sha256:71f6c244f8ac…\` · verdicts agree`} />

      <H2>Failures and spec gaps</H2>
      <P>
        <C>proof.failures</C> are code problems: each violated clause or failed scenario, with the intents it serves, its formal statement and the input that breaks
        it. <C>proof.specGaps</C> are spec problems: vacuous or weak clauses, intents with no obligation, and contradictions. The CLI reports each failing proof
        with its use case, its intent, and whether it belongs to the proposal or is a regression of an existing use case:
      </P>
      <Code lang="json" title="lastro prove P-001 · failures[0]" code={JSON.stringify(JSON.parse(OUT.notProved).failures[0], null, 2)} />

      <H2>Feedback for the coding agent</H2>
      <P>
        <C>agentFeedback(proof)</C> renders the failures as a prompt-ready brief. Paste it into any agent, or let your harness do it.
      </P>
      <Code lang="text" title="lastro feedback" code={JSON.parse(OUT.feedback).feedback} />
      <CodeTabs
        variants={[
          { label: "TypeScript", lang: "ts", code: `import { agentFeedback } from "lastro";\nagentFeedback(proof);` },
          { label: "Go", lang: "go", code: `lastro.AgentFeedback(proof.Failures, proof.SpecGaps)` },
          { label: ".NET", lang: "csharp", code: `Feedback.AgentFeedback(proof);` },
          { label: "CLI", lang: "bash", code: `$ lastro feedback P-001` },
        ]}
      />
    </>
  );
}
