# Receipt: P-006 Feature map, formal proofs whenever possible, and review with before/after snapshots

Status: **proving** · submitted by iury · baseline `sha256:e1df572be471d7f3a5c74a900afa64c438ffb7d13755a66229196de4ecbc7c07` · approval `sha256:5abe81fe01967f4d7a19964ed533613d37b8f2c7e01fb0b514822d2e27a15673`

Features learn their surfaces and dependencies, derivation proves formally whenever a finite model fits (including state machines with interleaving actors, and SMT when the space is too large), drivers run the application, action-sequence properties are evidenced, and reviewers see before/after snapshots of screens, contracts, data and examples per intent.

## Use cases

### ✏️ UC-046 Store use cases and proposals (modify)

- Before: The workspace keeps one file per entity: each use case in .lastro/usecases/<id>-<slug>.json (id, slug, name, description, history), each proof of a use case in .lastro/proof/<proof-id>-<slug>.json and each proposal in .lastro/proposals/<id>-<slug>.json, with its receipt next to it. Use cases are numbered UC-001, UC-002... and proposals P-001, P-002... with the next free number, never reused even after a removal; intents and proofs are numbered inside their proposal (P-007.I-03, P-007.PF-05) and keep that id when a proof moves to its use case; proofs derived straight from a use case are numbered inside it (UC-003.PF-01). Slugs are kebab-case and stable: a rename changes only the name. intents.json, spec.json and proof.json are no longer written. A use case file that still carries a proofs list is read as before, and its proofs move to .lastro/proof/ the next time the workspace writes that use case.
- After: The workspace keeps one file per entity: each use case in .lastro/usecases/<id>-<slug>.json (id, slug, name, description, history), each proof of a use case in .lastro/proof/<proof-id>-<slug>.json and each proposal in .lastro/proposals/<id>-<slug>.json, with its receipt next to it. Use cases are numbered UC-001, UC-002... and proposals P-001, P-002... with the next free number, never reused even after a removal; intents and proofs are numbered inside their proposal (P-007.I-03, P-007.PF-05) and keep that id when a proof moves to its use case; proofs derived straight from a use case are numbered inside it (UC-003.PF-01). Slugs are kebab-case and stable: a rename changes only the name. intents.json, spec.json and proof.json are no longer written. A use case file that still carries a proofs list is read as before, and its proofs move to .lastro/proof/ the next time the workspace writes that use case. A use case file may also carry surfaces, each with a kind (screen, route, command or api), a name in the user's words and a locator a driver understands, and dependsOn, the ids of the use cases it needs to work; both are optional.
- **P-006.I-01** `approved` A feature file may carry surfaces (kind screen, route, command or api; a name in the user's words; a locator the driver understands, such as a URL path, a CLI command, an HTTP method and path or a selector) and dependsOn, the ids of the features it needs to work. Both are optional, so existing files stay valid.
- Gains: P-006.PF-01, P-006.PF-02

### ✏️ UC-001 Extract intents from prose (modify)

- Before: propose(prose, { submittedBy }) creates the next proposal file in .lastro/proposals/ with status open and sends the prose and the current use cases to an LLM through an AskJson transport with a fixed system prompt and JSON Schema (step "extract"). The answer lists intents, each the addition, removal or change of exactly one use case: id (P-n.I-nn), slug, name, a natural-language description of the change, change (add, modify or remove), the use case (an existing id, or a newly reserved id and slug for an add), a provability (formal, empirical, beyond-proof), a beyondProofReason ("none" or one of the five reasons) and a verbatim quote, and replaces: the existing proofs of that use case the new behavior contradicts, each with a reason (extraction receives every touched use case with its proofs in readable notation; for a remove the SDK lists every proof of the use case itself); one resulting use case per touched use case with its name and, for add and modify, the complete new description (after), all its intents combined, and the intents that produce it; decisions that reference intents by zero-based index; and notPromised items. The SDK fills each modify and remove with the use case's current description (before); the LLM never supplies it. The proposal (lastro: proposal/v1) records the baseline (sha256 over the canonical use cases without their proofs), the submitter, every intent proposed with no approvals, decisions D1, D2... with origin extraction, and the review policy. Intents have no kind. Without an LLM transport or a custom extractor, propose() throws.
- After: propose(prose, { submittedBy }) creates the next proposal file in .lastro/proposals/ with status open and sends the prose and the current use cases to an LLM through an AskJson transport with a fixed system prompt and JSON Schema (step "extract"). The answer lists intents, each the addition, removal or change of exactly one use case: id (P-n.I-nn), slug, name, a natural-language description of the change, change (add, modify or remove), the use case (an existing id, or a newly reserved id and slug for an add), a provability (formal, empirical, beyond-proof), a beyondProofReason ("none" or one of the five reasons) and a verbatim quote, and replaces: the existing proofs of that use case the new behavior contradicts, each with a reason (extraction receives every touched use case with its proofs in readable notation; for a remove the SDK lists every proof of the use case itself); one resulting use case per touched use case with its name and, for add and modify, the complete new description (after), all its intents combined, and the intents that produce it; decisions that reference intents by zero-based index; and notPromised items. For every use case it adds, the answer also proposes surfaces and dependencies; for one it modifies, it keeps the existing surfaces and dependencies and adds or removes only those the change calls for. They are stored with the after and reviewed with its intents. The SDK fills each modify and remove with the use case's current description (before); the LLM never supplies it. The proposal (lastro: proposal/v1) records the baseline (sha256 over the canonical use cases without their proofs), the submitter, every intent proposed with no approvals, decisions D1, D2... with origin extraction, and the review policy. Intents have no kind. Without an LLM transport or a custom extractor, propose() throws.
- **P-006.I-02** `approved` For a feature an intent adds, the extract step proposes its surfaces and dependencies; for a feature an intent modifies, it keeps the existing ones and only adds or removes those the change calls for. They are stored with the after and reviewed with the intent.
- Gains: P-006.PF-03, P-006.PF-04

### ➕ UC-055 Print the feature map (add)

- After: lastro map prints the feature map deterministically: every feature with its surfaces, the features it depends on and the features that depend on it; a feature with no surface is listed as not reachable by the driver. It rejects a dependency on a feature that does not exist or was removed, and a dependency cycle, naming the features in it. When a proposal changes a feature, every feature that depends on it directly or transitively is listed as affected, and their proofs and snapshots run again with the proposal's own.
- **P-006.I-03** `approved` lastro map prints, deterministically, every feature with its surfaces, the features it depends on and the features that depend on it; a feature with no surface is listed as not reachable by the driver.
- **P-006.I-04** `approved` The feature map rejects a dependency on a feature that does not exist or was removed, and a dependency cycle, naming the features in the cycle.
- **P-006.I-05** `approved` When a proposal changes a feature, every feature that depends on it directly or transitively is listed as affected, and their proofs and snapshots run again with the proposal's own.
- Gains: P-006.PF-05, P-006.PF-06, P-006.PF-07, P-006.PF-08, P-006.PF-09

### ➕ UC-056 Bootstrap the feature map (add)

- After: lastro bootstrap --surfaces has the survey step read the code and propose surfaces and dependencies for the features that have none. It writes a bootstrap proposal whose intents only add surfaces and dependencies and never change a description, reviewed like any other proposal.
- **P-006.I-06** `approved` lastro bootstrap --surfaces has the survey step read the code and write a bootstrap proposal for features with no surfaces; its intents only add surfaces and dependencies and never change a description, and it is reviewed like any other proposal.
- Gains: P-006.PF-10, P-006.PF-11, P-006.PF-12

### ✏️ UC-013 Derive the proof plan (modify)

- Before: derive(proposal) requires an approved proposal and asks the LLM (step "derive") with the models declared in .lastro/config.json, each touched use case's before and after with its approved and amended intents, the existing proofs of those use cases, each decision's question and answer (the chosen or else the recommended option) and notPromised. It writes pending proofs into the proposal, numbered P-n.PF-nn: each has a slug, name and description, the intent and use case it checks, the model and subject it runs against, and a check that is either a clause (requires and ensures in LSL, assumptions used) or a scenario (concrete input and an LSL expect); plus assumptions and beyondProof items. Every remove intent gets a proof, written from the use case's before, that the old behavior no longer happens; when the model cannot express it, derivation returns a decision for the reviewer and the proposal cannot be applied while it is open. Derivation never retires a proof: the proposal's supersedes is the union of replaces over the approved and amended intents, each new proof that takes the place of one lists it, and an existing proof the approved behavior breaks that no approved intent replaces comes back as a decision that sends the intent back to review with the proof added to its replaces. Every expression is parsed and validated. The proposal moves to proving. Without an LLM transport or a custom deriver, derive() throws.
- After: derive(proposal) requires an approved proposal and asks the LLM (step "derive") with the models declared in .lastro/config.json, each touched use case's before and after with its approved and amended intents, the existing proofs of those use cases, each decision's question and answer (the chosen or else the recommended option) and notPromised. It writes pending proofs into the proposal, numbered P-n.PF-nn: each has a slug, name and description, the intent and use case it checks, the model and subject it runs against, and a check that is either a clause (requires and ensures in LSL, assumptions used) or a scenario (concrete input and an LSL expect); plus assumptions and beyondProof items. Every remove intent gets a proof, written from the use case's before, that the old behavior no longer happens; when the model cannot express it, derivation returns a decision for the reviewer and the proposal cannot be applied while it is open. Derivation never retires a proof: the proposal's supersedes is the union of replaces over the approved and amended intents, each new proof that takes the place of one lists it, and an existing proof the approved behavior breaks that no approved intent replaces comes back as a decision that sends the intent back to review with the proof added to its replaces. For each approved intent the derive step first tries a finite model the code can be checked against exhaustively (an input space of at most 100,000 points, or a state machine with at most 100,000 reachable states), then an SMT proof (the clauses compiled to SMT-LIB and checked by an SMT solver, by induction over the steps for a state machine), and falls back to sampling, traces or scenarios only when neither can express the intent, recording why as a reason where proof stops or as a domain too large to enumerate. Every expression is parsed and validated. The proposal moves to proving. Without an LLM transport or a custom deriver, derive() throws.
- **P-006.I-07** `approved` For each approved intent the derive step first tries a finite model checked exhaustively (at most 100,000 inputs or reachable states), then an SMT proof (clauses compiled to SMT-LIB, by induction for state machines), and falls back to sampling, traces or scenarios only when neither can express the intent, recording why.
- Gains: P-006.PF-13, P-006.PF-14, P-006.PF-15

### ✏️ UC-026 Render a receipt (modify)

- Before: The receipt is Markdown for a pull request, written next to the proposal: the overall result and counts per verdict; the proposal and plan hashes; the Rust check with coverage, table digest and whether verdicts agree; the use-case diff, with each use case added, modified (before and after) or removed with the intents behind it, each replaced proof next to the proofs that take its place, each superseded proof that still passes, each regression and, after apply, the new baseline hash; a table of intents with verdict, evidence, confidence and what backs each; notes; each proof with verdict, evidence, method (exhaustive, samples with failure bound, traces), mutants killed, formal statement, counterexample, violating trace and notes; trusted pieces; where proof stops; not promised; and pending contradictions.
- After: The receipt is Markdown for a pull request, written next to the proposal: the overall result and counts per verdict; the proposal and plan hashes; the Rust check with coverage, table digest and whether verdicts agree; the use-case diff, with each use case added, modified (before and after) or removed with the intents behind it, each replaced proof next to the proofs that take its place, each superseded proof that still passes, each regression and, after apply, the new baseline hash; a table of intents with verdict, evidence, confidence and what backs each; notes; each proof with verdict, evidence, method (exhaustive, samples with failure bound, traces), mutants killed, formal statement, counterexample, violating trace and notes; trusted pieces; where proof stops; not promised; pending contradictions; the count of intents proved formally, evidenced and beyond proof, with the reason for each one that is not formal; a Feature map section with the affected features; a Mermaid state diagram for every state-machine model; the snapshots with their hashes and comparisons; the unexpected changes and who accepted them; and the sign-offs with who gave them.
- **P-006.I-08** `approved` The receipt counts how many intents are proved formally, evidenced and beyond proof, with the reason for each one that is not formal.
- **P-006.I-35** `approved` The receipt adds a Feature map section with the affected features, a Mermaid state diagram for every state-machine model, the snapshots with their hashes and comparisons, the unexpected changes and who accepted them, and the sign-offs with who gave them.
- Gains: P-006.PF-16, P-006.PF-48

### ✏️ UC-011 Declare the proof vocabulary (modify)

- Before: A model has a name, inputs and outputs, each a domain: bool, enum of strings or numbers, int with optional min and max, or string with optional examples; plus named defs (parameters and an LSL body). Bool, enum and bounded int domains are finite and enumerable in a fixed, sorted-key order; unbounded ints and strings are not. Facts supplied by trusted external pieces enter as inputs. A model with no outputs is rejected by plan validation.
- After: A model has a name, inputs and outputs, each a domain: bool, enum of strings or numbers, int with optional min and max, or string with optional examples; plus named defs (parameters and an LSL body). Bool, enum and bounded int domains are finite and enumerable in a fixed, sorted-key order; unbounded ints and strings are not. Facts supplied by trusted external pieces enter as inputs. A model with no outputs is rejected by plan validation. A model may instead be a state machine: state variables with finite domains, an initial state and events, each with finite inputs, and optionally actors, several copies of one process that share part of the state; its subject is a step function from a state and an event, with the actor that raised it, to the next state.
- **P-006.I-09** `approved` A model may be a state machine: state variables with finite domains, an initial state and events each with finite inputs, optionally with actors (several copies of one process sharing part of the state); the code under proof is a step function from a state and an event, with the actor that raised it, to the next state.
- Gains: P-006.PF-17

### ➕ UC-057 Prove state machines (add)

- After: A state-machine model's clauses are invariants, which hold in every reachable state; step clauses, which relate a state, an event and the next state; and reachability clauses, which say a state satisfying a condition can be reached and guard the others against proving nothing. The events of its actors are explored in every possible interleaving. When the reachable state space across every interleaving has at most 100,000 states it is visited breadth first, and a clause that holds everywhere it applies is proved, with formal evidence and the number of states visited. A larger or unbounded space is first proved by SMT induction (the invariant holds initially and every step preserves it), which counts as proved with the method named; only when the solver cannot decide is it explored by random walks from the initial state with a fixed seed, and a clause that holds is then evidenced, never proved. A violation comes back as the shortest sequence of events, each with its actor, from the initial state that reaches it. Every state-machine model is drawn as a statechart from the user's point of view: states, transitions labelled with the events that cause them, each actor as a parallel region, and the states each invariant forbids marked, shown to the reviewer next to the intent's proofs with the counterexample path drawn on it.
- **P-006.I-10** `approved` The events of a state machine's actors are explored in every possible interleaving, so races between them are found.
- **P-006.I-11** `approved` State-machine clauses are invariants (hold in every reachable state), step clauses (relate a state, an event and the next state) and reachability clauses (a state satisfying a condition can be reached), which guard the others against proving nothing.
- **P-006.I-12** `approved` When the reachable state space across every interleaving has at most 100,000 states it is visited breadth first, and a clause that holds everywhere it applies is proved, with formal evidence and the number of states visited.
- **P-006.I-13** `approved` A state space too large to enumerate is first proved by SMT induction (the invariant holds initially and every step preserves it), which counts as proved with the method named; only when the solver cannot decide is it explored by random walks with a fixed seed, and a clause that holds is then evidenced, never proved.
- **P-006.I-14** `approved` A violated state-machine clause comes back with the shortest sequence of events, each with its actor, from the initial state that reaches the violation.
- **P-006.I-15** `approved` Every state-machine model is drawn as a statechart from the user's point of view (states, transitions labelled with events, actors as parallel regions, states each invariant forbids marked), shown next to the intent's proofs with the counterexample path drawn on it; a reviewer requests changes on the intent when it does not match what they meant.
- Gains: P-006.PF-18, P-006.PF-19, P-006.PF-20, P-006.PF-21, P-006.PF-22, P-006.PF-23, P-006.PF-24

### ✏️ UC-025 Cross-check with an independent prover (modify)

- Before: With a Rust prover (lastro-core, loaded as WASM in TypeScript or run as the native lastro-prove binary from Go and .NET), prove() tabulates the code over the whole input space and the prover re-checks every clause from that table. The prover drops out-of-domain rows and counts missing inputs and nondeterministic rows (an input that appears with two different outputs). It calls coverage complete when every input of the space has a row and no row is nondeterministic or out of domain; an input repeated with the same output still counts as covered once. It says proved only with complete coverage, checks vacuity, mutants and consistency itself, and returns a SHA-256 digest of the table. The proof is ok only if the SDK report is ok, the prover is ok and both engines give every clause the same verdict.
- After: With a Rust prover (lastro-core, loaded as WASM in TypeScript or run as the native lastro-prove binary from Go and .NET), prove() tabulates the code over the whole input space and the prover re-checks every clause from that table. The prover drops out-of-domain rows and counts missing inputs and nondeterministic rows (an input that appears with two different outputs). It calls coverage complete when every input of the space has a row and no row is nondeterministic or out of domain; an input repeated with the same output still counts as covered once. It says proved only with complete coverage, checks vacuity, mutants and consistency itself, and returns a SHA-256 digest of the table. The proof is ok only if the SDK report is ok, the prover is ok and both engines give every clause the same verdict. For state-machine models the TypeScript engine, the Rust prover, the Go SDK and the .NET SDK visit the states in the same order and give the same verdicts and counterexamples.
- **P-006.I-16** `approved` The TypeScript engine, the Rust prover, the Go SDK and the .NET SDK visit states in the same order and give the same verdicts and counterexamples for state-machine models.
- Gains: P-006.PF-25

### ➕ UC-058 Drive the application (add)

- After: Lastro drives the application through market protocols: web and Electron surfaces over W3C WebDriver (BiDi where supported) with any WebDriver server, Android and iOS surfaces over the same protocol through Appium, api surfaces over HTTP and command surfaces as processes. .lastro/config.json names under drivers the WebDriver or Appium server and its capabilities, the HTTP base URL or the command per surface kind. Only when no standard protocol fits does a driver speak Lastro's JSON lines protocol: Lastro sends a surface and a list of actions and the driver answers with the observed state and the snapshots it took. Lastro ships no browser or device; the docs show a web setup, an Android and iOS setup with Appium, an HTTP setup and a JSON lines driver.
- **P-006.I-17** `approved` Web and Electron surfaces are driven over W3C WebDriver (BiDi where supported) with any WebDriver server, Android and iOS over the same protocol through Appium, api surfaces over HTTP and command surfaces as processes; .lastro/config.json names the server, capabilities, base URL or command under drivers. Lastro's own JSON lines protocol is used only when no standard protocol fits.
- **P-006.I-18** `approved` Lastro ships no browser or device; the docs show a web setup, an Android and iOS setup with Appium, an HTTP setup and a JSON lines driver.
- Gains: P-006.PF-26, P-006.PF-27, P-006.PF-28

### ✏️ UC-021 Run Given/When/Then scenarios (modify)

- Before: Each scenario runs the code on its concrete input and checks its LSL expectation over input and output. It passes or fails, with the output or the error; with no subject every scenario fails. Scenarios are empirical evidence, and assumptions list the scenarios that check them.
- After: A scenario is a list of steps, each an input and the output expected from it: a user action with the expected observed state and, where layout matters, a reference screenshot for a screen; a request with the expected status and body for an api; a model input with its expected output for a pure function, checked by an LSL expectation. A scenario that names a surface runs through its driver; a reference screenshot matches when the share of pixels changed stays under the scenario's tolerance. It passes or fails, with the output or the error; with no subject every scenario fails, and a driver that fails to start or times out fails the scenario with the driver's error. When a reviewer signs off an after screenshot, it becomes the reference screenshot of that step for later regressions. Scenarios are empirical evidence, and assumptions list the scenarios that check them.
- **P-006.I-19** `approved` A scenario is a list of steps, each an input and its expected output: a user action with the expected state and optionally a reference screenshot for a screen, a request with the expected status and body for an api, a model input and output for a pure function. A driven scenario checks each step through its driver; a reference screenshot matches when the share of pixels changed stays under the scenario's tolerance.
- **P-006.I-20** `approved` A driver that fails to start or times out fails the scenario with the driver's error and never passes it.
- **P-006.I-38** `approved` When a reviewer signs off an after screenshot, it becomes the reference screenshot of that scenario step for the regressions that follow.
- Gains: P-006.PF-29, P-006.PF-30, P-006.PF-51

### ➕ UC-059 Check properties over action sequences (add)

- After: A property-based check names a surface, the actions a user or client can take there and an invariant over the observed state. Lastro generates action sequences with a fixed seed, runs each through the driver and checks the invariant after every action. When the surface has a state-machine model, the property is first checked over that model, exhaustively or by SMT induction, and proved there, and the sequences check that the running application follows the model; a property with no model that holds on every sequence is evidenced, with the number of sequences and actions run and the rule-of-three failure bound, never proved. A failing sequence is shortened to its shortest failing prefix and replayed once; if the replay does not fail, the check is reported as nondeterministic instead of violated.
- **P-006.I-21** `approved` A property-based check names a surface, the actions available there and an invariant over the observed state; Lastro generates action sequences with a fixed seed, runs each through the driver and checks the invariant after every action.
- **P-006.I-22** `approved` When the surface has a state-machine model, a property is first checked over that model, exhaustively or by SMT induction, and proved there, with the action sequences checking that the running application follows the model; a property with no model that holds on every sequence is evidenced with the rule-of-three bound, never proved.
- **P-006.I-23** `approved` A failing sequence is shortened to its shortest failing prefix and replayed once; if the replay does not fail, the check is reported as nondeterministic instead of violated.
- Gains: P-006.PF-31, P-006.PF-32, P-006.PF-33, P-006.PF-34, P-006.PF-35, P-006.PF-36

### ➕ UC-060 Snapshot before and after (add)

- After: At extraction every intent gets a before snapshot of each feature it touches, taken on the current code; when the proposal is proved the same snapshot is taken on the new code as the after. A screen snapshot (screen or route surface) is the images the driver captured while running the surface's scenarios, compared as the share of pixels changed. A contract snapshot is the request and response shapes an api or command surface's driver reports as JSON Schema, plus every feature's model inputs and outputs with their domains. A data snapshot is the structure of the data the feature reads or writes (tables and columns, or a JSON Schema of a stored document). Contracts and data are compared by a deterministic structural diff listing every field or column added, removed or changed in type, a removal or type change marked breaking. An examples snapshot is a table of the scenarios' inputs plus a fixed-seed sample of the model's inputs, run on the before and after code with both outputs side by side; the comparison lists the rows whose output changed, and a changed row where no approved intent's clause applies is an unexpected change that must be accepted by a reviewer. Snapshots are kept in .lastro/docs/<proposal>/<feature>/<intent>/before|after/ and committed with the proposal, images included; a whole-repository snapshot lives in .lastro/docs/<proposal>/repository/before|after/; the proposal records each one's SHA-256.
- **P-006.I-24** `approved` Every intent gets a before snapshot of each feature it touches on the current code at extraction, and the same snapshot on the new code when the proposal is proved.
- **P-006.I-25** `approved` For a screen or route surface the snapshot is the images the driver captured while running that surface's scenarios, compared as the share of pixels changed.
- **P-006.I-26** `approved` For an api or command surface the snapshot is the contract the driver reports (request and response shapes as JSON Schema), and for every feature the model's inputs and outputs with their domains.
- **P-006.I-27** `approved` The data snapshot is the structure of the data the feature reads or writes as the driver reports it: tables and columns, or a JSON Schema of a stored document.
- **P-006.I-28** `approved` Contracts and data are compared by a deterministic structural diff listing every field or column added, removed or changed in type; a removal or a type change is marked breaking.
- **P-006.I-29** `approved` The examples snapshot is a table of the scenarios' inputs plus a fixed-seed sample of the model's inputs, each run on the before and the after code with both outputs side by side; the comparison lists the rows whose output changed.
- **P-006.I-30** `approved` An example whose output changed although no approved intent's clause applies to its input is flagged as an unexpected change, and the proposal is not proved while an unexpected change is not accepted by a reviewer.
- **P-006.I-31** `approved` Snapshots are kept in .lastro/docs/<proposal>/<feature>/<intent>/before|after/ and committed with the proposal, images included (the repository's ignore rules let them through); the proposal records the SHA-256 of each so anyone can take it again and compare. A whole-repository snapshot lives in .lastro/docs/<proposal>/repository/before|after/.
- Gains: P-006.PF-37, P-006.PF-38, P-006.PF-39, P-006.PF-40, P-006.PF-41, P-006.PF-42, P-006.PF-43, P-006.PF-44

### ✏️ UC-023 Mark where proof stops (modify)

- Before: Beyond-proof items tie an intent to one of five reasons (external-code, world-facts, termination-timing, human-judgment, unstated) with a note. An intent with beyond-proof items and no useful clause gets the verdict beyond-proof, with notes and any passing scenarios as partial evidence. The receipt lists them under Where proof stops.
- After: Beyond-proof items tie an intent to one of five reasons (external-code, world-facts, termination-timing, human-judgment, unstated) with a note. An intent with beyond-proof items and no useful clause gets the verdict beyond-proof, with notes and any passing scenarios as partial evidence. The receipt lists them under Where proof stops. After the proof run, an intent beyond proof for human judgment waits for a sign-off: a reviewer looks at its before and after snapshots and signs it off or requests changes.
- **P-006.I-32** `approved` An intent beyond proof for human judgment, after the proof run, waits for a reviewer to look at its before and after snapshots and sign it off or request changes.

### ✏️ UC-048 Track a proposal's status (modify)

- Before: A proposal has one status: open while its review is pending; changes-requested while a reviewer has an open thread; approved once approval succeeds; proving once its proofs are derived; proved when every proof, new and regression, passes; applied once apply has synced the use cases; closed when it is abandoned. status lists the proposals with their status, and status <proposal> its pending items.
- After: A proposal has one status: open while its review is pending; changes-requested while a reviewer has an open thread; approved once approval succeeds; proving once its proofs are derived; proved when every proof, new and regression, passes, no intent waits for a sign-off and every unexpected change is accepted (until then it stays proving); applied once apply has synced the use cases; closed when it is abandoned. status lists the proposals with their status, and status <proposal> its pending items.
- **P-006.I-33** `approved` A proposal is not proved while an intent waits for its sign-off or an unexpected change waits to be accepted; it stays proving.
  - replaces P-001.PF-15 (A proposal is open or changes-requested until approved, then approved, proving and proved as its proofs pass; applied and closed are kept.): The lifecycle says proved as soon as the run is ok and every proof passed; a pending sign-off or an unaccepted unexpected change now keeps the proposal in proving.; now guarded by P-006.PF-45
- Gains: P-006.PF-45, P-006.PF-46

### ➕ UC-061 Review the evidence per intent (add)

- After: For each intent the review app shows the feature's place in the feature map (its surfaces, what it depends on and what it affects), the statechart drawn from its state-machine model when it has one, its before snapshots during the review, its before and after snapshots with their comparison after the proof run (images side by side for screens, a diff for contracts and data, the changed rows for examples), and the verdict of every proof and property with its counterexample.
- **P-006.I-34** `approved` For each intent the review app shows the feature's place in the feature map, the statechart drawn from its state-machine model, its before snapshots during review, before and after snapshots with their comparison after the proof run (images side by side, a diff for contracts and data, the changed rows for examples), and every proof and property verdict with its counterexample.
- Gains: P-006.PF-47

### ✏️ UC-035 Use Lastro from TypeScript, .NET or Go (modify)

- Before: Three SDKs expose the same pipeline (propose against the use cases, review with threads, pending, derive, returnDecisions, prove with the regression run, apply, gap review) with the same entities (use case, proposal, intent, proof), wire format, canonical hashes, seeded sampling, verdicts, receipts and prompts. Cross-check tests compare the .NET and Go reports and hashes with the TypeScript SDK, and all three with the Rust prover, on the reference login and its planted bugs.
- After: Three SDKs expose the same pipeline (propose against the use cases, review with threads, pending, derive, returnDecisions, prove with the regression run, apply, gap review) with the same entities (use case, proposal, intent, proof), wire format, canonical hashes, seeded sampling, verdicts, receipts and prompts. Cross-check tests compare the .NET and Go reports and hashes with the TypeScript SDK, and all three with the Rust prover, on the reference login and its planted bugs. Surfaces, the feature map, formal-first derivation with SMT, market-protocol drivers, state-machine proofs with actors, statecharts, action-sequence properties, snapshots, unexpected changes and sign-offs are in all three SDKs and the CLI, with the same files, hashes and verdicts, and the docs describe them.
- **P-006.I-36** `approved` The CLI and the TypeScript, Go and .NET SDKs implement surfaces, the feature map, formal-first derivation with SMT, market-protocol drivers, state-machine proofs with actors, statecharts, action-sequence properties, snapshots, unexpected changes and sign-offs with the same files, hashes and verdicts, and the docs describe them.
- Gains: P-006.PF-49

### ✏️ UC-040 Review an agent's proposal like a pull request (modify)

- Before: The plugin's propose skill has the agent write the change as prose in .lastro/proposals/<id>-<slug>.md, run lastro propose (answering handoffs itself), show the user the review the plugin renders as Markdown (the PostToolUse hook adds it to the agent's context after the first lastro command that shows a new or re-extracted proposal in a session, and a one-line summary after later ones; lastro-plugin.mjs review prints it), then ask the user every open decision and, per intent, shown with the existing proofs it replaces, approve, request changes (amend) or reject, recording only the user's answers with lastro review --as <user>, until lastro status is ready; then lastro derive writes the proposal's proofs. The PreToolUse hook makes every lastro review command that approves, answers, amends or rejects show a Claude Code permission prompt, and denies it in both harnesses when --as names an agent (claude, codex, agent, assistant, ai, bot). With a policy above one approval the board shows each intent's approvals out of the required number and the proposal waits for the other reviewers.
- After: The plugin's propose skill has the agent write the change as prose in .lastro/proposals/<id>-<slug>.md, run lastro propose (answering handoffs itself), show the user the review the plugin renders as Markdown (the PostToolUse hook adds it to the agent's context after the first lastro command that shows a new or re-extracted proposal in a session, and a one-line summary after later ones; lastro-plugin.mjs review prints it), then ask the user every open decision and, per intent, shown with the existing proofs it replaces, approve, request changes (amend) or reject, recording only the user's answers with lastro review --as <user>, until lastro status is ready; then lastro derive writes the proposal's proofs. The PreToolUse hook makes every lastro review command that approves, answers, amends or rejects show a Claude Code permission prompt, and denies it in both harnesses when --as names an agent (claude, codex, agent, assistant, ai, bot). With a policy above one approval the board shows each intent's approvals out of the required number and the proposal waits for the other reviewers. The review also lists the snapshots' paths, the unexpected changes and the sign-offs still due.
- **P-006.I-37** `approved` The plugin's review lists the snapshots' paths, the unexpected changes and the sign-offs still due.
- Gains: P-006.PF-50

## Proofs

- **P-006.PF-01** ✅ proved — A feature file is accepted exactly when it has no surface, or its surfaces have a kind of screen, route, command or api, a name and a locator; dependsOn never makes it invalid. (P-006.I-01, feature_surfaces/feature_surfaces)
  - `ensures (out.accepted = true) ⇔ ((in.surfaces = 0) ∨ ((in.kind ≠ "other") ∧ (in.hasName = true) ∧ (in.hasLocator = true)))`
- **P-006.PF-02** 🟡 evidenced — A feature file written before surfaces existed, with neither surfaces nor dependsOn, is still accepted. (P-006.I-01, feature_surfaces/feature_surfaces)
  - `input {"kind":"other","hasName":false,"hasLocator":false,"surfaces":0,"hasDependsOn":false}; expect out.accepted = true`
- **P-006.PF-03** ✅ proved — For a modified feature the after has the existing surfaces plus the ones added minus the ones removed, and removing more than the feature has is refused. (P-006.I-02, extract_surfaces/extract_surfaces)
  - `requires in.change = "modify"; ensures ((out.accepted = true) ⇔ (in.removes ≤ in.existing)) ∧ ((out.accepted = true) ⇒ (out.after = ((in.existing + in.adds) - in.removes)))`
- **P-006.PF-04** ✅ proved — For a feature an intent adds, the after carries exactly the surfaces and dependencies the extraction proposed. (P-006.I-02, extract_surfaces/extract_surfaces)
  - `requires in.change = "add"; ensures (out.accepted = true) ∧ (out.after = in.proposedOnAdd)`
- **P-006.PF-05** ✅ proved — Printing the feature map twice over the same features gives the same output. (P-006.I-03, feature_map/feature_map)
  - `ensures out.same = true`
- **P-006.PF-06** ✅ proved — In a valid map a feature is listed as reachable by the driver exactly when it has a surface. (P-006.I-03, feature_map/feature_map)
  - `ensures (out.ok = true) ⇒ ((out.aReachable = true) ⇔ (in.aSurface = true))`
- **P-006.PF-07** ✅ proved — The map is accepted exactly when no feature depends on an unknown or removed feature and the dependencies form no cycle. (P-006.I-04, feature_map/feature_map)
  - `ensures (out.ok = true) ⇔ (¬((in.ab = true) ∧ (in.bc = true) ∧ (in.ca = true) ∧ ¬(in.bRemoved = true)) ∧ ¬(in.aUnknown = true) ∧ ¬((in.bRemoved = true) ∧ (in.ab = true)))`
- **P-006.PF-08** ✅ proved — A cycle is reported exactly when A, B and C depend on each other in a ring, and the error names all three. (P-006.I-04, feature_map/feature_map)
  - `ensures ((out.cycle = true) ⇔ ((in.ab = true) ∧ (in.bc = true) ∧ (in.ca = true) ∧ ¬(in.bRemoved = true))) ∧ ((out.cycle = true) ⇒ (out.named = 3))`
- **P-006.PF-09** ✅ proved — With A depending on B, B on C and C on A as given, a feature is affected exactly when it depends, directly or through others, on the changed feature; the changed feature itself is not listed. (P-006.I-05, feature_map/feature_map)
  - `requires (¬((in.ab = true) ∧ (in.bc = true) ∧ (in.ca = true) ∧ ¬(in.bRemoved = true)) ∧ ¬(in.aUnknown = true) ∧ ¬((in.bRemoved = true) ∧ (in.ab = true))) ∧ ¬(in.bRemoved = true); ensures ((out.affectedA = true) ⇔ (((in.changed = "b") ∧ (in.ab = true)) ∨ ((in.changed = "c") ∧ (in.ab = true) ∧ (in.bc = true)))) ∧ ((out.affectedB = true) ⇔ (((in.changed = "c") ∧ (in.bc = true)) ∨ ((in.changed = "a") ∧ (in.bc = true) ∧ (in.ca = true)))) ∧ ((out.affectedC = true) ⇔ (((in.changed = "a") ∧ (in.ca = true)) ∨ ((in.changed = "b") ∧ (in.ca = true) ∧ (in.ab = true))))`
- **P-006.PF-10** ✅ proved — A feature gets a bootstrap intent exactly when it has no surfaces yet. (P-006.I-06, bootstrap_surfaces/bootstrap_surfaces)
  - `ensures (out.proposed = true) ⇔ ¬(in.hasSurfaces = true)`
- **P-006.PF-11** ✅ proved — A bootstrap --surfaces intent is accepted exactly when it adds a surface or dependency and does not touch the description, and every description is kept. (P-006.I-06, bootstrap_surfaces/bootstrap_surfaces)
  - `ensures ((out.accepted = true) ⇔ ((in.addsSurface = true) ∧ ¬(in.touchesDescription = true))) ∧ (out.descriptionKept = true)`
- **P-006.PF-12** ✅ proved — The bootstrap proposal starts open and is reviewed like any other. (P-006.I-06, bootstrap_surfaces/bootstrap_surfaces)
  - `ensures out.reviewed = true`
- **P-006.PF-13** ✅ proved — When the input space or reachable states number at most 100,000, the clause is checked exhaustively: proved when it holds, violated otherwise, with no reason recorded. (P-006.I-07, formal_first/formal_first)
  - `requires (in.space = "small") ∧ (((in.smt = "proves") ⇒ (in.holds = true)) ∧ ((in.smt = "refutes") ⇒ ¬(in.holds = true))); ensures (out.method = "exhaustive") ∧ (out.reason = "none") ∧ ((out.verdict = "proved") ⇔ (in.holds = true)) ∧ ((out.verdict = "violated") ⇔ ¬(in.holds = true))`
- **P-006.PF-14** ❌ violated — When the space is too large and the solver proves or refutes the clause, the method is smt and the verdict is proved or violated accordingly. (P-006.I-07, formal_first/formal_first)
  - `requires (in.space = "large") ∧ ((in.smt = "proves") ∨ (in.smt = "refutes")) ∧ (((in.smt = "proves") ⇒ (in.holds = true)) ∧ ((in.smt = "refutes") ⇒ ¬(in.holds = true))); ensures (out.method = "smt") ∧ (out.reason = "none") ∧ ((out.verdict = "proved") ⇔ (in.smt = "proves")) ∧ ((out.verdict = "violated") ⇔ (in.smt = "refutes"))`
  - Counterexample: `{"holds":true,"smt":"proves","space":"large"}` → `{"method":"smt","verdict":"evidenced","reason":"none"}`
- **P-006.PF-15** ✅ proved — When the space is too large and the solver cannot decide or express the clause, it is sampled, the reason is solver-unknown or not-expressible, and the verdict is never proved. (P-006.I-07, formal_first/formal_first)
  - `requires (in.space = "large") ∧ ((in.smt = "unknown") ∨ (in.smt = "inexpressible")) ∧ (((in.smt = "proves") ⇒ (in.holds = true)) ∧ ((in.smt = "refutes") ⇒ ¬(in.holds = true))); ensures (out.method = "sampled") ∧ (out.verdict ≠ "proved") ∧ ((in.smt = "unknown") ⇒ (out.reason = "solver-unknown")) ∧ ((in.smt = "inexpressible") ⇒ (out.reason = "not-expressible")) ∧ ((in.holds = true) ⇒ (out.verdict = "evidenced"))`
- **P-006.PF-16** ✅ proved — The counts equal the number of intents proved formally, evidenced and beyond proof, and there is one reason per evidenced or beyond-proof intent. (P-006.I-08, formal_coverage/formal_coverage)
  - `ensures (out.countFormal = in.formal) ∧ (out.countEvidenced = in.evidenced) ∧ (out.countBeyond = in.beyond) ∧ (out.reasons = (in.evidenced + in.beyond))`
- **P-006.PF-17** ✅ proved — A state-machine model is accepted exactly when its state variables have finite domains, its initial state is in them and its events have finite inputs, with or without actors. (P-006.I-09, sm_model/sm_model)
  - `ensures (out.accepted = true) ⇔ ((in.stateFinite = true) ∧ (in.initialInDomain = true) ∧ (in.eventsFinite = true))`
- **P-006.PF-18** ✅ proved — A shared counter that actors increment by reading and then writing is violated exactly when two or more actors run without an atomic step, and the counterexample uses two actors. (P-006.I-10, sm_interleave/sm_interleave)
  - `ensures ((out.violated = true) ⇔ ((in.actors ≥ 2) ∧ ¬(in.atomic = true))) ∧ ((out.violated = true) ⇒ (out.cexActors = 2))`
- **P-006.PF-19** ✅ proved — Over a state space small enough to visit, each kind of clause is proved when it holds (for a reachability clause, when the state is reached) and violated otherwise. (P-006.I-11, sm_clauses/sm_clauses)
  - `requires in.space = "small"; ensures ((out.verdict = "proved") ⇔ (in.holds = true)) ∧ ((out.verdict = "violated") ⇔ ¬(in.holds = true))`
- **P-006.PF-20** ✅ proved — When the reachable states number at most 100,000 the method is breadth-first search, every state is counted, and a clause that holds is proved with formal evidence. (P-006.I-12, sm_clauses/sm_clauses)
  - `requires in.space = "small"; ensures (out.method = "bfs") ∧ (out.states > 0) ∧ ((in.holds = true) ⇒ ((out.verdict = "proved") ∧ (out.evidence = "formal")))`
- **P-006.PF-21** ✅ proved — For an invariant or step clause over a space too large to visit, a solver that proves the induction gives proved with formal evidence and the smt-induction method named. (P-006.I-13, sm_clauses/sm_clauses)
  - `requires (in.space = "large") ∧ (in.smt = "proves") ∧ (in.clause ≠ "reach") ∧ ¬((in.smt = "proves") ∧ ¬(in.holds = true)); ensures (out.verdict = "proved") ∧ (out.evidence = "formal") ∧ (out.method = "smt-induction")`
- **P-006.PF-22** ❌ violated — For an invariant or step clause over a large space that the solver cannot decide, the method is random walks with a fixed seed and the verdict is never proved; a clause that holds is evidenced. (P-006.I-13, sm_clauses/sm_clauses)
  - `requires (in.space = "large") ∧ (in.smt = "unknown") ∧ (in.clause ≠ "reach"); ensures (out.method = "random-walk") ∧ (out.verdict ≠ "proved") ∧ ((in.holds = true) ⇒ ((out.verdict = "evidenced") ∧ (out.evidence = "empirical")))`
  - Counterexample: `{"clause":"step","holds":false,"smt":"unknown","space":"large"}` → `{"verdict":"violated","evidence":"counterexample","method":"bfs","states":2,"shortest":true,"actorsNamed":true}`
- **P-006.PF-23** ✅ proved — A violated invariant or step clause over a visited state space comes back with the shortest sequence of events from the initial state, each naming its actor. (P-006.I-14, sm_clauses/sm_clauses)
  - `requires (in.space = "small") ∧ ¬(in.holds = true) ∧ (in.clause ≠ "reach"); ensures (out.verdict = "violated") ∧ (out.evidence = "counterexample") ∧ (out.shortest = true) ∧ (out.actorsNamed = true)`
- **P-006.PF-24** ✅ proved — The statechart has one parallel region per actor, labels every transition with its event, marks the states an invariant forbids exactly when there are some, and draws the counterexample path exactly when there is one. (P-006.I-15, statechart/statechart)
  - `ensures (out.regions = in.actors) ∧ (out.labelled = true) ∧ ((out.marked = true) ⇔ (in.forbids = true)) ∧ ((out.pathDrawn = true) ⇔ (in.cex = true))`
- **P-006.PF-25** ✅ proved — For correct and buggy state machines, the Rust prover, the Go SDK and the .NET SDK visit the same number of states and give the same verdicts and counterexamples as the TypeScript engine. (P-006.I-16, sm_crosscheck/sm_crosscheck)
  - `ensures (out.sameVerdict = true) ∧ (out.sameCounterexample = true) ∧ (out.sameStates = true)`
- **P-006.PF-26** ✅ proved — With a standard driver configured, web and Electron surfaces use WebDriver, Android and iOS use Appium, api surfaces use HTTP and command surfaces run as a process. (P-006.I-17, driver_protocol/driver_protocol)
  - `requires in.config = "standard"; ensures (((in.target = "web") ∨ (in.target = "electron")) ⇒ (out.protocol = "webdriver")) ∧ (((in.target = "android") ∨ (in.target = "ios")) ⇒ (out.protocol = "appium")) ∧ ((in.target = "api") ⇒ (out.protocol = "http")) ∧ ((in.target = "command") ⇒ (out.protocol = "process"))`
- **P-006.PF-27** ✅ proved — The protocol is JSON lines exactly when the configuration names a JSON lines driver, and none when no driver is configured. (P-006.I-17, driver_protocol/driver_protocol)
  - `ensures ((out.protocol = "jsonlines") ⇔ (in.config = "jsonlines")) ∧ ((out.protocol = "none") ⇔ (in.config = "none"))`
- **P-006.PF-28** ✅ proved — No SDK package depends on a browser or device, and the docs have a web, an Appium, an HTTP and a JSON lines setup. (P-006.I-18, shipped_drivers/shipped_drivers)
  - `ensures (out.bundlesBrowser = false) ∧ (out.documented = true)`
- **P-006.PF-29** ✅ proved — With a driver that runs, a scenario step passes exactly when its observed output matches the expected one and, for a screen step with a reference screenshot, the share of pixels changed is at most the tolerance; a pass counts as empirical evidence. (P-006.I-19, driven_scenario/driven_scenario)
  - `requires in.driver = "ok"; ensures ((out.passed = true) ⇔ ((in.matches = true) ∧ ((in.step ≠ "screen") ∨ ¬(in.hasReference = true) ∨ (in.pixelShare ≤ in.tolerance)))) ∧ ((out.passed = true) ⇒ (out.evidence = "empirical")) ∧ (out.driverError = false)`
- **P-006.PF-30** ✅ proved — When the driver fails to start or times out, the scenario fails with the driver's error and is never passed. (P-006.I-20, driven_scenario/driven_scenario)
  - `requires in.driver ≠ "ok"; ensures (out.passed = false) ∧ (out.driverError = true) ∧ (out.evidence = "none")`
- **P-006.PF-31** ✅ proved — Running the same property-based check twice generates the same action sequences. (P-006.I-21, action_property/action_property)
  - `ensures out.sameSequences = true`
- **P-006.PF-32** 🟡 evidenced — A property checked through the driver, with no state-machine model, that holds after every action of every sequence is evidenced with the rule-of-three bound. (P-006.I-21, action_property/action_property)
  - `input {"hasModel":false,"modelHolds":false,"sequencesHold":true,"replayFails":false}; expect (out.verdict = "evidenced") ∧ (out.evidence = "empirical") ∧ (out.bound = true)`
- **P-006.PF-33** ✅ proved — With a model, a property that holds over the model and on every driven sequence is proved formally; one that the model violates is violated. (P-006.I-22, action_property/action_property)
  - `requires in.hasModel = true; ensures (((in.modelHolds = true) ∧ (in.sequencesHold = true)) ⇒ ((out.verdict = "proved") ∧ (out.evidence = "formal"))) ∧ (¬(in.modelHolds = true) ⇒ (out.verdict = "violated"))`
- **P-006.PF-34** ✅ proved — Without a model, a property that holds on every sequence is evidenced with the rule-of-three bound, never proved. (P-006.I-22, action_property/action_property)
  - `requires ¬(in.hasModel = true); ensures (out.verdict ≠ "proved") ∧ ((in.sequencesHold = true) ⇒ ((out.verdict = "evidenced") ∧ (out.bound = true)))`
- **P-006.PF-35** ✅ proved — When a driven sequence fails, the check is violated exactly when the replay of its shortened prefix fails again, and nondeterministic exactly when it does not. (P-006.I-23, action_property/action_property)
  - `requires ¬(in.sequencesHold = true) ∧ (¬(in.hasModel = true) ∨ (in.modelHolds = true)); ensures ((out.verdict = "violated") ⇔ (in.replayFails = true)) ∧ ((out.verdict = "nondeterministic") ⇔ ¬(in.replayFails = true))`
- **P-006.PF-36** ✅ proved — A sequence that first fails at action k is shortened to its first k actions; one that never fails has no prefix. (P-006.I-23, shrink_sequence/shrink_sequence)
  - `ensures ((in.failAt ≤ in.length) ⇒ (out.prefix = in.failAt)) ∧ ((in.failAt > in.length) ⇒ (out.prefix = 0))`
- **P-006.PF-37** ✅ proved — A snapshot taken at extraction is a before snapshot, and one taken at the proof run is an after snapshot. (P-006.I-24, snapshot_kinds/snapshot_kinds)
  - `ensures (out.side = "before") ⇔ (in.phase = "extract")`
- **P-006.PF-38** ✅ proved — A feature gets a screen snapshot exactly when its surface is a screen or a route. (P-006.I-25, snapshot_kinds/snapshot_kinds)
  - `ensures (out.screen = true) ⇔ ((in.surface = "screen") ∨ (in.surface = "route"))`
- **P-006.PF-39** ✅ proved — A feature gets a contract snapshot exactly when its surface is an api or a command, or it has a model. (P-006.I-26, snapshot_kinds/snapshot_kinds)
  - `ensures (out.contract = true) ⇔ ((in.surface = "api") ∨ (in.surface = "command") ∨ (in.hasModel = true))`
- **P-006.PF-40** ✅ proved — A feature gets a data snapshot exactly when its driver reports the structure of the data it reads or writes. (P-006.I-27, snapshot_kinds/snapshot_kinds)
  - `ensures (out.data = true) ⇔ (in.hasData = true)`
- **P-006.PF-41** ✅ proved — A field present only after is added, present only before is removed, present in both with different types is a type change, and otherwise unchanged; removals and type changes are breaking. (P-006.I-28, structural_diff/structural_diff)
  - `ensures ((out.change = "none") ⇔ (in.before = in.after)) ∧ ((out.change = "added") ⇔ ((in.before = "absent") ∧ (in.after ≠ "absent"))) ∧ ((out.change = "removed") ⇔ ((in.after = "absent") ∧ (in.before ≠ "absent"))) ∧ ((out.breaking = true) ⇔ ((out.change = "removed") ∨ (out.change = "type")))`
- **P-006.PF-42** ✅ proved — The examples snapshot runs the whole model space when it has at most 1,000 points, and the comparison lists a row exactly when its output changed. (P-006.I-29, examples_compare/examples_compare)
  - `ensures ((out.wholeSpace = true) ⇔ (in.points = "small")) ∧ ((out.listed = true) ⇔ (in.changed = true))`
- **P-006.PF-43** ✅ proved — An example is an unexpected change exactly when its output changed and no approved clause applies to its input, and it blocks the proposal until the policy's number of reviewers accept it. (P-006.I-30, examples_compare/examples_compare)
  - `ensures ((out.unexpected = true) ⇔ ((in.changed = true) ∧ ¬(in.clauseApplies = true))) ∧ ((out.blocks = true) ⇔ ((out.unexpected = true) ∧ (in.acceptances < in.required)))`
- **P-006.PF-44** ✅ proved — Every snapshot file lands in .lastro/docs/<proposal>/<feature>/<intent>/before|after/ (or repository/before|after/), is not ignored by git, and its recorded SHA-256 matches the file. (P-006.I-31, snapshot_files/snapshot_files)
  - `ensures (out.layoutOk = true) ∧ (out.ignored = false) ∧ (out.hashRecorded = true) ∧ (out.hashMatches = true)`
- **P-006.PF-45** ✅ proved — After an ok run where every proof passed, the proposal is changes-requested when a reviewer requested changes on a human-judgment intent, proved when every due sign-off and acceptance has the policy's number of reviewers, and proving otherwise. (P-006.I-33, proved_gate/proved_gate)
  - `requires (in.runOk = true) ∧ (in.allPassed = true); ensures ((out.status = "changes-requested") ⇔ ((in.judgment = true) ∧ (in.changesRequested = true))) ∧ ((out.status = "proved") ⇔ (((¬(in.judgment = true) ∨ (in.signoffs ≥ in.required)) ∧ (¬(in.unexpected = true) ∨ (in.acceptances ≥ in.required))) ∧ ¬((in.judgment = true) ∧ (in.changesRequested = true))))`
- **P-006.PF-46** ✅ proved — Without an ok run where every proof passed, the proposal is not proved, whatever sign-offs it has. (P-006.I-33, proved_gate/proved_gate)
  - `requires ¬((in.runOk = true) ∧ (in.allPassed = true)); ensures out.status ≠ "proved"`
- **P-006.PF-47** ✅ proved — For each intent the review app shows its place in the feature map, the statechart when there is a model, the before snapshot, the after snapshot only after the proof run with the comparison for its kind, and a counterexample exactly when a verdict is violated. (P-006.I-34, review_evidence/review_evidence)
  - `ensures (out.mapShown = true) ∧ ((out.statechart = true) ⇔ (in.machine = true)) ∧ (out.before = true) ∧ ((out.after = true) ⇔ (in.phase = "proved")) ∧ ((in.phase = "review") ⇒ (out.comparison = "none")) ∧ (((in.phase = "proved") ∧ (in.kind = "screen")) ⇒ (out.comparison = "images")) ∧ (((in.phase = "proved") ∧ ((in.kind = "contract") ∨ (in.kind = "data"))) ⇒ (out.comparison = "diff")) ∧ (((in.phase = "proved") ∧ (in.kind = "examples")) ⇒ (out.comparison = "rows")) ∧ ((out.counterexample = true) ⇔ (in.violated = true))`
- **P-006.PF-48** ✅ proved — The receipt has a Feature map section exactly when features are affected, a Mermaid diagram exactly when there is a state-machine model, snapshot hashes exactly when there are snapshots, who accepted exactly when an unexpected change was accepted, and who signed off exactly when there is a sign-off. (P-006.I-35, receipt_evidence/receipt_evidence)
  - `ensures ((out.mapSection = true) ⇔ (in.affected = true)) ∧ ((out.mermaid = true) ⇔ (in.machine = true)) ∧ ((out.hashes = true) ⇔ (in.snapshots = true)) ∧ ((out.acceptedBy = true) ⇔ (in.unexpected = "accepted")) ∧ ((out.signedBy = true) ⇔ (in.signoff = true))`
- **P-006.PF-49** ✅ proved — For the same workspace, the Go and .NET SDKs give byte-identical feature maps, statecharts, structural diffs, verdicts and snapshot hashes to the TypeScript SDK. (P-006.I-36, sdk_parity/sdk_parity)
  - `ensures out.same = true`
- **P-006.PF-50** ✅ proved — The plugin's review lists the snapshot paths exactly when there are snapshots, the unexpected changes exactly when there are some, and the sign-offs still due exactly when there are some. (P-006.I-37, plugin_review_evidence/plugin_review_evidence)
  - `ensures ((out.listsPaths = true) ⇔ (in.snapshots = true)) ∧ ((out.listsUnexpected = true) ⇔ (in.unexpected = true)) ∧ ((out.listsSignoffs = true) ⇔ (in.signoffDue = true))`
- **P-006.PF-51** ✅ proved — A step's reference screenshot becomes its after screenshot exactly when a reviewer signed that screenshot off, whether or not it had a reference before. (P-006.I-38, reference_screenshot/reference_screenshot)
  - `ensures (out.referenceIsAfter = true) ⇔ (in.signedOff = true)`

## Regression

137 use-case proof(s) ran; 0 failed. Engines agree: yes.

- ⚠️ **P-001.PF-15** is superseded but still passes: the behavior it guarded may not have changed.
- Unproved use cases (no proofs yet): UC-033, UC-038, UC-045

## Snapshots

- after examples `docs/P-006/UC-001/I-02/after/examples-extract_surfaces.json` sha256:b04d903a6bfed42129c585bedb08fb42289c5c7d3c1b93b0559a3e036d44e612
- after examples `docs/P-006/UC-001/I-02/after/examples-extraction_rules.json` sha256:f6ba77427af9eec0ad080cefdaa269fff34f43c5cee2bba714ebe9a235307a58
- after contract `docs/P-006/UC-001/I-02/after/model-extract_surfaces.json` sha256:8a9ae2bc5b3a1c5def960dc366728a2c28242007a38976b1e01537b4e347d520
- after contract `docs/P-006/UC-001/I-02/after/model-extraction_rules.json` sha256:1c951ef34d72dd1b3d0d35ba95521a6d021c5970b182ac068d1ca4887bbceee1
- after examples `docs/P-006/UC-011/I-09/after/examples-domain_kinds.json` sha256:34472ac394855c72264f1b9d4caa82372c24ad43c82b68ff0bf14a50933d8930
- after examples `docs/P-006/UC-011/I-09/after/examples-sm_model.json` sha256:a8d784356a89c89d6844bb1e3d5fc964580d8bfcb7e24c663c354420a8b2436d
- after contract `docs/P-006/UC-011/I-09/after/model-domain_kinds.json` sha256:18d3fdf8496f1ff3c338c72db3c9fd466b9e9ab496288fffcaa09079b2163927
- after contract `docs/P-006/UC-011/I-09/after/model-sm_model.json` sha256:e882367430ad4d25089803380b9ecb9024128c898756859f7d4e23a87854e7df
- after examples `docs/P-006/UC-013/I-07/after/examples-formal_first.json` sha256:166f287aaec03221cb186bba3ceaec520d889c7491968d57387783d90e1cd985
- after examples `docs/P-006/UC-013/I-07/after/examples-supersedes.json` sha256:b755b0f39875126ef5bcd9b85c6a7303ab3cf662cd8408aa03ae13815ae49b08
- after contract `docs/P-006/UC-013/I-07/after/model-formal_first.json` sha256:9bdff6d46d2389ecdfe105fb55745f843d18a4a43a5b122692ac9b9291ef2903
- after contract `docs/P-006/UC-013/I-07/after/model-supersedes.json` sha256:cfaf69deab351c511bdbc9e2543fe1336dd0190710ee2e0f6a0c8fe14e8875dd
- after examples `docs/P-006/UC-021/I-19/after/examples-driven_scenario.json` sha256:0864f7aa376a4914791439ce459036453c140a6ae4323effbcc5f1408b4abfed
- after examples `docs/P-006/UC-021/I-19/after/examples-reference_screenshot.json` sha256:daf0254ecadf7c3aeee2f40fcd74fd0e3bd06b17a35487ce3024446af87c3db5
- after examples `docs/P-006/UC-021/I-19/after/examples-scenario_run.json` sha256:bc18557e265b4ef951240cd98393b4d5c5a346f2b71fbc162ca87c2a3495c53d
- after contract `docs/P-006/UC-021/I-19/after/model-driven_scenario.json` sha256:65dd88071d098e1fcc2de79f61adbf860ac97a513c92db1e2560e5289c6e34f7
- after contract `docs/P-006/UC-021/I-19/after/model-reference_screenshot.json` sha256:6269d0c326e01b4e412b2b22ebe6b6104c5b1c266958a6dc1880b830803e393d
- after contract `docs/P-006/UC-021/I-19/after/model-scenario_run.json` sha256:a3944eaabd727106877ac8ba8bc39d36812b5ebd143992d68215592c098226b3
- after examples `docs/P-006/UC-021/I-20/after/examples-driven_scenario.json` sha256:0864f7aa376a4914791439ce459036453c140a6ae4323effbcc5f1408b4abfed
- after examples `docs/P-006/UC-021/I-20/after/examples-reference_screenshot.json` sha256:daf0254ecadf7c3aeee2f40fcd74fd0e3bd06b17a35487ce3024446af87c3db5
- after examples `docs/P-006/UC-021/I-20/after/examples-scenario_run.json` sha256:bc18557e265b4ef951240cd98393b4d5c5a346f2b71fbc162ca87c2a3495c53d
- after contract `docs/P-006/UC-021/I-20/after/model-driven_scenario.json` sha256:65dd88071d098e1fcc2de79f61adbf860ac97a513c92db1e2560e5289c6e34f7
- after contract `docs/P-006/UC-021/I-20/after/model-reference_screenshot.json` sha256:6269d0c326e01b4e412b2b22ebe6b6104c5b1c266958a6dc1880b830803e393d
- after contract `docs/P-006/UC-021/I-20/after/model-scenario_run.json` sha256:a3944eaabd727106877ac8ba8bc39d36812b5ebd143992d68215592c098226b3
- after examples `docs/P-006/UC-021/I-38/after/examples-driven_scenario.json` sha256:0864f7aa376a4914791439ce459036453c140a6ae4323effbcc5f1408b4abfed
- after examples `docs/P-006/UC-021/I-38/after/examples-reference_screenshot.json` sha256:daf0254ecadf7c3aeee2f40fcd74fd0e3bd06b17a35487ce3024446af87c3db5
- after examples `docs/P-006/UC-021/I-38/after/examples-scenario_run.json` sha256:bc18557e265b4ef951240cd98393b4d5c5a346f2b71fbc162ca87c2a3495c53d
- after contract `docs/P-006/UC-021/I-38/after/model-driven_scenario.json` sha256:65dd88071d098e1fcc2de79f61adbf860ac97a513c92db1e2560e5289c6e34f7
- after contract `docs/P-006/UC-021/I-38/after/model-reference_screenshot.json` sha256:6269d0c326e01b4e412b2b22ebe6b6104c5b1c266958a6dc1880b830803e393d
- after contract `docs/P-006/UC-021/I-38/after/model-scenario_run.json` sha256:a3944eaabd727106877ac8ba8bc39d36812b5ebd143992d68215592c098226b3
- after examples `docs/P-006/UC-023/I-32/after/examples-intent_rollup.json` sha256:53e8e37ec0cda450d116f9087024ad693876f931163895c62a40073af42242f2
- after contract `docs/P-006/UC-023/I-32/after/model-intent_rollup.json` sha256:bd6b3e0c6e7643965bed4c2bcf87f288198266cedbf9bac109fa316918b21e27
- after examples `docs/P-006/UC-025/I-16/after/examples-crosscheck.json` sha256:ebcc5b2108f216ce6b44fc465bed10be4795ead45ff3143f3f9240a321eae62c
- after examples `docs/P-006/UC-025/I-16/after/examples-sm_crosscheck.json` sha256:6c8a6479049670a8ba05cfd993ec909df2d37a18216eefe264d78a13e91fe6eb
- after contract `docs/P-006/UC-025/I-16/after/model-crosscheck.json` sha256:973ebfd5516dc694169bf10286ba8e6eb58fc4c2dba4e83d9a784a92527c3436
- after contract `docs/P-006/UC-025/I-16/after/model-sm_crosscheck.json` sha256:577d3d42409c672858839fb0f41ac6341325d080a03ab8ed662cc54f86bf7fa5
- after examples `docs/P-006/UC-026/I-08/after/examples-formal_coverage.json` sha256:72b02104929e18a0ebd0874b153bedf47d872bed425eb3f58604399bb0387371
- after examples `docs/P-006/UC-026/I-08/after/examples-receipt_evidence.json` sha256:33e516b468ef921882ba01f02494692b3c45446100836355e36ae20c54b0ad0f
- after examples `docs/P-006/UC-026/I-08/after/examples-receipt_sections.json` sha256:06dc80f770c9695b90e25f0e1eeff72a20783b104e5826b8522413525e906400
- after contract `docs/P-006/UC-026/I-08/after/model-formal_coverage.json` sha256:8c478d0b9b602a0fe9eb8bb356c87ab49969827d333f1e6fd37e0584108a87dc
- after contract `docs/P-006/UC-026/I-08/after/model-receipt_evidence.json` sha256:34391badc732a4427e4a71d5c00b4beb51175acdf718dfe381584c341714d06a
- after contract `docs/P-006/UC-026/I-08/after/model-receipt_sections.json` sha256:1f69f9e9efa7a4dbf15339e3ef04f159036a4cf9a8390c1f709207a678339542
- after examples `docs/P-006/UC-026/I-35/after/examples-formal_coverage.json` sha256:72b02104929e18a0ebd0874b153bedf47d872bed425eb3f58604399bb0387371
- after examples `docs/P-006/UC-026/I-35/after/examples-receipt_evidence.json` sha256:33e516b468ef921882ba01f02494692b3c45446100836355e36ae20c54b0ad0f
- after examples `docs/P-006/UC-026/I-35/after/examples-receipt_sections.json` sha256:06dc80f770c9695b90e25f0e1eeff72a20783b104e5826b8522413525e906400
- after contract `docs/P-006/UC-026/I-35/after/model-formal_coverage.json` sha256:8c478d0b9b602a0fe9eb8bb356c87ab49969827d333f1e6fd37e0584108a87dc
- after contract `docs/P-006/UC-026/I-35/after/model-receipt_evidence.json` sha256:34391badc732a4427e4a71d5c00b4beb51175acdf718dfe381584c341714d06a
- after contract `docs/P-006/UC-026/I-35/after/model-receipt_sections.json` sha256:1f69f9e9efa7a4dbf15339e3ef04f159036a4cf9a8390c1f709207a678339542
- after examples `docs/P-006/UC-035/I-36/after/examples-sdk_parity.json` sha256:b9c026dcd655f3e7e8c05563d7b46c68359594378b5f53a7c6865ee946b336e4
- after contract `docs/P-006/UC-035/I-36/after/model-sdk_parity.json` sha256:ff0e20e54b41bac869676c89ba755e65c874c1f6744721934ed4f8fed3a50ca2
- after examples `docs/P-006/UC-040/I-37/after/examples-plugin_review_evidence.json` sha256:9bd29d0424c16be3244d93c8d7b48552023ad73e97f3800eab2e940e072d7fc7
- after examples `docs/P-006/UC-040/I-37/after/examples-review_guard.json` sha256:75135bbfca5c65640ffc610ae6054ad1fb720bfccec415deb29b35f170e553bc
- after contract `docs/P-006/UC-040/I-37/after/model-plugin_review_evidence.json` sha256:7d1eb4a231803d9cb40d93f00d147f945697390e55a4ef8ce02e1e298f393122
- after contract `docs/P-006/UC-040/I-37/after/model-review_guard.json` sha256:7814070f052492a22c7040acba8a7b3f87950d33ff0110e8aa2135306bbaa7d1
- after examples `docs/P-006/UC-046/I-01/after/examples-feature_surfaces.json` sha256:5ae7cb918745773c408773950fed3eed326e1aaee00a927d7fba83efbd61649d
- after examples `docs/P-006/UC-046/I-01/after/examples-next_use_case_id.json` sha256:d4ba6763d4d04bf4637451b385698304b78ffb2a1087befc9cb00547889ccd8d
- after examples `docs/P-006/UC-046/I-01/after/examples-proof_files.json` sha256:ee5943f28cbdcdd234277ff1051bb4ef428dc1aa55aa856368e897bae62e3db3
- after examples `docs/P-006/UC-046/I-01/after/examples-slug_check.json` sha256:ed9e94ac7ae825637952739df1321683bff99335322d5634fafa3ee1ca53ad5c
- after contract `docs/P-006/UC-046/I-01/after/model-feature_surfaces.json` sha256:316d475bc7cbbfa3547b7a9941ecfe713a0cf5d94714f859058e3bc4d7fe77b4
- after contract `docs/P-006/UC-046/I-01/after/model-next_use_case_id.json` sha256:5107c0822786c29b610d9e0b01a7cc1bd576ba2ce173faccd1f58cc9595101ae
- after contract `docs/P-006/UC-046/I-01/after/model-proof_files.json` sha256:84d405bac87f027e31a625f211d12e1e1359c27b2ad7b79f9df2604d0fad81c5
- after contract `docs/P-006/UC-046/I-01/after/model-slug_check.json` sha256:92aabcfe103e382885ca17b70a3afc75bcc47ba1b061905cfb23973d73d983e1
- after examples `docs/P-006/UC-048/I-33/after/examples-proposal_status.json` sha256:1009ec88d135238c88576d2adbc61ffe4f16f83094b4499becfcde8a30175e29
- after examples `docs/P-006/UC-048/I-33/after/examples-proved_gate.json` sha256:2f64c2d07aca47238b9ef610f9e4f3cf9bc82212319ed768aa6c485873d92db7
- after contract `docs/P-006/UC-048/I-33/after/model-proposal_status.json` sha256:5c46a9c69997af8d49289a5b0a88a8634ba6c05015ca98bb08a9048ae9f5000b
- after contract `docs/P-006/UC-048/I-33/after/model-proved_gate.json` sha256:3744d5e3ff270effb9a5e490c0a1bc76742f9e9d076cc2ad32773f1369ae9166
- after examples `docs/P-006/UC-055/I-03/after/examples-feature_map.json` sha256:2ad5365a587ccaefac2a9b41ab896663e0496fe1bd717099f1b02d8bb8b7d3bf
- after contract `docs/P-006/UC-055/I-03/after/model-feature_map.json` sha256:a0b1c3dd2384d44efb8c884d7b46ce6a637e11a485c5804f8acd8416bebe83af
- after examples `docs/P-006/UC-055/I-04/after/examples-feature_map.json` sha256:2ad5365a587ccaefac2a9b41ab896663e0496fe1bd717099f1b02d8bb8b7d3bf
- after contract `docs/P-006/UC-055/I-04/after/model-feature_map.json` sha256:a0b1c3dd2384d44efb8c884d7b46ce6a637e11a485c5804f8acd8416bebe83af
- after examples `docs/P-006/UC-055/I-05/after/examples-feature_map.json` sha256:2ad5365a587ccaefac2a9b41ab896663e0496fe1bd717099f1b02d8bb8b7d3bf
- after contract `docs/P-006/UC-055/I-05/after/model-feature_map.json` sha256:a0b1c3dd2384d44efb8c884d7b46ce6a637e11a485c5804f8acd8416bebe83af
- after examples `docs/P-006/UC-056/I-06/after/examples-bootstrap_surfaces.json` sha256:444993e39268ae84d9e002f2a53a729dce22f9d44501b98ec939291cbdc851ec
- after contract `docs/P-006/UC-056/I-06/after/model-bootstrap_surfaces.json` sha256:a7ac25263c97c4ec45c9419cf67bc82005772ad5ae8ead982d7398b610f6958a
- after examples `docs/P-006/UC-057/I-10/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-10/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-10/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-10/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-10/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-10/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-057/I-11/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-11/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-11/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-11/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-11/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-11/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-057/I-12/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-12/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-12/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-12/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-12/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-12/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-057/I-13/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-13/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-13/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-13/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-13/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-13/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-057/I-14/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-14/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-14/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-14/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-14/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-14/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-057/I-15/after/examples-sm_clauses.json` sha256:67c372f8bd1094b07d63d87db9b74201bd9d70dbab565948b56fa73e43d7ddee
- after examples `docs/P-006/UC-057/I-15/after/examples-sm_interleave.json` sha256:f69a714ead2de9ba7ed900ac9e0fd2669a78ce685990bcff72653beef96c90bb
- after examples `docs/P-006/UC-057/I-15/after/examples-statechart.json` sha256:42f1ccfdfdcb339236bfc682a7c4488c4cffc075b4bfced5289a51606a14e673
- after contract `docs/P-006/UC-057/I-15/after/model-sm_clauses.json` sha256:157591db4d2eced383a2556f66a9241edefd7c86716916de1e7293fa2ada5560
- after contract `docs/P-006/UC-057/I-15/after/model-sm_interleave.json` sha256:8a54d5368b2f66bba1047814be9f4dc9b743d414219e5b6dcbc1562d5d3e912f
- after contract `docs/P-006/UC-057/I-15/after/model-statechart.json` sha256:033ef8f7d87330ce31b77b8755d0f468a85e925bab76b3d06b8970d3220456e3
- after examples `docs/P-006/UC-058/I-17/after/examples-driver_protocol.json` sha256:277ce91a3c59488a4d6328d99238056b6a3ba8666869854b293ae30e4e6ade50
- after examples `docs/P-006/UC-058/I-17/after/examples-shipped_drivers.json` sha256:8a3c4fc03abe04c29ff8430438b1841f6b2e5739ea74980411d66f6ffd0d675f
- after contract `docs/P-006/UC-058/I-17/after/model-driver_protocol.json` sha256:e8627d424eb5401f76892b7c74f2ffa3724b3ef2ed44d93eb727c1f167a21827
- after contract `docs/P-006/UC-058/I-17/after/model-shipped_drivers.json` sha256:88d1655bb7fc5dbea9037bca7b7b2f293b0803260cd2e40e57cb91f6af393001
- after examples `docs/P-006/UC-058/I-18/after/examples-driver_protocol.json` sha256:277ce91a3c59488a4d6328d99238056b6a3ba8666869854b293ae30e4e6ade50
- after examples `docs/P-006/UC-058/I-18/after/examples-shipped_drivers.json` sha256:8a3c4fc03abe04c29ff8430438b1841f6b2e5739ea74980411d66f6ffd0d675f
- after contract `docs/P-006/UC-058/I-18/after/model-driver_protocol.json` sha256:e8627d424eb5401f76892b7c74f2ffa3724b3ef2ed44d93eb727c1f167a21827
- after contract `docs/P-006/UC-058/I-18/after/model-shipped_drivers.json` sha256:88d1655bb7fc5dbea9037bca7b7b2f293b0803260cd2e40e57cb91f6af393001
- after examples `docs/P-006/UC-059/I-21/after/examples-action_property.json` sha256:34c3f09f76d1e3c1e8f4d4bbfcc856d9313e5548b22f20cdb3fa1e331200d204
- after examples `docs/P-006/UC-059/I-21/after/examples-shrink_sequence.json` sha256:2981b955afe10ccf8a7c0a270ba52c646045cd1c1356edfd6d9d153fc9af7f25
- after contract `docs/P-006/UC-059/I-21/after/model-action_property.json` sha256:e23e22cb7da8347f8ab7e464e0aaabedce5dfb2668399258947eeaf6782e3c42
- after contract `docs/P-006/UC-059/I-21/after/model-shrink_sequence.json` sha256:46badf323fd321890b129250d2232b780c864922f4c15bdbf0f829d23d5c7b09
- after examples `docs/P-006/UC-059/I-22/after/examples-action_property.json` sha256:34c3f09f76d1e3c1e8f4d4bbfcc856d9313e5548b22f20cdb3fa1e331200d204
- after examples `docs/P-006/UC-059/I-22/after/examples-shrink_sequence.json` sha256:2981b955afe10ccf8a7c0a270ba52c646045cd1c1356edfd6d9d153fc9af7f25
- after contract `docs/P-006/UC-059/I-22/after/model-action_property.json` sha256:e23e22cb7da8347f8ab7e464e0aaabedce5dfb2668399258947eeaf6782e3c42
- after contract `docs/P-006/UC-059/I-22/after/model-shrink_sequence.json` sha256:46badf323fd321890b129250d2232b780c864922f4c15bdbf0f829d23d5c7b09
- after examples `docs/P-006/UC-059/I-23/after/examples-action_property.json` sha256:34c3f09f76d1e3c1e8f4d4bbfcc856d9313e5548b22f20cdb3fa1e331200d204
- after examples `docs/P-006/UC-059/I-23/after/examples-shrink_sequence.json` sha256:2981b955afe10ccf8a7c0a270ba52c646045cd1c1356edfd6d9d153fc9af7f25
- after contract `docs/P-006/UC-059/I-23/after/model-action_property.json` sha256:e23e22cb7da8347f8ab7e464e0aaabedce5dfb2668399258947eeaf6782e3c42
- after contract `docs/P-006/UC-059/I-23/after/model-shrink_sequence.json` sha256:46badf323fd321890b129250d2232b780c864922f4c15bdbf0f829d23d5c7b09
- after examples `docs/P-006/UC-060/I-24/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-24/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-24/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-24/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-24/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-24/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-24/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-24/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-25/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-25/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-25/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-25/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-25/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-25/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-25/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-25/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-26/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-26/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-26/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-26/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-26/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-26/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-26/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-26/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-27/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-27/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-27/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-27/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-27/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-27/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-27/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-27/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-28/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-28/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-28/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-28/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-28/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-28/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-28/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-28/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-29/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-29/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-29/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-29/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-29/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-29/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-29/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-29/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-30/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-30/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-30/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-30/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-30/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-30/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-30/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-30/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-060/I-31/after/examples-examples_compare.json` sha256:d9dce6a79529c133e6229f0d5e728afe7757a265d222f8b100a1354ea6f3a880
- after examples `docs/P-006/UC-060/I-31/after/examples-snapshot_files.json` sha256:51aa5b889ddc75436579768679e0946d1df24a9739c6e0caaa16e7c4c264ebe3
- after examples `docs/P-006/UC-060/I-31/after/examples-snapshot_kinds.json` sha256:c5a94a94013e5c98bac32cd7e4dc1f4a5e6b95f9ca18b94e91dd5d327bb8519a
- after examples `docs/P-006/UC-060/I-31/after/examples-structural_diff.json` sha256:bd04a242a40cbf66b0500870cdd812f134666f04f952b7f2785ba034a591459d
- after contract `docs/P-006/UC-060/I-31/after/model-examples_compare.json` sha256:5e1beaabb2d7e9be2ddb321b06b11e412a86c6a24cdb05ee71b35a53bb228ab3
- after contract `docs/P-006/UC-060/I-31/after/model-snapshot_files.json` sha256:2989aa23a65e85a788e0148ce64bf70ec12ce1a36af94d31f0d2b20872af3dd1
- after contract `docs/P-006/UC-060/I-31/after/model-snapshot_kinds.json` sha256:3d2f3ccbbd2cd00e09f8880d438d5956b745a9afd3653e165a487eabd87cf1c3
- after contract `docs/P-006/UC-060/I-31/after/model-structural_diff.json` sha256:02b91e73be45e27b13fd34b79fc64425676853c2af6548b6b0496ad5b0f98086
- after examples `docs/P-006/UC-061/I-34/after/examples-review_evidence.json` sha256:a8aed9971553defc5ee51e162b317716377337396d6923f6d3c3ca7c7aecd99c
- after contract `docs/P-006/UC-061/I-34/after/model-review_evidence.json` sha256:95b5736ad8715ac434eb1f4e25d3d7084feffe529ec2273b7ac73c18d114ced8

## Sign-offs

- P-006.I-32: waiting

## Decisions

- **D1** How many fixed-seed model inputs go into the examples snapshot, besides the scenarios? → The whole space when it has at most 1,000 points (iury)
- **D2** What is the driver timeout that fails a driven scenario? → 30 s per action, configurable (iury)
- **D3** How many reviewers must sign off a human-judgment intent or accept an unexpected change? → The review policy's approvals (iury)

## Not promised

- Lastro does not ship a browser, a device or an Appium server.
- Lastro does not run TLC or Lean, and does not export TLA+.
- Liveness properties (something eventually happens) are not checked.
- A pixel or contract change does not fail a proof by itself; only unexpected example changes block until accepted.
- Lastro does not generate the driver for an application.

