# Lastro design (v0)

> **Lastro** is what backs something. A plan written in prose is non-deterministic and read by an AI. Lastro gives it
> a deterministic backing: intents a human reviewed, proofs derived from them, and a reproducible check against what
> the program actually does.

Direct inspiration: [PDD Kit (GitHub Next and Microsoft Research)](https://githubnext.com/projects/pdd-kit/).

## 1. What we take from PDD Kit

| PDD Kit idea | In Lastro |
|---|---|
| Human review moves from code up to **intent** | The reviewable artifact is the list of intents, not the code or the proof |
| Intent artifact with an **intent map** (each requirement tied to the user's words) | Every intent carries `source.quote` plus offsets into the original prose |
| **Decision cards**: one question, options with consequences, one recommended | Native `DecisionCard`; ambiguity becomes a question, never a silent guess |
| Formalizable parts become `requires/ensures` contracts; the rest becomes a tested informal spec | `Clause` (formal) and `Scenario` Given/When/Then (empirical) |
| External dependencies become uninterpreted functions with a trusted contract | The decision core receives external facts as **inputs**; each fact is an `Assumption` (trusted piece) |
| **Trust card**: you accept X; if it is wrong, Y breaks; checked by tests Z | `Assumption { youAccept, ifWrong, checkedBy[] }` |
| **Mutation check** against vibe verification | Every clause must reject at least one mutant, or it is reported as `weak` |
| Contradictions in the spec go back to the team as a decision | The consistency check emits a `DecisionCard` |
| **Receipt** in the PR | `renderReceipt()` in Markdown, with hashes and the Rust cross-check |
| Evidence labels: formal (proof) vs empirical (tests are evidence, not proof) | Every result carries `evidence: "formal" | "empirical" | "trusted" | "none"` |

## 2. Pipeline

1. **Extract** (non-deterministic). An `IntentExtractor` (a Claude adapter is included) reads the prose and returns
   intents with quotes, kind, proposed provability and open questions.
2. **Review** (human). Approve, reject, amend, answer decision cards. This is where "the AI misunderstood" is caught
   early. `freeze()` then computes a **canonical hash** of the artifact; everything after is tied to it.
3. **Derive** (non-deterministic, but gated). A `ProofDeriver` turns approved intents into a `ProofPlan` written in
   **LSL** (Lastro Spec Language, a JSON AST that is never `eval`ed). `validatePlan()` checks deterministically that
   only approved intents are cited, every approved intent has an obligation or a beyond-proof reason, and every
   expression uses only model variables.
4. **Verify** (deterministic). The plan is checked against the program's **definitive output**: a function of the
   program (`subject`) and/or recorded production traces. Two engines run it: the TypeScript engine and the
   independent Rust prover `lastro-core`.
5. **Receipt**. Per intent: the level of assurance and what it rests on.

## 3. Proved vs confidence: verdict semantics

| Verdict | When | Label |
|---|---|---|
| `proved` | Checked for **every** input in the declared domain (exhaustive enumeration today, SMT next) | formal |
| `proved-under-assumptions` | Proved, but depends on trusted pieces (for example, that bcrypt compares correctly) | formal + trusted |
| `evidenced` | Domain too large or infinite: deterministic sampling (fixed seed) found no failure. With *N* failure-free samples, the failure rate is at most 3/N at 95% confidence (rule of three) | empirical |
| `violated` | A counterexample was found; it is in the receipt | formal or empirical |
| `vacuous` / `weak` | The if-part is never true, or no mutant was rejected: the clause proves nothing useful | gap |
| `beyond-proof` | The intent falls in one of the categories where proof stops and needs human sign-off | none |
| `gap` | An approved intent with no obligation | none |

"Proved" in v0 means **proof by exhaustion over a finite declared domain**. It is a real proof, not a sample, but it
holds for the model. The bridge between the model and the world is the trusted pieces, which the receipt lists one
by one.

## 4. Where proof stops

| `BeyondProofReason` | Example | What Lastro does |
|---|---|---|
| `external-code` | bcrypt, the database driver, the JWT library | becomes an `Assumption` backed by tests |
| `world-facts` | database contents, the clock | becomes a model input plus an `Assumption` |
| `termination-timing` | "must answer in 200 ms", a hung query | `beyond-proof`; may get an empirical scenario |
| `human-judgment` | "the error message must be clear" | `beyond-proof`; human sign-off in the receipt |
| `unstated` | a requirement nobody wrote down | listed under **Not promised** |

The receipt never hides these parts: they appear as trusted or beyond proof, with the reason.

## 5. The Rust prover (`lastro-core`)

`lastro-core` is a second, independent implementation of the LSL semantics. It does not trust the caller's claims:

- It receives the model, the clauses and a **table** of input/output rows. It enumerates the declared input space
  itself and reports coverage (`rows`, `space`, `missing`, out-of-domain and nondeterministic rows).
- It says `proved` only when the table covers every point of the space. A partial table yields `evidenced` and
  `ok: false`.
- Per clause it checks violation, vacuity and mutants (boolean negation, every other constant for domains with at
  most 16 values, integer ±1), and it runs a pairwise consistency check that reports contradictory clauses.
- It returns a SHA-256 digest of the table, so a receipt names exactly what was checked.

It builds as a native CLI (`lastro-prove`, JSON on stdin) and as `wasm32-unknown-unknown` with a small raw ABI
(`lastro_alloc`, `lastro_free`, `lastro_prove`). The SDK loads it with `loadRustProver()` and builds the table with
`tabulate()`. The test suite asserts that both engines agree verdict by verdict on the reference login and on every
planted bug.

## 6. The sandbox

`apps/sandbox` is the review surface. It has three parts:

- **Prose**: the text with the intent map highlighted, intent cards (approve, amend, reject) and decision cards.
- **Code**: the generated implementation, editable, with one-click planted bugs shown as a diff.
- **Contrast**: per-clause verdicts from both engines, mutants, counterexamples you can run, LSL and SMT-LIB2,
  the intent roll-up, trusted pieces, where proof stops, a single-login runner and the receipt.

Every verdict on screen comes from the real SDK and the WASM prover running in the page.

## 7. Out of scope for v0

- Proving arbitrary TypeScript directly. v0 proves the **decision core** (a pure function over facts), as PDD Kit
  does with `decide_login`; side effects sit behind trusted pieces.
- Unbounded string domains: empirical evidence only.

## 8. Next steps

1. SMT backend (Z3 via WASM) using `toSmtLib`.
2. CLI `lastro extract | review | derive | verify | receipt` and a GitHub Action that posts the receipt on the PR.
3. Native bindings for `lastro-core` (N-API, PyO3) so other language SDKs share the prover.
4. A comprehension quiz generated from the clauses, to confirm the reviewer understood what they approved.
