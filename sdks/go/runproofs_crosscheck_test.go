package lastro_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// Machine, driven and property proofs run as the TS SDK runs them: same verdicts, notes and counterexamples. The
// driven scenarios and the property go through one HTTP server both SDKs drive.
const runProofsScript = `
import * as L from "./dist/src/index.js";
import { driveRunner } from "./dist/src/evidence.js";
const proofs = JSON.parse(process.env.PROOFS);
const counter = JSON.parse(process.env.MACHINE);
const step = (x) =>
  x.event === "read" && x.pc === "idle" ? { t: x.n, pc: "read" } : x.event === "write" && x.pc === "read" ? { n: x.t + 1, pc: "done" } : null;
const drivers = { api: { baseUrl: process.env.BASE } };
const ws = { file: (p) => p, root: process.cwd() };
const { outcomes } = await L.runProofs(proofs.map((proof) => ({ proof, role: "proposal" })), { models: { counter }, subjects: { counter: step }, drive: driveRunner(ws, drivers) });
process.stdout.write(JSON.stringify(outcomes));
`

func TestMachineDrivenAndPropertyProofsRunAsInTheTSSDK(t *testing.T) {
	dir := sdkDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pay":
			w.Header().Set("content-type", "application/json")
			fmt.Fprint(w, `{"paid":true,"total":10}`)
		case "/boom":
			w.WriteHeader(500)
			fmt.Fprint(w, "boom")
		default:
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer srv.Close()
	machine := `{"name":"counter","kind":"machine","actors":2,"state":{"n":{"type":"int","min":0,"max":9}},"local":{"pc":{"type":"enum","values":["idle","read","done"]},"t":{"type":"int","min":0,"max":9}},"initial":{"n":0,"pc":"idle","t":0},"events":{"read":{},"write":{}}}`
	done := func(a int) string {
		return fmt.Sprintf(`{"op":"ite","args":[{"op":"eq","args":[{"var":"in.pc_%d"},{"const":"done"}]},{"const":1},{"const":0}]}`, a)
	}
	pf := func(id, check string) string {
		return fmt.Sprintf(`{"id":"P-001.PF-%s","slug":"pf-%s","name":"x","description":"x","intent":"P-001.I-01","useCase":"UC-001","model":"counter","subject":"counter","check":%s,"status":"pending"}`, id, id, check)
	}
	proofs := "[" + pf("01", `{"kind":"machine","clause":"invariant","ensures":{"op":"eq","args":[{"var":"in.n"},{"op":"add","args":[`+done(1)+`,`+done(2)+`]}]}}`) + "," +
		pf("02", `{"kind":"machine","clause":"reach","ensures":{"op":"eq","args":[{"var":"in.n"},{"const":2}]}}`) + "," +
		pf("03", `{"kind":"machine","clause":"step","requires":{"op":"eq","args":[{"var":"in.event"},{"const":"write"}]},"ensures":{"op":"eq","args":[{"var":"out.pc"},{"const":"done"}]}}`) + "," +
		pf("04", `{"kind":"driven","surface":{"kind":"api","locator":"POST /pay"},"steps":[{"input":{"method":"POST","path":"/pay","body":{"cart":"c1"}},"expect":{"status":200,"body":{"paid":true}}}]}`) + "," +
		pf("05", `{"kind":"driven","surface":{"kind":"api","locator":"POST /pay"},"steps":[{"input":{"path":"/pay"},"expect":{"status":200}},{"input":{"path":"/pay"},"expect":{"body":{"total":11}}}]}`) + "," +
		pf("06", `{"kind":"property","surface":{"kind":"api","locator":"/"},"actions":[{"path":"/a"},{"path":"/b"}],"invariant":{"op":"eq","args":[{"var":"out.status"},{"const":200}]}}`) + "," +
		pf("07", `{"kind":"property","surface":{"kind":"api","locator":"/"},"actions":[{"path":"/a"},{"path":"/boom"}],"invariant":{"op":"eq","args":[{"var":"out.status"},{"const":200}]}}`) + "]"

	cmd := exec.Command("node", "--input-type=module", "-e", runProofsScript)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PROOFS="+proofs, "MACHINE="+machine, "BASE="+srv.URL)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}

	var entries []lastro.ProofEntry
	if err := json.Unmarshal([]byte(proofs), &entries); err != nil {
		t.Fatal(err)
	}
	var m lastro.MachineModel
	_ = json.Unmarshal([]byte(machine), &m)
	set := []lastro.RoleProof{}
	for _, e := range entries {
		set = append(set, lastro.RoleProof{Proof: e, Role: "proposal"})
	}
	step := func(x lastro.Env) (lastro.Env, error) {
		n, _ := x["n"].(float64)
		tv, _ := x["t"].(float64)
		switch {
		case x["event"] == "read" && x["pc"] == "idle":
			return lastro.Env{"t": n, "pc": "read"}, nil
		case x["event"] == "write" && x["pc"] == "read":
			return lastro.Env{"n": tv + 1, "pc": "done"}, nil
		}
		return nil, nil
	}
	drivers := &lastro.DriverConfig{API: json.RawMessage(fmt.Sprintf(`{"baseUrl":%q}`, srv.URL))}
	outcomes, _, err := lastro.RunProofs(context.Background(), set, lastro.RunOptions{
		Machines: map[string]lastro.MachineModel{"counter": m},
		Subjects: map[string]lastro.Subject{"counter": step},
		Drive:    lastro.Drivers{Config: drivers, Dir: ".", Root: "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ts []json.RawMessage
	_ = json.Unmarshal(out, &ts)
	if len(ts) != len(outcomes) {
		t.Fatalf("TS %d outcomes, Go %d:\n%s", len(ts), len(outcomes), out)
	}
	for i := range ts {
		if g, w := canonOf(t, outcomes[i]), canon(t, ts[i]); g != w {
			t.Errorf("outcome %d:\nTS %s\nGo %s", i, w, g)
		}
	}
	// A proof's check is written back exactly as the TS SDK writes it.
	var all []map[string]json.RawMessage
	_ = json.Unmarshal([]byte(proofs), &all)
	for i, e := range entries {
		b, _ := json.Marshal(e.Check)
		if string(b) != string(all[i]["check"]) {
			t.Errorf("check %d: written %s, read %s", i, b, all[i]["check"])
		}
	}
}
