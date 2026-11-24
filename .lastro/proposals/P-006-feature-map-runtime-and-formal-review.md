# Feature map, formal proofs whenever possible, and review with before/after snapshots

Lastro learns how a user reaches each feature, proves behavior formally whenever a finite model can express it
(including state machines whose actors interleave, as TLA+ model checking does, and with an SMT solver when the space is too large to enumerate), runs the application through its
surfaces, and shows the reviewer what changed next to the intents and verdicts: screens for the frontend, contracts,
data structures and examples of behavior for the backend. The engines stay the judges: Lastro never writes or fixes
application code, and its only AI steps stay intent extraction (with the survey that reads code) and proof
derivation. This proposal speaks of features, as the proposal that renames use cases to features does.

## The feature map

A feature may carry surfaces and dependencies. A surface says how a user or a client reaches the feature: its kind
(screen, route, command or api), a name in the user's words ("the left sidebar's chat list") and a locator the
driver understands (a URL path, a CLI command, an HTTP method and path, a selector). A dependency names another
feature this one needs to work (opening a chat depends on signing in). The extraction step proposes surfaces and
dependencies for every feature an intent adds, and for a feature an intent modifies it keeps the surfaces and
dependencies the feature already has, adding or removing only the ones the change calls for; the reviewer approves
them with the intent, like the description. A feature with no surface is not reachable by the driver and is listed as such.

`lastro map` prints the feature map: every feature with its surfaces, what it depends on and what depends on it. It
is deterministic. It rejects a dependency on a feature that does not exist or that was removed, and a dependency
cycle, naming the features in it. When a proposal changes a feature, the map lists the features that depend on it,
directly or not, as affected: their proofs and snapshots run again with the proposal's own.

For a repository whose features have no surfaces yet, `lastro bootstrap --surfaces` asks the survey step to read the
code and propose surfaces and dependencies for them. It writes a bootstrap proposal whose intents only add surfaces
and dependencies, never change a description, and that proposal is reviewed like any other.

## Formal whenever possible

The derive step must prove formally whenever it can. For each approved intent it first tries a finite model the
code can be checked against exhaustively: an input space of at most 100,000 points, or a state machine whose
reachable states number at most 100,000. When the space is larger, it tries an SMT proof (the clauses compiled to
SMT-LIB and checked by an SMT solver, by induction over the steps for a state machine). Only when neither can
express the intent does it fall back to sampling, traces or scenarios, and it then records why, in one of the reasons where proof stops or as a domain too
large to enumerate. The receipt counts how many intents are proved formally, evidenced, and beyond proof, with the
reason for each one that is not formal.

## State-machine proofs

A model may be a state machine: state variables with finite domains, an initial state, and events, each with finite
inputs. It may declare actors, several copies of the same process (two devices, three clients) that share part of
the state; their events interleave in every possible order, which is how races between them are found. The code
under proof is a step function from a state and an event (with the actor that raised it) to the next state.

Clauses are of three kinds: an invariant must hold in every reachable state; a step clause relates a state, an
event and the next state; a reachability clause says a state that satisfies a condition can be reached, which guards
the other clauses against proving nothing. When the reachable state space, across every interleaving, has at most
100,000 states, Lastro visits all of it breadth first, and a clause that holds everywhere it applies is proved, with
formal evidence and the number of states visited. A larger or unbounded space is not sampled first: if it can be proved, it is. Lastro tries an inductive proof
with the SMT solver (the invariant holds initially and every step preserves it), and a clause it proves this way is
proved, with formal evidence and the method named. Only when the solver cannot decide does Lastro explore by random
walks from the initial state with a fixed seed, and a clause that holds is then evidenced, never proved. A violation
comes back as the shortest sequence of events, each with its actor, from the initial state that reaches it. The TypeScript engine, the
Rust prover, the Go SDK and the .NET SDK visit the states in the same order and give the same verdicts and
counterexamples.

Every state-machine model is drawn as a statechart from the user's point of view: its states, the transitions
between them labelled with the events that cause them, each actor as a parallel region, and the states each invariant
forbids marked. The reviewer sees the statechart next to the intent's proofs and the counterexample path drawn on it,
and requests changes on the intent when the statechart does not match what they meant.

## Driving the application

Lastro drives the application through the protocols the market already uses, so any existing driver works. A web or
Electron surface is driven over W3C WebDriver (BiDi where the driver supports it), with any WebDriver server such
as chromedriver or geckodriver. An Android or iOS surface is driven over the same protocol through Appium. An api
surface is called over HTTP directly, and a command surface is run as a process. `.lastro/config.json` names, under
`drivers`, the WebDriver or Appium server and its capabilities, the HTTP base URL, or the command for each surface
kind. Only when no standard protocol fits does a driver speak Lastro's own JSON lines protocol: Lastro sends a
surface and a list of actions, and the driver answers with the observed state and the snapshots it took. Lastro
ships no browser or device of its own; the docs show a web setup, an Android and iOS setup with Appium, an HTTP setup
and a JSON lines driver.

A scenario is a list of steps, each an input and the output expected from it. For a screen the input is a user
action and the expected output is the observed state and, where layout matters, a reference screenshot; for an api
the input is a request and the expected output is the response, its status and its body; for a pure function it is
an input and an output of the model. A scenario that names a surface runs through its driver, each step's output is
checked against the expected one, and the result is empirical evidence; a reference screenshot matches when the share
of pixels changed stays under the tolerance the scenario sets. A driver that fails to start or times out fails the
scenario with the driver's error, never passes it. When a reviewer signs off an after screenshot, it becomes the
reference screenshot of that step for the regressions that follow.

## Properties over action sequences

A property-based check names a surface, the actions a user or client can take there and an invariant over the
observed state. Lastro generates action sequences with a fixed seed, runs each through the driver and checks the
invariant after every action. If it can be proved, it is: when the surface has a
state-machine model, the property is first checked over that model (exhaustively or by SMT induction) and is
proved there, and the action sequences then check that the running application follows the model. A property with
no model that holds on every sequence is evidenced, with the number of sequences and actions run and the
rule-of-three failure bound; it is never proved. A failing sequence is shortened to the shortest
prefix that still fails and then replayed once: if the replay does not fail, the check is reported as
nondeterministic instead of violated.

## Before and after snapshots

When a proposal is extracted, every intent gets a before snapshot of each feature it touches, taken on the current
code; when the proposal is proved, the same snapshot is taken on the new code and is the after. A snapshot has one
of four kinds, chosen by the feature's surfaces and models:

- screen: for a screen or route surface, the images the driver captured while running that surface's scenarios;
- contract: for an api or command surface, the contract the driver reports (the endpoint's or command's request and
  response shapes, as JSON Schema) and, for every feature, the model's inputs and outputs with their domains;
- data: the structure of the data the feature reads or writes, as the driver reports it (tables and columns, or a
  JSON Schema of a stored document);
- examples: a table of example inputs, the scenarios' inputs plus a fixed-seed sample of the model's inputs, each
  run on the before and the after code with both outputs side by side.

Each pair gets a deterministic comparison: for screens, the share of pixels changed; for contracts and data, a
structural diff that lists every field or column added, removed or changed in type, and marks a removal or a type
change as breaking; for examples, the rows whose output changed. An example whose output changed although no
approved intent's clause applies to its input is flagged as an unexpected change, and the proposal is not proved
while an unexpected change is not accepted by a reviewer.

Snapshots are kept with the documentation of the change, under the proposal, the feature and the intent that changed
it: `.lastro/docs/<proposal>/<feature>/<intent>/before|after/`. They are committed with the proposal, images included,
so every reviewer sees them in the pull request without taking them again; the proposal also records the SHA-256 of
each one, so anyone can take it again and see whether the result still matches. A snapshot of the whole repository
rather than of one feature lives in `.lastro/docs/<proposal>/repository/before|after/`.

An intent whose promise is a matter of human judgment (the layout looks right, the message is clear, the new error
response is helpful) stays beyond proof, but after the proof run it waits for a sign-off: a reviewer looks at its
before and after snapshots and signs it off or requests changes. A proposal is not proved while such an intent waits
for its sign-off.

## Reviewing the evidence

The review app shows, for each intent: the feature's place in the feature map (its surfaces, what it depends on and
what it affects), the statechart drawn from its state-machine model when it has one, its before snapshots during the
review and its before and after snapshots with their comparison after the proof run (images side by side for
screens, a diff for contracts and data, the changed rows for examples), and the verdict of every proof and property
with its counterexample. The receipt adds a Feature map section with the affected features, a state diagram in
Mermaid for every state-machine model, the formal coverage counts, the snapshots with their hashes and comparisons,
the unexpected changes and who accepted them, and the sign-offs with who gave them.

The CLI, the TypeScript, Go and .NET SDKs, the docs and the plugin's skills cover all of this, and the plugin's
review shows the snapshots' paths, the unexpected changes and the sign-offs still due.
