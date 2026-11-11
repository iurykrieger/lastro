package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/examples/login"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// extractAnswer is what an agent would write as the answer to the extract handoff.
func extractAnswer() any {
	a := must(login.Extract(login.Prose))
	var intents, decisions []map[string]any
	for _, i := range a.Intents {
		reason := i.BeyondProofReason
		if reason == "" {
			reason = "none"
		}
		intents = append(intents, map[string]any{"statement": i.Statement, "kind": i.Kind, "provability": i.Provability, "beyondProofReason": reason, "quote": i.Source.Quote})
	}
	for _, d := range a.Decisions {
		var idx []int
		for _, id := range d.IntentIDs {
			idx = append(idx, must(strconvAtoi(id[1:]))-1)
		}
		decisions = append(decisions, map[string]any{"intentIndexes": idx, "question": d.Question, "options": d.Options, "recommended": d.Recommended})
	}
	return map[string]any{"intents": intents, "decisions": decisions, "notPromised": a.NotPromised}
}

func strconvAtoi(s string) (int, error) {
	var n int
	err := json.Unmarshal([]byte(s), &n)
	return n, err
}

// deriveAnswer is what an agent would write as the answer to the derive handoff.
func deriveAnswer(frozen lastro.IntentArtifact) any {
	plan := login.DerivePlan(frozen)
	var clauses, scenarios []map[string]any
	for _, c := range plan.Clauses {
		req := ""
		if c.Requires != nil {
			req = string(must(json.Marshal(c.Requires)))
		}
		assumes := c.Assumes
		if assumes == nil {
			assumes = []string{}
		}
		clauses = append(clauses, map[string]any{"id": c.ID, "intentIds": c.IntentIDs, "description": c.Description, "requires": req, "ensures": string(must(json.Marshal(c.Ensures))), "assumes": assumes})
	}
	for _, s := range plan.Scenarios {
		scenarios = append(scenarios, map[string]any{"id": s.ID, "intentIds": s.IntentIDs, "given": s.Given, "when": s.When, "then": s.Then, "inputJson": string(must(json.Marshal(s.Input))), "expect": string(must(json.Marshal(s.Expect)))})
	}
	return map[string]any{"clauses": clauses, "assumptions": plan.Assumptions, "scenarios": scenarios, "beyondProof": plan.BeyondProof}
}

func write(t *testing.T, path string, v any) {
	t.Helper()
	var b []byte
	if s, ok := v.(string); ok {
		b = []byte(s)
	} else {
		b = must(json.Marshal(v))
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func goCLI(t *testing.T, ws []string, argv ...string) (int, map[string]any) {
	t.Helper()
	code, body := run(context.Background(), append(argv, ws...))
	var out map[string]any
	if err := json.Unmarshal([]byte(lastro.StringifyIndent(body)), &out); err != nil {
		t.Fatal(err)
	}
	return code, out
}

func TestGoCLIAlone(t *testing.T) {
	dir := t.TempDir()
	ws := []string{"--dir", filepath.Join(dir, ".lastro"), "--llm", "handoff"}
	write(t, filepath.Join(dir, "model.json"), login.Model)
	write(t, filepath.Join(dir, "prose.md"), login.Prose)

	if code, _ := goCLI(t, ws, "init", "--model", filepath.Join(dir, "model.json"), "--approvals", "2"); code != 0 {
		t.Fatal("init")
	}
	code, body := goCLI(t, ws, "plan", filepath.Join(dir, "prose.md"))
	if code != 3 || body["status"] != "needs-llm" {
		t.Fatalf("plan: %d %v", code, body)
	}
	write(t, body["answer"].(string), extractAnswer())
	if code, _ := goCLI(t, ws, "plan", filepath.Join(dir, "prose.md")); code != 1 {
		t.Fatal("planned, review pending")
	}
	if _, body := goCLI(t, ws, "spec"); body["status"] != "review-pending" {
		t.Fatal(body)
	}
	goCLI(t, ws, "review", "--as", "ana", "approve-all")
	goCLI(t, ws, "review", "--as", "ana", "amend", "I5", login.FixedI5)
	goCLI(t, ws, "review", "--as", "ana", "answer", "D1", "0")
	goCLI(t, ws, "review", "--as", "ana", "answer", "D2", "0")
	if code, body := goCLI(t, ws, "status"); code != 1 || !strings.Contains(strings.Join(toStrings(body["pending"]), ";"), "I1 needs 1 more approval") {
		t.Fatalf("a policy of two approvals: %v", body["pending"])
	}
	goCLI(t, ws, "review", "--as", "bruno", "approve-all")
	if code, _ := goCLI(t, ws, "status"); code != 0 {
		t.Fatal("status")
	}
	code, body = goCLI(t, ws, "spec")
	if code != 3 {
		t.Fatal(body)
	}
	var art lastro.IntentArtifact
	must(0, readJSON(filepath.Join(dir, ".lastro", "intents.json"), &art))
	write(t, body["answer"].(string), deriveAnswer(must(lastro.Freeze(art))))
	if code, body := goCLI(t, ws, "spec"); code != 0 {
		t.Fatal(body)
	}

	// The code under proof as a recorded table.
	plan := login.DerivePlan(must(lastro.Freeze(art)))
	write(t, filepath.Join(dir, "good.json"), must(lastro.Tabulate(plan.Model, login.Decide(""), 0)))
	write(t, filepath.Join(dir, "bad.json"), must(lastro.Tabulate(plan.Model, login.Decide("admin-everywhere"), 0)))
	if code, body := goCLI(t, ws, "prove", "--table", filepath.Join(dir, "good.json")); code != 0 {
		t.Fatal(body)
	}
	code, body = goCLI(t, ws, "prove", "--table", filepath.Join(dir, "bad.json"))
	if code != 1 || !strings.Contains(body["feedback"].(string), "counterexample") {
		t.Fatal(body)
	}
	if code, _ := goCLI(t, ws, "feedback"); code != 1 {
		t.Fatal("feedback")
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

// TestSharedWorkspaceWithTheTSCLI runs one .lastro/ workspace through both CLIs: the TS CLI plans,
// the Go CLI reviews and derives the spec, both prove the same spec.
func TestSharedWorkspaceWithTheTSCLI(t *testing.T) {
	sdk := must(filepath.Abs(filepath.Join("..", "..", "..", "typescript")))
	cli := filepath.Join(sdk, "dist", "src", "cli.js")
	loginJS := filepath.Join(sdk, "dist", "examples", "login", "login.js")
	if _, err := os.Stat(loginJS); err != nil {
		t.Skip("TS SDK not built: run `npm run build -w lastro`")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}
	dir := t.TempDir()
	ws := []string{"--dir", filepath.Join(dir, ".lastro"), "--llm", "handoff"}
	ts := func(argv ...string) (int, map[string]any) {
		t.Helper()
		cmd := exec.Command("node", append([]string{cli}, append(argv, ws...)...)...)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(out, &body); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return code, body
	}
	write(t, filepath.Join(dir, "model.json"), login.Model)
	write(t, filepath.Join(dir, "prose.md"), login.Prose)
	write(t, filepath.Join(dir, "impl.mjs"), "import { makeLogin } from "+string(must(json.Marshal(loginJS)))+";\nexport default makeLogin();\nexport const buggy = makeLogin(\"skip-password\");\n")
	// The same TS implementation as a JSON-lines program, for the Go CLI.
	write(t, filepath.Join(dir, "subject.mjs"), `import { makeLogin } from `+string(must(json.Marshal(loginJS)))+`;
import { createInterface } from "node:readline";
const decide = makeLogin(process.argv[2] || undefined);
for await (const line of createInterface({ input: process.stdin })) process.stdout.write(JSON.stringify(decide(JSON.parse(line))) + "\n");
`)

	// TS: init and plan (the agent answers the extract handoff).
	if code, body := ts("init", "--model", filepath.Join(dir, "model.json")); code != 0 {
		t.Fatal(body)
	}
	code, body := ts("plan", filepath.Join(dir, "prose.md"))
	if code != 3 {
		t.Fatal(body)
	}
	// The Go CLI asks the very same question (same request id).
	if _, goBody := goCLI(t, ws, "plan", filepath.Join(dir, "prose.md")); goBody["answer"] != body["answer"] {
		t.Fatalf("handoff ids differ: go %v, ts %v", goBody["answer"], body["answer"])
	}
	write(t, body["answer"].(string), extractAnswer())
	if code, _ := ts("plan", filepath.Join(dir, "prose.md")); code != 1 {
		t.Fatal("TS plan")
	}

	// Go: review what the TS CLI planned.
	goCLI(t, ws, "review", "--as", "ana", "approve-all")
	goCLI(t, ws, "review", "--as", "ana", "amend", "I5", login.FixedI5)
	goCLI(t, ws, "review", "--as", "ana", "answer", "D1", "0")
	goCLI(t, ws, "review", "--as", "ana", "answer", "D2", "0")
	if code, _ := ts("status"); code != 0 {
		t.Fatal("the TS CLI should see the Go review as complete")
	}

	// Go asks for the derivation; the TS CLI asks the same question.
	code, goBody := goCLI(t, ws, "spec")
	if code != 3 {
		t.Fatal(goBody)
	}
	if _, tsBody := ts("spec"); tsBody["answer"] != goBody["answer"] {
		t.Fatalf("derive handoff ids differ: go %v, ts %v", goBody["answer"], tsBody["answer"])
	}
	var art lastro.IntentArtifact
	must(0, readJSON(filepath.Join(dir, ".lastro", "intents.json"), &art))
	write(t, goBody["answer"].(string), deriveAnswer(must(lastro.Freeze(art))))
	if code, body := goCLI(t, ws, "spec"); code != 0 {
		t.Fatal(body)
	}

	// Both CLIs prove the spec the Go CLI wrote, against the same TS implementation.
	if code, body := ts("prove", filepath.Join(dir, "impl.mjs")); code != 0 {
		t.Fatalf("TS prove of a Go spec: %v", body)
	}
	if code, body := ts("prove", filepath.Join(dir, "impl.mjs"), "--export", "buggy"); code != 1 {
		t.Fatal(body)
	}
	code, body = goCLI(t, ws, "prove", "--cmd", "node "+filepath.Join(dir, "subject.mjs"))
	if code != 0 {
		t.Fatalf("Go prove of a JS program: %v", body)
	}
	if lastro.FindRustProver() != nil && body["enginesAgree"] != true {
		t.Fatal("engines should agree")
	}
	code, body = goCLI(t, ws, "prove", "--cmd", "node "+filepath.Join(dir, "subject.mjs")+" skip-password")
	if code != 1 || !strings.Contains(body["feedback"].(string), `"password":"wrong"`) {
		t.Fatal(body)
	}
	// The TS CLI reads the proof the Go CLI wrote.
	if code, body := ts("feedback"); code != 1 || !strings.Contains(body["feedback"].(string), "counterexample") {
		t.Fatal(body)
	}
}
