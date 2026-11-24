import { A, C, Callout, Code, DataTable, H1, H2, H3, Lead, P, Ul } from "@/components/doc";

// ---------------------------------------------------------------------------
// Features, surfaces and the feature map
// ---------------------------------------------------------------------------

const FEATURE = `{
  "id": "UC-003",
  "slug": "checkout",
  "name": "Checkout",
  "description": "...",
  "surfaces": [
    { "kind": "screen", "name": "Checkout page", "locator": "/checkout" },
    { "kind": "api", "name": "Pay", "locator": "POST /pay" }
  ],
  "dependsOn": ["UC-001", "UC-002"],
  "history": []
}`;

const MAP = `$ lastro map P-004
{
  "status": "map",
  "proposal": "P-004",
  "changed": ["UC-001"],
  "affected": ["UC-002", "UC-003", "UC-004"],
  "features": [ ... ],
  "unreachable": ["UC-002", "UC-004"],
  "text": "UC-001 login\\n  surfaces: route Login (/login)\\n  dependents: UC-002, UC-003\\n..."
}`;

export function RuntimeFeatureMap() {
  return (
    <>
      <H1>Features and the feature map</H1>
      <Lead>
        A feature (a use case file) can say where a person or a client reaches it, its <em>surfaces</em>, and which other features it needs. Together they form
        the feature map: what a change touches, and what it may break.
      </Lead>

      <H2>Surfaces and dependencies</H2>
      <P>
        A surface has a <C>kind</C> (<C>screen</C>, <C>route</C>, <C>command</C> or <C>api</C>), a <C>name</C> in the user's words and a <C>locator</C> the
        driver understands: a URL path, a CLI command, an HTTP method and path, or a selector. <C>dependsOn</C> lists the ids of the features it needs to work.
        Both are optional, and both live in the feature's own file in <C>.lastro/usecases/</C>.
      </P>
      <Code lang="json" title=".lastro/usecases/UC-003-checkout.json" code={FEATURE} />
      <P>
        When an intent adds a feature, the extract step proposes its surfaces and dependencies; when it modifies one, it keeps the existing ones and only adds or
        removes what the change calls for. They are stored with the after and reviewed with the intent, and <C>apply</C> writes them to the feature's file.
      </P>

      <H2>lastro map</H2>
      <P>
        Prints every feature with its surfaces, the features it depends on and the features that depend on it, always in the same order. A feature with no surface
        is listed under <C>unreachable</C>: no driver can reach it. With a proposal, the map is the one the proposal would leave, and every feature that depends
        on a changed one, directly or transitively, is <C>affected</C>: its proofs and snapshots run again with the proposal's own.
      </P>
      <Code lang="json" code={MAP} />
      <P>
        The map refuses a dependency on a feature that does not exist or was removed, and a dependency cycle (status <C>invalid-map</C>, exit 1, with the
        features in the cycle). <C>apply</C> refuses a proposal that would leave an invalid map.
      </P>

      <H2>Surfaces for an existing repository</H2>
      <P>
        <C>lastro bootstrap --surfaces</C> has the survey step read the code and write a bootstrap proposal for every feature with no surfaces. Its intents only
        add surfaces and dependencies, never change a description, and are reviewed like any other proposal. See <A to="bootstrap">Existing repositories</A>.
      </P>
    </>
  );
}

// ---------------------------------------------------------------------------
// State machines
// ---------------------------------------------------------------------------

const MACHINE = `{
  "name": "counter",
  "kind": "machine",
  "actors": 2,
  "state": { "n": { "type": "int", "min": 0, "max": 9 } },
  "local": {
    "pc": { "type": "enum", "values": ["idle", "read", "done"] },
    "t": { "type": "int", "min": 0, "max": 9 }
  },
  "initial": { "n": 0, "pc": "idle", "t": 0 },
  "events": { "read": {}, "write": {} }
}`;

const STEP = `// The code under proof: a state and an event in, the next state out (null: not enabled).
export function counterStep(x) {
  if (x.event === "read" && x.pc === "idle") return { t: x.n, pc: "read" };
  if (x.event === "write" && x.pc === "read") return { n: x.t + 1, pc: "done" };
  return null;
}`;

const VIOLATION = `{
  "clauseId": "C1",
  "kind": "invariant",
  "verdict": "violated",
  "evidence": "counterexample",
  "method": "bfs",
  "checked": 13,
  "path": [
    { "actor": 1, "event": "read", "input": {} },
    { "actor": 2, "event": "read", "input": {} },
    { "actor": 1, "event": "write", "input": {} },
    { "actor": 2, "event": "write", "input": {} }
  ],
  "state": { "n": 1, "pc_1": "done", "t_1": 0, "pc_2": "done", "t_2": 0 }
}`;

const EXPLORE = `$ lastro explore request.json     # {machine, clauses, table, limit?, path?}; stdin without a file
{ "status": "explored", "report": { ... }, "statechart": { ... }, "mermaid": "stateDiagram-v2\\n..." }`;

export function RuntimeStateMachines() {
  return (
    <>
      <H1>State machines</H1>
      <Lead>
        Some behavior is not a function of one input: a session, a cart, two workers updating one counter. A state-machine model proves it over every reachable
        state and every interleaving of its actors, and draws it as a statechart people can read.
      </Lead>

      <H2>The model</H2>
      <P>
        A model with <C>"kind": "machine"</C> has state variables with finite domains, an initial state and events, each with finite inputs. <C>actors</C> runs
        several copies of one process: <C>state</C> is shared, and each actor has its own copy of <C>local</C> (<C>pc_1</C>, <C>pc_2</C>...). The code under
        proof is a step function: it gets the state, the event and its inputs, and returns the next state, or <C>null</C> when the event is not enabled.
      </P>
      <Code lang="json" title=".lastro/models/counter.json" code={MACHINE} />
      <Code lang="js" title="the step function" code={STEP} />

      <H2>Clauses</H2>
      <DataTable
        head={["Clause", "Holds when"]}
        rows={[
          [<C>invariant</C>, "it is true in every reachable state"],
          [<C>step</C>, <>every step whose state and event satisfy <C>requires</C> leads to a next state that satisfies <C>ensures</C> (<C>in.*</C> before, <C>out.*</C> after)</>],
          [<C>reach</C>, "some reachable state satisfies the condition: it guards the others against proving nothing"],
        ]}
      />

      <H2>How a clause is decided</H2>
      <Ul>
        <li>
          Up to 100,000 reachable states across every interleaving, the space is visited breadth first (actors in ascending order, events sorted, inputs
          enumerated). A clause that holds everywhere it applies is <strong>proved</strong>, with the number of states visited.
        </li>
        <li>
          A larger space is first proved by SMT induction: the invariant holds initially and every step preserves it. That counts as proved, with the method
          named (<C>smt-induction</C>). The solver is the one in <C>.lastro/config.json</C> under <C>smt</C> (<C>{`{"command": "z3 -in", "timeoutMs": 10000}`}</C>).
        </li>
        <li>
          Only when the solver cannot decide is it explored by 200 random walks of up to 50 steps with a fixed seed: <strong>evidenced</strong>, never proved.
        </li>
      </Ul>
      <P>A violated clause comes back with the shortest sequence of events, each with its actor, from the initial state to the violation:</P>
      <Code lang="json" title="the lost update: both actors read 0, both write 1" code={VIOLATION} />

      <H2>Statecharts</H2>
      <P>
        Every state-machine model is drawn as a statechart from the user's point of view: states, transitions labelled with their events, one parallel region
        for the shared state and one per actor, the states an invariant forbids marked, and the counterexample path drawn on it. The receipt carries it as a
        Mermaid <C>stateDiagram-v2</C>, and the <A to="plugins/review-app">review app</A> shows it next to the intent's proofs.
      </P>

      <H2>Four engines, one answer</H2>
      <P>
        The TypeScript engine records every step it asks the code about. The Rust prover, the Go SDK and the .NET SDK explore the same machine over that table on
        their own, and must visit the same states and give the same verdicts and counterexamples; a missing row counts as a disagreement. <C>explore</C> runs
        that exploration from the command line, in every CLI:
      </P>
      <Code lang="bash" code={EXPLORE} />
    </>
  );
}

// ---------------------------------------------------------------------------
// Drivers
// ---------------------------------------------------------------------------

const WEB = `"drivers": {
  "screen": {
    "webdriver": "http://localhost:9515",
    "capabilities": { "browserName": "chrome", "goog:chromeOptions": { "args": ["--headless=new"] } },
    "baseUrl": "http://localhost:5173"
  }
}`;

const APPIUM = `"drivers": {
  "screen": {
    "appium": "http://localhost:4723",
    "capabilities": {
      "platformName": "Android",
      "appium:automationName": "UiAutomator2",
      "appium:app": "./app/build/outputs/apk/debug/app-debug.apk"
    }
  }
}
// iOS: "platformName": "iOS", "appium:automationName": "XCUITest", "appium:app": "./build/App.app"`;

const HTTP = `"drivers": {
  "api": { "baseUrl": "http://localhost:3000", "headers": { "authorization": "Bearer test-token" } },
  "command": { "process": true, "cwd": "." },
  "data": { "command": "node scripts/schema.mjs" },
  "timeoutMs": 30000
}`;

const JSONLINES = `"drivers": { "screen": { "jsonlines": "node drivers/tui.mjs" } }

-> {"open": {"kind": "screen", "locator": "/chats"}}   <- {"ok": true}
-> {"act": {"type": "click", "target": "#send"}}       <- {"observed": {...}, "screenshot": "<base64 PNG>"}
-> {"screenshot": true}                                <- {"screenshot": "<base64 PNG>"}
-> {"close": true}                                     <- {"ok": true}`;

const DRIVEN = `{
  "kind": "driven",
  "surface": { "kind": "api", "locator": "POST /pay" },
  "steps": [
    { "input": { "body": { "cart": "c1" } }, "expect": { "status": 200, "body": { "paid": true } } },
    { "input": { "body": { "cart": "c1" } }, "expect": { "status": 409 } }
  ]
}`;

const PROPERTY = `{
  "kind": "property",
  "surface": { "kind": "screen", "locator": "/cart" },
  "actions": [{ "type": "click", "target": "#add" }, { "type": "click", "target": "#remove" }],
  "invariant": { "op": "ge", "args": [{ "var": "out.count" }, { "const": 0 }] }
}`;

export function RuntimeDrivers() {
  return (
    <>
      <H1>Drivers</H1>
      <Lead>
        A driver runs the application through one of its surfaces, over the protocols the market already uses. Lastro ships no browser, device or Appium server:
        you name yours in <C>.lastro/config.json</C> under <C>drivers</C>.
      </Lead>

      <DataTable
        head={["Surface", "Protocol"]}
        rows={[
          [<>web and Electron <C>screen</C> / <C>route</C></>, "W3C WebDriver, with any WebDriver server (chromedriver, geckodriver, Electron's)"],
          [<>Android and iOS <C>screen</C></>, <>the same protocol through Appium (<C>appium:*</C> capabilities)</>],
          [<C>api</C>, "HTTP, called directly"],
          [<C>command</C>, "a process: the locator is the command, the input's stdin goes in"],
          ["any kind", "Lastro's JSON lines protocol, only when no standard one fits"],
        ]}
      />
      <P>
        A driver that fails to start or times out (30 seconds per action by default, <C>timeoutMs</C>) fails the scenario with the driver's error. It never
        passes it.
      </P>

      <div data-setup="web">
      <H2>Web</H2>
      <Code lang="json" title=".lastro/config.json" code={WEB} />
      </div>
      <div data-setup="appium">
      <H2>Android and iOS with Appium</H2>
      <Code lang="json" title=".lastro/config.json" code={APPIUM} />
      </div>
      <div data-setup="http">
      <H2>HTTP, commands and data</H2>
      <P>
        <C>data</C> is a command that prints the structure of the application's data as JSON (<C>{`{"tables": {...}}`}</C> or a JSON Schema): the data snapshot
        of <A to="runtime/evidence">before and after evidence</A>.
      </P>
      <Code lang="json" title=".lastro/config.json" code={HTTP} />
      </div>
      <div data-setup="jsonlines">
      <H2>JSON lines</H2>
      <P>The driver is a process: Lastro writes one JSON object per line and reads one back. Any answer with <C>error</C> fails the step with that error.</P>
      <Code lang="text" code={JSONLINES} />
      </div>

      <H2>Driven scenarios</H2>
      <P>
        A <C>driven</C> proof is a list of steps, each an input and the output expected from it: a user action and the expected state (and optionally a reference
        screenshot) for a screen, a request and the expected status and body for an api. Every field given must equal the observed one; fields left out are not
        checked.
      </P>
      <Code lang="json" code={DRIVEN} />

      <H2>Properties over action sequences</H2>
      <P>
        A <C>property</C> proof names a surface, the actions available there and an invariant over the observed state (<C>out.*</C>). Lastro generates action
        sequences with a fixed seed, runs each through the driver and checks the invariant after every action. A failing sequence is shortened to its shortest
        failing prefix and replayed once; if the replay does not fail, the check is <C>nondeterministic</C>, not violated.
      </P>
      <Code lang="json" code={PROPERTY} />
      <Callout title="Proved over the model, followed by the application">
        When the surface has a state-machine model (<C>machine</C> and <C>events</C> on the proof), the property is first checked over the model, exhaustively or
        by SMT induction, and proved there; the action sequences then check that the running application follows the model. A property with no model that holds
        on every sequence is evidenced, with the rule-of-three bound.
      </Callout>
    </>
  );
}

// ---------------------------------------------------------------------------
// Before and after evidence
// ---------------------------------------------------------------------------

const TREE = `.lastro/docs/P-006/
  UC-003/I-07/before/checkout-1.png
  UC-003/I-07/after/checkout-1.png
  UC-003/I-07/before/contract-pay.json
  UC-003/I-07/after/contract-pay.json
  UC-003/I-08/before/examples-shipping.json
  UC-003/I-08/after/examples-shipping.json
  before/data.json
  after/data.json`;

const PROVE = `$ lastro prove P-006
{
  "status": "waiting-for-review",
  "signoffsDue": ["P-006.I-07"],
  "unexpected": [{ "id": "U1", "intent": "P-006.I-08", "input": { "member": false, "subtotal": 99 }, "before": { "fee": 15 }, "after": { "fee": 0 } }],
  ...
}`;

const REVIEW = `$ lastro review P-006 --as ana signoff I-07 docs/P-006/UC-003/I-07/after/checkout-1.png
$ lastro review P-006 --as ana request-changes I-07 "The total moved below the fold"
$ lastro review P-006 --as ana accept U1
$ lastro review P-006 --as ana reference docs/P-006/UC-003/I-07/after/checkout-1.png`;

export function RuntimeEvidence() {
  return (
    <>
      <H1>Before and after evidence</H1>
      <Lead>
        A proof says the code does what the intent says. Evidence shows the reviewer what changed: every intent gets snapshots of the features it touches on the
        current code, and the same snapshots on the new code when the proposal is proved.
      </Lead>

      <H2>What a snapshot is</H2>
      <DataTable
        head={["Kind", "Taken from", "Compared by"]}
        rows={[
          [<C>screen</C>, "the images the driver captured while running a screen or route surface's driven scenarios", "the share of pixels changed"],
          [<C>contract</C>, "the request and response shapes an api or command driver reports, as JSON Schema; and for every feature, the model's inputs and outputs with their domains", "a structural diff"],
          [<C>data</C>, <>the structure of the data, from the <C>data</C> driver: tables and columns, or a JSON Schema</>, "a structural diff"],
          [<C>examples</C>, "the scenarios' inputs plus a fixed-seed sample of the model's inputs, each run on the code", "the rows whose output changed"],
        ]}
      />
      <P>
        The structural diff lists every field or column added, removed or changed in type, always in the same order; a removal or a type change is marked{" "}
        <strong>breaking</strong>. The before snapshots are taken at extraction (<C>propose</C>, or <C>lastro snapshot P-006 before</C>), the after ones by{" "}
        <C>prove</C> (or <C>lastro snapshot P-006 after</C>).
      </P>

      <H2>Where they live</H2>
      <P>
        Snapshots are kept under <C>.lastro/docs/&lt;proposal&gt;/&lt;feature&gt;/&lt;intent&gt;/before|after/</C> and committed with the proposal, images
        included. The proposal records the SHA-256 of each file, so anyone can take them again and compare.
      </P>
      <Code lang="text" code={TREE} />

      <H2>Unexpected changes and sign-offs</H2>
      <P>
        An example whose output changed although no approved intent's clause applies to its input is an <strong>unexpected change</strong>. An intent beyond
        proof that needs human judgment (a layout, a tone) waits, after the run, for a reviewer to look at its before and after and <strong>sign it off</strong>{" "}
        or request changes. While either is due, the proposal is not proved: <C>prove</C> reports <C>waiting-for-review</C> and exits 1, and{" "}
        <C>apply</C> refuses it.
      </P>
      <Code lang="json" code={PROVE} />
      <Code lang="bash" code={REVIEW} />
      <P>
        The screenshots named on a sign-off (paths under <C>.lastro/</C>) are copied to the intent's <C>reference/</C> folder: from then on they are the
        reference of that scenario step for the regressions that follow. <C>reference</C> does the same for one screenshot without a sign-off. A request for
        changes keeps the proposal open until the next run is signed off.
      </P>

      <H3>In the receipt</H3>
      <P>
        The receipt adds a <C>Feature map</C> section with the affected features, a Mermaid state diagram for every state-machine model, the snapshots with their
        hashes and comparisons, the unexpected changes and who accepted them, and the sign-offs with who gave them. It also counts how many intents were proved
        formally, evidenced or beyond proof, with the reason for each one that is not formal. See <A to="concepts/receipts">receipts</A>.
      </P>
    </>
  );
}
