package lastro_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// State machines must be explored exactly as the TS engine explores them: same states, verdicts, paths, notes and
// statecharts. The TS engine records every step it asks the code about; the Go engine explores over that table.
const machineScript = `
import * as L from "./dist/src/index.js";
const v = (n) => ({ var: n });
const k = (x) => ({ const: x });
const op = (o, ...args) => ({ op: o, args });
const done = (a) => op("ite", op("eq", v("in.pc_" + a), k("done")), k(1), k(0));
const counter = (actors) => ({
  name: "counter", kind: "machine", actors,
  state: { n: { type: "int", min: 0, max: 9 } },
  local: { pc: { type: "enum", values: ["idle", "read", "done"] }, t: { type: "int", min: 0, max: 9 } },
  initial: { n: 0, pc: "idle", t: 0 },
  events: { read: {}, write: {} },
});
const counterStep = (atomic) => (x) =>
  x.event === "read" && x.pc === "idle" ? (atomic ? { n: x.n + 1, pc: "done" } : { t: x.n, pc: "read" })
  : !atomic && x.event === "write" && x.pc === "read" ? { n: x.t + 1, pc: "done" } : null;
const sum = (actors) => Array.from({ length: actors }, (_, i) => done(i + 1)).reduce((a, b) => op("add", a, b));
const clauses = (actors) => [
  { id: "C1", kind: "invariant", holds: op("eq", v("in.n"), sum(actors)) },
  { id: "C2", kind: "step", requires: op("eq", v("in.event"), k("write")), ensures: op("eq", v("out.pc"), k("done")) },
  { id: "C3", kind: "step", requires: op("eq", v("in.pc"), k("done")), ensures: k(true) },
  { id: "C4", kind: "reach", condition: op("eq", v("in.n"), k(actors)) },
  { id: "C5", kind: "reach", condition: op("eq", v("in.n"), k(9)) },
];
const dial = {
  name: "dial", kind: "machine",
  state: { at: { type: "int", min: 0, max: 5 }, mode: { type: "enum", values: ["on", "off"] } },
  initial: { at: 0, mode: "off" },
  events: { turn: { by: { type: "int", min: 1, max: 2 } }, toggle: {} },
};
const dialStep = (x) => x.event === "toggle" ? { mode: x.mode === "on" ? "off" : "on" } : x.mode === "on" ? { at: Math.min(5, x.at + x.by) } : null;
const dialClauses = [
  { id: "D1", kind: "invariant", holds: op("le", v("in.at"), k(4)) },
  { id: "D2", kind: "step", requires: op("eq", v("in.event"), k("turn")), ensures: op("gt", v("out.at"), v("in.at")) },
];
const cases = [
  { name: "atomic", m: counter(2), step: counterStep(true), clauses: clauses(2) },
  { name: "lost-update", m: counter(2), step: counterStep(false), clauses: clauses(2) },
  { name: "three-actors", m: counter(3), step: counterStep(false), clauses: clauses(3) },
  { name: "walks", m: counter(3), step: counterStep(true), clauses: clauses(3), limit: 5 },
  { name: "dial", m: dial, step: dialStep, clauses: dialClauses },
];
const out = [];
for (const c of cases) {
  const table = [];
  const step = (x) => { const o = c.step(x); table.push({ input: { ...x }, output: o ?? null }); return o; };
  const report = await L.exploreMachine(c.m, step, c.clauses, c.limit ? { limit: c.limit } : {});
  const path = report.clauses.find((r) => r.verdict === "violated" && r.path)?.path ?? [];
  const chart = await L.statechart(c.m, step, c.clauses, path, c.limit ? { limit: c.limit } : {});
  out.push({ name: c.name, request: { machine: c.m, clauses: c.clauses, table, limit: c.limit ?? 0, path }, report, chart, mermaid: L.mermaidStatechart(chart) });
}
process.stdout.write(JSON.stringify(out));
`

func TestMachinesAreExploredAsTheTSEngineDoes(t *testing.T) {
	dir := sdkDir(t)
	cmd := exec.Command("node", "--input-type=module", "-e", machineScript)
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string `json:"name"`
		Request struct {
			Machine lastro.MachineModel    `json:"machine"`
			Clauses []lastro.MachineClause `json:"clauses"`
			Table   []lastro.StepRow       `json:"table"`
			Limit   int                    `json:"limit"`
			Path    []lastro.MachineEvent  `json:"path"`
		} `json:"request"`
		Report  json.RawMessage `json:"report"`
		Chart   json.RawMessage `json:"chart"`
		Mermaid string          `json:"mermaid"`
	}
	if err := json.Unmarshal(out, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 5 {
		t.Fatalf("%d cases", len(cases))
	}
	for _, c := range cases {
		r := c.Request
		step := lastro.TableStep(r.Table)
		report := lastro.ExploreMachine(r.Machine, step, r.Clauses, lastro.MachineOptions{Limit: r.Limit})
		if g, w := canonOf(t, report), canon(t, c.Report); g != w {
			t.Errorf("%s report:\nTS %s\nGo %s", c.Name, w, g)
		}
		chart := lastro.MachineStatechart(r.Machine, step, r.Clauses, r.Path, r.Limit)
		if g, w := canonOf(t, chart), canon(t, c.Chart); g != w {
			t.Errorf("%s statechart:\nTS %s\nGo %s", c.Name, w, g)
		}
		if g := lastro.MermaidStatechart(chart); g != c.Mermaid {
			t.Errorf("%s mermaid:\nTS %s\nGo %s", c.Name, c.Mermaid, g)
		}
	}
}
