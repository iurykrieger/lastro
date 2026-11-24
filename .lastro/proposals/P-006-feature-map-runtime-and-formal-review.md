# Feature map, runtime and state-machine proofs, and review with before/after captures

Lastro learns how a user reaches each use case, runs the application through that path, proves state machines
exhaustively when they are finite, and shows the reviewer what changed on screen next to the intents and verdicts.
The engines stay the judges: Lastro never writes or fixes application code, and its only AI steps stay intent
extraction (with the survey that reads code) and proof derivation.

## The feature map

A use case may carry surfaces and dependencies. A surface says how a user reaches the use case from the user's
point of view: its kind (screen, route, command or api), a name in the user's words ("the left sidebar's chat list")
and a locator the driver understands (a URL path, a CLI command, a selector). A dependency names another use case
this one needs to work (opening a chat depends on signing in). The extraction step proposes surfaces and dependencies
for every use case an intent adds or modifies, and the reviewer approves them with the intent, like the description.
A use case with no surface is not reachable by the driver and is listed as such.

`lastro features` prints the feature map: every use case with its surfaces, what it depends on and what depends on
it. It is deterministic. It rejects a dependency on a use case that does not exist or that was removed, and a
dependency cycle, naming the use cases in it. When a proposal changes a use case, the map lists the use cases that
depend on it, directly or not, as affected: their proofs and captures run again with the proposal's own.

For a repository whose use cases have no surfaces yet, `lastro bootstrap --features` asks the survey step to read the
code and propose surfaces and dependencies for them. It writes a bootstrap proposal whose intents only add surfaces
and dependencies, never change a description, and that proposal is reviewed like any other.

## Driving the application

A driver is a process named in `.lastro/config.json` under `drivers`, written in any language, that Lastro starts
and talks to over JSON lines: Lastro sends a surface and a list of user actions (open, click, type, press, wait for),
and the driver answers with the observed state (named values the model declares as outputs) and the paths of the
captures it took. Lastro ships no browser of its own; the docs show a Playwright driver for web and Electron apps
and a process driver for CLIs.

A scenario may name a surface and a list of actions instead of a concrete input. It then runs through the driver:
the expectation is checked against the observed state, the result is empirical evidence, and a driver that fails to
start or times out fails the scenario with the driver's error, never passes it.

## State-machine proofs

A model may be a state machine: state variables with finite domains, an initial state, and events, each with finite
inputs. The code under proof is a step function from a state and an event to the next state. Clauses are of three
kinds: an invariant must hold in every reachable state; a step clause relates a state, an event and the next state;
a reachability clause says a state that satisfies a condition can be reached, which guards the other clauses against
proving nothing.

When the reachable state space has at most 100,000 states, Lastro visits all of it breadth first, and a clause that
holds everywhere it applies is proved, with formal evidence and the number of states visited. A larger or unbounded
space is explored by random walks from the initial state with a fixed seed, and a clause that holds is evidenced,
never proved. A violation comes back as the shortest sequence of events from the initial state that reaches it. The
TypeScript engine, the Rust prover, the Go SDK and the .NET SDK visit the states in the same order and give the same
verdicts and counterexamples.

## Properties over user action sequences

A property-based check names a surface, the actions a user can take there and an invariant over the observed state.
Lastro generates action sequences with a fixed seed, runs each through the driver and checks the invariant after
every action. A property that holds on every sequence is evidenced, with the number of sequences and actions run and
the rule-of-three failure bound; it is never proved. A failing sequence is shortened to the shortest prefix that
still fails and then replayed once: if the replay does not fail, the check is reported as nondeterministic instead of
violated.

## Before and after captures

When a proposal is extracted, every intent that touches a use case with a screen or route surface gets a before
capture: Lastro runs that surface's scenarios through the driver on the current code and stores what the driver
captured. When the proposal is proved, the same scenarios run on the new code and give the after capture. Captures
live in `.lastro/captures/<proposal>/<use case>/before|after/` and are never committed to git; the proposal records
the SHA-256 of each capture, so anyone can capture again and see whether the result still matches. A pixel
comparison of each before and after pair is computed deterministically and reported as the share of pixels changed.

An intent whose promise is a matter of human judgment (the layout looks right, the message is clear) stays beyond
proof, but after the proof run it waits for a sign-off: a reviewer looks at its before and after captures and signs
it off or requests changes. A proposal is not proved while such an intent waits for its sign-off.

## Reviewing the evidence

The review app shows, for each intent: the use case's place in the feature map (its surfaces, what it depends on and
what it affects), the statechart drawn from its state-machine model when it has one, its before capture during the
review and its before and after captures with the pixel comparison after the proof run, and the verdict of every
proof and property with its counterexample. The receipt adds a Feature map section with the affected use cases, a
state diagram in Mermaid for every state-machine model, the captures with their hashes and pixel comparison, and the
sign-offs with who gave them.

The CLI, the TypeScript, Go and .NET SDKs, the docs and the plugin's skills cover all of this, and the plugin's
review shows the captures' paths and the sign-offs still due.
