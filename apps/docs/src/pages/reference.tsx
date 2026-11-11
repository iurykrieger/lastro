import { A, C, Callout, Code, CodeTabs, DataTable, H1, H2, Lead, P, Ul } from "@/components/doc";

// ---------------------------------------------------------------------------
// LSL
// ---------------------------------------------------------------------------

const OPS: [string, string, string, string][] = [
  ["and", "1+", "a ∧ b", "all hold"],
  ["or", "1+", "a ∨ b", "at least one holds"],
  ["not", "1", "¬a", "negation"],
  ["implies", "2", "a ⇒ b", "if a then b"],
  ["iff", "2", "a ⇔ b", "a exactly when b"],
  ["eq / neq", "2", "a = b, a ≠ b", "equality of any values"],
  ["lt / le / gt / ge", "2", "a < b, a ≤ b, a > b, a ≥ b", "numeric comparison"],
  ["add / mul", "2+", "a + b, a · b", "arithmetic"],
  ["sub", "2", "a - b", "subtraction"],
  ["ite", "3", "if c then a else b", "conditional value"],
  ["in", "2+", "a ∈ {x, y}", "a equals one of the values"],
];

export function RefLsl() {
  return (
    <>
      <H1>LSL</H1>
      <Lead>
        The Lastro Spec Language is a tiny expression language written as JSON. It is safe to accept from an LLM because it is never evaluated as code: both
        engines interpret the AST, and every expression is validated against the model first.
      </Lead>

      <H2>Expressions</H2>
      <Code
        lang="text"
        code={`{"var": "in.<input>"}  |  {"var": "out.<output>"}  |  {"var": "<param>"}      variables
{"const": true | 15 | "admin"}                                                constants
{"op": "<operator>", "args": [ … ]}                                           operators
{"call": "<def>", "args": [ … ]}                                              model definitions`}
      />
      <P>
        Variables are namespaced: <C>in.*</C> are model inputs, <C>out.*</C> are outputs, and bare names are the parameters of a definition inside its body.
      </P>

      <H2>Operators</H2>
      <DataTable
        head={["op", "Arguments", "Shown as", "Meaning"]}
        rows={OPS.map(([op, n, shown, meaning]) => [<C>{op}</C>, n, <span className="font-mono text-[13px]">{shown}</span>, meaning])}
      />

      <H2>A clause, three ways</H2>
      <P>"A non-member under 100 pays exactly 15", as the derivation returns it, as the builders write it, and as the receipt shows it.</P>
      <CodeTabs
        variants={[
          {
            label: "JSON",
            lang: "json",
            code: `{
  "requires": { "op": "and", "args": [
    { "op": "not", "args": [{ "var": "in.member" }] },
    { "op": "lt", "args": [{ "var": "in.subtotal" }, { "const": 100 }] }
  ]},
  "ensures": { "op": "eq", "args": [{ "var": "out.fee" }, { "const": 15 }] }
}`,
          },
          { label: "TypeScript", lang: "ts", code: `import { L } from "lastro";\n\nconst requires = L.and(L.not(L.in("member")), L.lt(L.in("subtotal"), 100));\nconst ensures = L.eq(L.out("fee"), 15);` },
          { label: "Go", lang: "go", code: `requires := lastro.And(lastro.Not(lastro.In("member")), lastro.Lt(lastro.In("subtotal"), 100))\nensures := lastro.Eq(lastro.Out("fee"), 15)` },
          { label: ".NET", lang: "csharp", code: `var requires = L.And(L.Not(L.In("member")), L.Lt(L.In("subtotal"), 100));\nvar ensures = L.Eq(L.Out("fee"), 15);` },
          { label: "Receipt", lang: "text", code: `¬in.member ∧ (in.subtotal < 100) ⇒ out.fee = 15` },
        ]}
      />
      <P>
        Builders: <C>in</C>, <C>out</C>, <C>v</C> (a parameter), <C>c</C> (a constant), one per operator, <C>oneOf(a, …values)</C> for <C>in</C>, and{" "}
        <C>call(name, …args)</C>. Plain values are wrapped as constants.
      </P>

      <H2>Definitions</H2>
      <P>
        A model can name shared sub-expressions in <C>defs</C>, so clauses stay readable and the derivation reuses them. Recursion is cut off at depth 64.
      </P>
      <Code
        lang="json"
        code={`"defs": {
  "named": {
    "params": [],
    "description": "the request names an account that exists and the database answered",
    "body": { "op": "and", "args": [
      { "op": "neq", "args": [{ "var": "in.sends" }, { "const": "neither" }] },
      { "var": "in.accountFound" },
      { "op": "neq", "args": [{ "var": "in.infra" }, { "const": "db-fails" }] }
    ]}
  }
}`}
      />

      <H2>Validation</H2>
      <P>
        <C>validateExpr(expr, model)</C> returns every problem it finds: unknown variables, unknown definitions, wrong arity, malformed nodes. The derivation's
        answer goes through it before a plan exists, so an LLM cannot smuggle in a variable the model does not have.
      </P>

      <H2>SMT-LIB2</H2>
      <P>
        <C>toSmtLib(model, clause)</C> compiles <C>requires ⇒ ensures</C> into an SMT-LIB2 script that asserts its negation, for Z3, cvc5 or another solver.{" "}
        <C>unsat</C> means the clause holds for every pair the domains allow. This is the bridge to the planned SMT backend.
      </P>
      <Code
        lang="text"
        title="toSmtLib(model, C1)"
        code={`(set-logic ALL)
(declare-const |in.subtotal| Int)
(assert (>= |in.subtotal| 0))
(assert (<= |in.subtotal| 200))
(declare-const |in.member| Bool)
(declare-const |out.fee| Int)
(assert (and (>= |out.fee| 0) (< |out.fee| 2)))
(assert (not (=> (>= |in.subtotal| 100) (= |out.fee| 0))))
(check-sat)
(get-model)`}
      />
      <Callout kind="note">Enum values are encoded as their index in the domain, so a solver's model has to be read back through the declared values.</Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

export function RefDataModel() {
  return (
    <>
      <H1>Data model</H1>
      <Lead>
        Every artifact is versioned JSON with a <C>lastro</C> tag. The shapes are the same in every SDK: camelCase fields, kebab-case enum values.
      </Lead>

      <H2>Domain</H2>
      <Code
        lang="ts"
        code={`type Domain =
  | { type: "bool" }
  | { type: "enum"; values: (string | number)[] }
  | { type: "int"; min?: number; max?: number }
  | { type: "string"; examples?: string[] };`}
      />

      <H2>Model</H2>
      <Code
        lang="ts"
        code={`interface Model {
  name: string;
  inputs: Record<string, Domain>;
  outputs: Record<string, Domain>;
  defs?: Record<string, { params: string[]; body: Expr; description?: string }>;
}`}
      />

      <H2>IntentArtifact</H2>
      <Code
        lang="ts"
        code={`interface IntentArtifact {
  lastro: "intent-artifact/v0";
  prose: string;
  intents: Intent[];
  decisions: DecisionCard[];
  notPromised: string[];
  policy?: { requiredApprovals: number };
  hash?: string;                       // set by approve()
}

interface Intent {
  id: string;                          // "I1"
  statement: string;
  kind: "behavior" | "invariant" | "preserve" | "non-functional";
  provability: "formal" | "empirical" | "beyond-proof";
  beyondProofReason?: "external-code" | "world-facts" | "termination-timing" | "human-judgment" | "unstated";
  source: { quote: string; start: number; end: number };
  status: "proposed" | "approved" | "rejected" | "amended";
  reviewNote?: string;
  originalStatement?: string;          // kept when amended
  approvals?: string[];
  decisionId?: string;                 // the intent records the answer to this decision
}

interface DecisionCard {
  id: string;                          // "D1"
  intentIds: string[];
  question: string;
  options: { label: string; consequence: string }[];
  recommended: number;
  chosen?: number;
  origin: "extraction" | "consistency-check" | "verification";
}`}
      />

      <H2>ProofPlan</H2>
      <Code
        lang="ts"
        code={`interface ProofPlan {
  lastro: "proof-plan/v0";
  artifactHash: string;                // the approved intents it was derived from
  model: Model;
  clauses: { id: string; intentIds: string[]; description: string; requires?: Expr; ensures: Expr; assumes?: string[] }[];
  assumptions: { id: string; intentIds: string[]; piece: string; contract: string; youAccept: string; ifWrong: string; checkedBy: string[] }[];
  scenarios: { id: string; intentIds: string[]; given: string; when: string; then: string; input: Record<string, unknown>; expect: Expr }[];
  beyondProof: { intentId: string; reason: BeyondProofReason; note: string }[];
}`}
      />

      <H2>VerificationReport</H2>
      <Code
        lang="ts"
        code={`interface VerificationReport {
  lastro: "verification-report/v0";
  artifactHash: string;
  planHash: string;
  model: string;
  clauses: ClauseResult[];             // verdict, evidence, method, checked, counterexample?, mutants?, failureRateUpperBound95?
  scenarios: { scenarioId: string; status: "passed" | "failed"; output?: Env; error?: string }[];
  intents: IntentResult[];             // verdict, evidence[], clauses, assumptions, scenarios, confidence, notes
  assumptions: Assumption[];
  beyondProof: BeyondProofItem[];
  notPromised: string[];
  decisions: DecisionCard[];           // contradictions
  summary: Record<IntentVerdict, number>;
  ok: boolean;
}`}
      />
      <P>
        See <A to="concepts/verdicts">Proved, trusted, evidenced</A> for what each verdict means, and <A to="cli/workspace">the workspace</A> for where each
        object is stored.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// Rust prover
// ---------------------------------------------------------------------------

export function RefRust() {
  return (
    <>
      <H1>Rust prover</H1>
      <Lead>
        <C>lastro-core</C> (<C>crates/lastro-core</C>) is a second, independent implementation of the LSL semantics. It does not trust the caller: it enumerates the
        declared input space itself and only says "proved" when every point is covered.
      </Lead>

      <H2>What it checks</H2>
      <Ul>
        <li>Coverage: the size of the input space, the rows given, missing points, out-of-domain rows, and nondeterministic rows (one input, two outputs).</li>
        <li>Per clause: violation (with a counterexample), vacuity, and the same output mutants as the TypeScript engine.</li>
        <li>Consistency: clauses that cannot all hold together.</li>
        <li>A SHA-256 digest of the table, so a receipt names exactly what was checked.</li>
      </Ul>
      <P>
        A table that does not cover the space yields <C>evidenced</C> and <C>ok: false</C>, never <C>proved</C>.
      </P>

      <H2>As a CLI</H2>
      <Code
        lang="bash"
        code={`$ cargo build --release -p lastro-core --bin lastro-prove
$ lastro-prove < request.json > report.json     # exit 0 proved and consistent, 1 otherwise, 2 bad input`}
      />
      <CodeTabs
        variants={[
          {
            label: "request.json",
            lang: "json",
            code: `{
  "model": { "name": "shipping_fee", "inputs": { … }, "outputs": { … } },
  "clauses": [ { "id": "C1", "intentIds": ["I1"], "requires": { … }, "ensures": { … } } ],
  "table": [
    { "input": { "subtotal": 0, "member": false }, "output": { "fee": 15 } },
    { "input": { "subtotal": 0, "member": true }, "output": { "fee": 0 } }
  ],
  "consistencyLimit": 4096
}`,
          },
          {
            label: "report.json",
            lang: "json",
            code: `{
  "engine": "lastro-core 0.1.0",
  "model": "shipping_fee",
  "coverage": { "space": 402, "rows": 402, "complete": true, "missing": 0, "outOfDomain": 0, "nondeterministic": 0 },
  "clauses": [
    { "clauseId": "C1", "verdict": "proved", "evidence": "formal", "checked": 202, "mutants": { "total": 2, "killed": 1 } }
  ],
  "contradictions": [],
  "tableDigest": "sha256:71f6c244f8ac2c362d7c8afc7dcb2bab418a6a2534428e4e616eace5c29b68ca",
  "ok": true
}`,
          },
          {
            label: "partial table",
            lang: "json",
            code: `"coverage": { "space": 402, "rows": 300, "complete": false, "missing": 102,
              "firstMissing": { "member": false, "subtotal": 150 }, "outOfDomain": 0, "nondeterministic": 0 },
"clauses": [ { "clauseId": "C1", "verdict": "evidenced", … } ],
"ok": false`,
          },
        ]}
      />

      <H2>As WASM</H2>
      <P>
        The crate builds to <C>wasm32-unknown-unknown</C> with a raw ABI and no wasm-bindgen, so any host can load it:
      </P>
      <Ul>
        <li>
          <C>lastro_alloc(len) → ptr</C>, then write the UTF-8 request there;
        </li>
        <li>
          <C>lastro_prove(ptr, len) → u64</C>: the high 32 bits are the result pointer, the low 32 bits its length;
        </li>
        <li>
          read the JSON result, then <C>lastro_free(ptr, len)</C> both buffers.
        </li>
      </Ul>
      <Code
        lang="ts"
        code={`import { loadRustProver, tabulate } from "lastro";

const rust = await loadRustProver(readFileSync("sdks/typescript/wasm/lastro_core.wasm"));
const report = rust.prove(plan, await tabulate(plan.model, decide)); // tabulate runs the subject on every input`}
      />

      <H2>In the SDKs</H2>
      <DataTable
        head={["SDK", "How it runs the prover"]}
        rows={[
          ["TypeScript", <><C>loadRustProver(wasm)</C>, passed as <C>rust</C> to <C>createLastro</C>. The CLI loads <C>wasm/lastro_core.wasm</C> automatically.</>],
          ["Go", <><C>FindRustProver()</C>: <C>$LASTRO_PROVE</C>, then <C>PATH</C>, then <C>target/release</C> or <C>target/debug</C> upward from the working directory.</>],
          [".NET", <><C>RustProver.Locate()</C>: <C>$LASTRO_PROVE</C>, then <C>PATH</C>, then the <C>target/</C> of a Cargo workspace above the app's directory. Null when absent.</>],
        ]}
      />
      <P>
        <C>cargo test -p lastro-core && cargo clippy -p lastro-core -- -D warnings</C> runs its own tests; the SDK cross-checks make sure it and the TypeScript
        engine agree.
      </P>
    </>
  );
}
