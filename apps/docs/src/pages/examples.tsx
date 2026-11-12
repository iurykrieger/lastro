import { A, C, Callout, Code, DataTable, H1, H2, Lead, Ol, P, Ul } from "@/components/doc";
import { Badge } from "@/components/ui/badge";
import { Quiz } from "@/live/kit";
import { BugHunt, LoginRunner, ReviewPlayground, TrustPlayground } from "@/live/login";
import { ShippingPlayground } from "@/live/shipping";
import { PROSE } from "@lastro/example-login";
import { SHIPPING_PROSE } from "@/live/runtime";

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------

export function ExamplesOverview() {
  return (
    <>
      <H1>Try it in the browser</H1>
      <Lead>
        Every example in this section runs the real TypeScript SDK and the Rust prover (compiled to WebAssembly) in your browser. Nothing is mocked or recorded:
        change a control and the proof runs again.
      </Lead>
      <Callout kind="note" title="What runs, and what is replayed">
        Lastro has two AI steps, extracting intents and deriving the proofs. The examples replay those two answers (written by hand, the same way the{" "}
        <A to="quickstart">quickstart</A> does with the handoff), so they work offline and give the same result for everyone. Everything after that is live:
        review, approval, validation, both engines, the receipt and the agent feedback.
      </Callout>

      <H2>Two programs</H2>
      <DataTable
        head={["Example", "Input space", "What it shows"]}
        rows={[
          [<A to="examples/shipping">Shipping fee</A>, "402 inputs", "The quickstart's spec. Fix an off-by-one bug until every clause is proved."],
          [<A to="examples/login">Store login</A>, "72 inputs", "A login with roles, three trusted pieces, two decisions and a mistake by the AI that the review has to catch."],
        ]}
      />

      <H2>The examples</H2>
      <Ul>
        <li>
          <A to="examples/shipping">Fix the shipping fee</A>: edit the code with controls and read the proof, the feedback and the receipt.
        </li>
        <li>
          <A to="examples/login">Run one login</A>: pick a request, follow it through the code, and see which clauses apply to it.
        </li>
        <li>
          <A to="examples/bugs">Plant a bug</A>: six realistic bugs, the clause that catches each one, and the counterexample.
        </li>
        <li>
          <A to="examples/review">Review it yourself</A>: approve the AI's mistake, answer the decisions the other way, reject intents, and watch which code passes.
        </li>
        <li>
          <A to="examples/trust">Trusted pieces and traces</A>: what "proved under assumptions" means, and a production trace that breaks a proved clause.
        </li>
      </Ul>
      <P>
        Two more live widgets sit next to the concepts they explain: <A to="concepts/spec">the anatomy of a clause</A>, and{" "}
        <A to="concepts/verdicts">when proof stops</A>, where a bug survives 2,000 samples.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Shipping
// ---------------------------------------------------------------------------

export function ExampleShipping() {
  return (
    <>
      <H1>Fix the shipping fee</H1>
      <Lead>The quickstart's spec, with the code under your control. Start from the buggy version and make the proof pass.</Lead>
      <Code lang="markdown" title="prose.md" code={SHIPPING_PROSE} />
      <P>
        The reviewed spec has three clauses over a model with <C>subtotal</C> (an integer from 0 to 200) and <C>member</C> (a boolean), so the space has 201 × 2 =
        402 inputs. Both engines check all of them.
      </P>
      <ShippingPlayground />
      <H2>Things to try</H2>
      <Ol>
        <li>
          Switch the comparison to <C>&gt;=</C>. Every clause becomes <Badge variant="proved">Proved</Badge> and the feedback says "Every proof passes."
        </li>
        <li>Keep <C>&gt;=</C> and move the threshold to 99 or 101. One clause fails each time, and the counterexample sits exactly on the boundary.</li>
        <li>Turn off "Members ship free". C2 fails, and its counterexample is a member with a small cart.</li>
        <li>Set the fee for everyone else to 0. C1 and C2 still hold; C3 fails. A one-sided spec would have let free shipping for everyone pass.</li>
      </Ol>
      <Callout kind="tip">
        The text under "lastro feedback" is exactly what <C>agentFeedback(proof)</C> and <C>lastro feedback</C> hand to a coding agent: the clause, its LSL and a
        concrete counterexample.
      </Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

export function ExampleLogin() {
  return (
    <>
      <H1>Run one login</H1>
      <Lead>A store login with customers, administrators and three pieces of code the proof has to trust. Start by running a single request.</Lead>
      <Code lang="markdown" title="the prose" code={PROSE} />
      <P>
        The model has four inputs: what the request <C>sends</C> (email, username, both or neither), whether the lookup found the account, what bcrypt said about
        the password, and whether the infrastructure failed. That is 4 × 2 × 3 × 3 = 72 requests, and the reviewed spec has eleven clauses over them.
      </P>
      <LoginRunner />
      <H2>Reading the result</H2>
      <P>
        A clause is an implication. On most inputs its if-part is false and it <strong>does not apply</strong>; it can only fail on the inputs where it applies. A
        proof is this same check, run on all 72 requests at once: that is what the next examples do.
      </P>
      <Ul>
        <li>
          "Both fields sent" signs in as a customer: the reviewed I5 says the email wins. C11 applies and holds.
        </li>
        <li>"Database down" stops at the lookup with 500. Only C7 applies.</li>
        <li>
          Pick a bug under "Implementation" and look for a request where a clause <strong>fails</strong>. Then see how fast the prover finds one on{" "}
          <A to="examples/bugs">Plant a bug</A>.
        </li>
      </Ul>
    </>
  );
}

export function ExampleBugs() {
  return (
    <>
      <H1>Plant a bug</H1>
      <Lead>Six small edits to the reference login, each a bug someone has shipped. The proof names the clause it breaks and the request that shows it.</Lead>
      <BugHunt />
      <H2>What to look for</H2>
      <Ul>
        <li>
          <strong>Refuse everyone</strong> never hands out a wrong token, so every "only the right password" clause still holds. It is caught by C3, the both-ways
          clause from decision D1 ("the right password always gets a token"). Five other clauses become <C>vacuous</C>: no run ever issues a token, so they prove
          nothing.
        </li>
        <li>
          <strong>Check the username first</strong> and <strong>write "admin" into every token</strong> break the same three role clauses, with different
          counterexamples.
        </li>
        <li>
          <strong>Treat an unreadable hash as a wrong password</strong> is a bug only because the team answered D2 with "500, as today". Answer it the other way on{" "}
          <A to="examples/review">Review it yourself</A> and it becomes the correct code.
        </li>
      </Ul>
    </>
  );
}

export function ExampleReview() {
  return (
    <>
      <H1>Review it yourself</H1>
      <Lead>
        The proof can only be as right as the intents behind it. Here the AI misread one sentence of the prose, and the review is the only place that catches it.
      </Lead>
      <Quiz
        question={`The prose says: "If both are sent, the email wins." Which intent should the reviewer approve?`}
        options={[
          "If email and username are sent together, the request is invalid (400).",
          "If both email and username are sent, the email wins: the account is looked up among customers.",
          "Either one: the proof will catch the difference later.",
        ]}
        answer={1}
        explain="The first one is what the AI extracted. If you approve it, the spec demands a 400, the correct code fails the proof, and an agent iterating against that spec would break the login to make it pass."
      />
      <ReviewPlayground />
      <H2>Experiments</H2>
      <DataTable
        head={["Change", "With the reference code", "Why"]}
        rows={[
          ["Approve the AI's reading of I5", <Badge variant="fail">C11 fails</Badge>, "The spec now asks for 400 when both fields are sent; the code lets the email win, as the prose says."],
          ["D1: only refuse the wrong one", <Badge variant="proved">Proved</Badge>, <>The both-ways clause disappears. Pick "Refuse everyone" as the code: no clause fails any more.</>],
          ["D2: 401", <Badge variant="fail">C6 fails</Badge>, <>The reference answers 500 for an unreadable hash. "Treat an unreadable hash as a wrong password" now passes.</>],
          ["Reject I2", <Badge variant="proved">Proved</Badge>, <>Same effect as the second answer to D1: "Refuse everyone" no longer fails, it only leaves vacuous clauses.</>],
          ["Reject I7", <Badge variant="proved">Proved</Badge>, <>Nobody promised 500 for a database failure any more, so "Report a database outage as 401" passes.</>],
        ]}
      />
      <P>
        Every change produces a new artifact hash. A spec, a proof and a receipt are tied to that hash, so a proof never outlives the review it was derived from.
        See <A to="concepts/review">Review and approval policy</A> and <A to="concepts/decisions">Decisions</A>.
      </P>
    </>
  );
}

export function ExampleTrust() {
  return (
    <>
      <H1>Trusted pieces and traces</H1>
      <Lead>
        The login calls a database lookup, bcrypt and a JWT signer. The proof cannot see inside them, so it names them, states their contracts, and proves the
        rest.
      </Lead>
      <TrustPlayground />
      <H2>What the verdicts mean</H2>
      <Ul>
        <li>
          I1 and I2 are <Badge variant="trusted">Proved under assumptions</Badge>: true for every request, if the lookup and bcrypt keep their contracts.
        </li>
        <li>
          I3 to I7 are <Badge variant="proved">Proved</Badge>: they hold whatever the trusted pieces return.
        </li>
        <li>
          I8 ("responds quickly") and I9 ("the message is clear") are <Badge variant="beyond">Beyond proof</Badge>: timing and human judgment go to a load test and
          a product sign-off, and the receipt says so.
        </li>
      </Ul>
      <P>
        Turn on the trace. A real request with a wrong password got a token, which no run of the code on the model can do. The code is not wrong; a trusted piece
        is. Lastro reports the clauses as violated with a note pointing at the assumption, and I1 drops to <Badge variant="fail">Violated</Badge>. See{" "}
        <A to="concepts/verdicts">Proved, trusted, evidenced</A>.
      </P>
    </>
  );
}
