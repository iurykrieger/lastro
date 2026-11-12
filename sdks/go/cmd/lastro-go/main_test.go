package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// The shipping example of the docs: prose, model, and the answers an agent writes to the two handoffs.
var example = filepath.Join("..", "..", "..", "..", "apps", "docs", "examples", "shipping")

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.WriteFile(path, must(json.Marshal(v)), 0o644); err != nil {
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

// shipping is the decision core; buggy charges an order of exactly 100.
func shipping(buggy bool) lastro.Subject {
	return func(in lastro.Env) (lastro.Env, error) {
		sub, member := in["subtotal"].(float64), in["member"].(bool)
		free := sub >= 100
		if buggy {
			free = sub > 100
		}
		if member || free {
			return lastro.Env{"fee": 0.0}, nil
		}
		return lastro.Env{"fee": 15.0}, nil
	}
}

func tables(t *testing.T, dir string) (good, bad string) {
	var model lastro.Model
	must(0, readJSON(filepath.Join(example, "model.json"), &model))
	good, bad = filepath.Join(dir, "good.json"), filepath.Join(dir, "bad.json")
	write(t, good, must(lastro.Tabulate(model, shipping(false), 0)))
	write(t, bad, must(lastro.Tabulate(model, shipping(true), 0)))
	return good, bad
}

func ids(v any) []string {
	out := []string{}
	for _, x := range v.([]any) {
		switch y := x.(type) {
		case string:
			out = append(out, y)
		case map[string]any:
			if id, ok := y["id"].(string); ok {
				out = append(out, id)
			} else {
				out = append(out, y["proof"].(string))
			}
		}
	}
	return out
}

func TestGoCLIAlone(t *testing.T) {
	dir := t.TempDir()
	ws := []string{"--dir", filepath.Join(dir, ".lastro"), "--llm", "handoff"}
	if code, body := goCLI(t, ws, "init", "--model", filepath.Join(example, "model.json"), "--approvals", "2"); code != 0 || body["review"].(map[string]any)["approvals"] != 2.0 {
		t.Fatal(body)
	}
	if code, body := goCLI(t, ws, "plan", "x.md"); code != 2 || !strings.Contains(body["error"].(string), "was replaced") {
		t.Fatal(body)
	}
	code, body := goCLI(t, ws, "propose", filepath.Join(example, "prose.md"), "--as", "ana")
	if code != 3 || body["step"] != "extract" {
		t.Fatalf("propose: %d %v", code, body)
	}
	copyFile(t, filepath.Join(example, "answers", "propose.json"), body["answer"].(string))
	code, body = goCLI(t, ws, "propose", filepath.Join(example, "prose.md"), "--as", "ana")
	if code != 1 || body["proposal"].(map[string]any)["id"] != "P-001" {
		t.Fatalf("proposed, review pending: %v", body)
	}
	if got := ids(body["intents"]); !reflect.DeepEqual(got, []string{"P-001.I-01", "P-001.I-02", "P-001.I-03", "P-001.I-04"}) {
		t.Fatal(got)
	}
	if _, body := goCLI(t, ws, "derive", "P-001"); body["status"] != "review-pending" {
		t.Fatal(body)
	}
	goCLI(t, ws, "review", "P-001", "--as", "ana", "answer", "D1", "1")
	goCLI(t, ws, "review", "P-001", "--as", "ana", "comment", "I-03", "Is 15 the final fee?")
	if _, body := goCLI(t, ws, "review", "P-001", "--as", "bruno", "resolve", "T1"); body["status"] != nil {
		t.Fatalf("only the opener or the submitter resolves: %v", body)
	}
	goCLI(t, ws, "review", "P-001", "--as", "ana", "reply", "T1", "Yes.")
	goCLI(t, ws, "review", "P-001", "--as", "ana", "resolve", "T1")
	goCLI(t, ws, "review", "P-001", "--as", "ana", "approve-all")
	code, body = goCLI(t, ws, "status", "P-001")
	if code != 1 || !strings.Contains(strings.Join(ids(body["pending"]), ";"), "P-001.I-01 needs 1 more approval(s)") {
		t.Fatalf("a policy of two approvals: %v", body["pending"])
	}
	if _, body := goCLI(t, ws, "review", "1", "--as", "bruno", "approve-all"); body["proposal"].(map[string]any)["approved"] != true {
		t.Fatal(body)
	}
	code, body = goCLI(t, ws, "derive", "P-001")
	if code != 3 || body["step"] != "derive" {
		t.Fatal(body)
	}
	copyFile(t, filepath.Join(example, "answers", "proofs.json"), body["answer"].(string))
	if code, body := goCLI(t, ws, "derive", "P-001"); code != 0 || len(ids(body["proofs"])) != 4 {
		t.Fatal(body)
	}

	good, bad := tables(t, dir)
	code, body = goCLI(t, ws, "prove", "P-001", "--table", bad)
	if code != 1 || !reflect.DeepEqual(ids(body["failures"]), []string{"P-001.PF-01"}) || !strings.Contains(body["feedback"].(string), `counterexample: {"member":false,"subtotal":100} returned {"fee":15}`) {
		t.Fatal(body)
	}
	if code, _ := goCLI(t, ws, "apply", "P-001"); code != 1 {
		t.Fatal("apply refuses a failed run")
	}
	if code, body := goCLI(t, ws, "prove", "P-001", "--table", good); code != 0 || body["feedback"] != "Every proof passes." {
		t.Fatal(body)
	}
	runs := strings.Split(strings.TrimSpace(string(must(os.ReadFile(filepath.Join(dir, ".lastro", "proposals", "P-001-shipping-fee.runs.jsonl"))))), "\n")
	if len(runs) != 2 || !strings.HasPrefix(runs[1], `{"run":2,"at":"`) {
		t.Fatalf("one line per run: %v", runs)
	}
	code, body = goCLI(t, ws, "apply", "P-001")
	if code != 0 || !reflect.DeepEqual(ids(body["written"]), []string{"UC-001"}) {
		t.Fatal(body)
	}
	var u lastro.UseCase
	must(0, readJSON(filepath.Join(dir, ".lastro", "usecases", "UC-001-charge-shipping.json"), &u))
	if len(u.Proofs) != 4 || !reflect.DeepEqual(u.History, []string{"P-001"}) {
		t.Fatal(u)
	}
	if code, body := goCLI(t, ws, "regress", "--table", bad); code != 1 || !reflect.DeepEqual(ids(body["regressions"]), []string{"P-001.PF-01"}) {
		t.Fatal(body)
	}
	if code, body := goCLI(t, ws, "status"); code != 0 || body["proofs"] != 4.0 {
		t.Fatal(body)
	}
}

func TestOldWorkspacesAreRefused(t *testing.T) {
	dir := t.TempDir()
	ws := []string{"--dir", dir}
	write(t, filepath.Join(dir, "intents.json"), map[string]any{})
	if code, body := goCLI(t, ws, "status"); code != 2 || !strings.Contains(body["error"].(string), "old workspace format") {
		t.Fatal(body)
	}
}

// TestSharedWorkspaceWithTheTSCLI runs one .lastro/ workspace through both CLIs: they ask the same
// handoff questions, print the same summaries, compute the same hashes and write the same receipts.
func TestSharedWorkspaceWithTheTSCLI(t *testing.T) {
	cli := must(filepath.Abs(filepath.Join("..", "..", "..", "typescript", "dist", "src", "cli.js")))
	if _, err := os.Stat(cli); err != nil {
		t.Skip("TS SDK not built: run `npm run build -w lastro`")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}
	dir := t.TempDir()
	lastroDir := filepath.Join(dir, ".lastro")
	ws := []string{"--dir", lastroDir, "--llm", "handoff"}
	ts := func(argv ...string) (int, map[string]any) {
		t.Helper()
		cmd := exec.Command("node", append([]string{cli}, append(argv, ws...)...)...)
		cmd.Dir = dir
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
	same := func(what string, a, b map[string]any) {
		t.Helper()
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("%s differs:\nTS %s\nGo %s", what, must(json.Marshal(a)), must(json.Marshal(b)))
		}
	}
	for _, f := range []string{"model.json", "prose.md", "shipping.mjs", "shipping-buggy.mjs"} {
		copyFile(t, filepath.Join(example, f), filepath.Join(dir, f))
	}
	// The same JS implementation as a JSON-lines program, for the Go CLI.
	subject := func(module string) string {
		path := filepath.Join(dir, "lines-"+module)
		os.WriteFile(path, []byte(`import decide from "./`+module+`";
import { createInterface } from "node:readline";
for await (const line of createInterface({ input: process.stdin })) process.stdout.write(JSON.stringify(decide(JSON.parse(line))) + "\n");
`), 0o644)
		return "node " + path
	}

	if code, body := ts("init", "--model", filepath.Join(dir, "model.json"), "--subject", "./shipping.mjs"); code != 0 {
		t.Fatal(body)
	}
	_, goInit := goCLI(t, ws, "init", "--model", filepath.Join(dir, "model.json"))
	_, tsInit := ts("init", "--model", filepath.Join(dir, "model.json"))
	same("init", tsInit, goInit)

	// Both CLIs ask the very same extract question.
	code, tsBody := ts("propose", filepath.Join(dir, "prose.md"), "--as", "ana")
	if code != 3 {
		t.Fatal(tsBody)
	}
	_, goBody := goCLI(t, ws, "propose", filepath.Join(dir, "prose.md"), "--as", "ana")
	same("the propose handoff", tsBody, goBody)
	copyFile(t, filepath.Join(example, "answers", "propose.json"), tsBody["answer"].(string))
	if code, _ := ts("propose", filepath.Join(dir, "prose.md"), "--as", "ana"); code != 1 {
		t.Fatal("TS propose")
	}
	_, a := ts("status", "P-001")
	_, b := goCLI(t, ws, "status", "P-001")
	same("status of a TS proposal", a, b)

	// Go reviews what the TS CLI proposed; the TS CLI sees the approval as valid (same hash).
	goCLI(t, ws, "review", "P-001", "--as", "ana", "answer", "D1", "1")
	if _, body := goCLI(t, ws, "review", "P-001", "--as", "ana", "approve-all"); body["proposal"].(map[string]any)["approved"] != true {
		t.Fatal(body)
	}
	_, a = ts("status", "P-001")
	_, b = goCLI(t, ws, "status", "P-001")
	same("status of a Go-approved proposal", a, b)
	if a["proposal"].(map[string]any)["status"] != "approved" {
		t.Fatalf("the TS CLI recomputes the Go hash: %v", a["proposal"])
	}

	// Both ask the same derive question; the TS CLI derives, the Go CLI proves.
	_, tsBody = ts("derive", "P-001")
	_, goBody = goCLI(t, ws, "derive", "P-001")
	same("the derive handoff", tsBody, goBody)
	copyFile(t, filepath.Join(example, "answers", "proofs.json"), tsBody["answer"].(string))
	if code, body := ts("derive", "P-001"); code != 0 {
		t.Fatal(body)
	}
	receipt := filepath.Join(lastroDir, "proposals", "P-001-shipping-fee.receipt.md")
	read := func() string { return string(must(os.ReadFile(receipt))) }
	rust := lastro.FindRustProver() != nil
	_, tsBody = ts("prove", "P-001", "./shipping-buggy.mjs")
	tsReceipt := read()
	code, goBody = goCLI(t, ws, "prove", "P-001", "--cmd", subject("shipping-buggy.mjs"))
	if code != 1 {
		t.Fatal(goBody)
	}
	engines := regexp.MustCompile(` Engines agree: \w+\.`)
	if rust {
		same("prove (failing)", tsBody, goBody)
		if engines.ReplaceAllString(tsReceipt, "") != engines.ReplaceAllString(read(), "") {
			t.Fatalf("receipts differ:\nTS\n%s\nGo\n%s", tsReceipt, read())
		}
	}
	_, a = ts("feedback", "P-001")
	_, b = goCLI(t, ws, "feedback", "P-001")
	same("feedback", a, b)

	// prove --stream: the same events (progress and crosscheck aside: they depend on the subject and the provers found).
	tsOut, _ := exec.Command("node", append([]string{cli, "prove", "P-001", filepath.Join(dir, "shipping-buggy.mjs"), "--stream"}, ws...)...).Output()
	var goLines []string
	_, goResult := runWith(context.Background(), append([]string{"prove", "P-001", "--stream", "--cmd", subject("shipping-buggy.mjs")}, ws...), func(e obj) { goLines = append(goLines, lastro.Stringify(e)) })
	goLines = append(goLines, lastro.Stringify(append(obj{{"event", "result"}}, goResult...)))
	events := func(lines []string) []map[string]any {
		out := []map[string]any{}
		for _, l := range lines {
			var e map[string]any
			if l == "" || json.Unmarshal([]byte(l), &e) != nil || e["event"] == "progress" || e["event"] == "crosscheck" {
				continue
			}
			delete(e, "enginesAgree")
			delete(e, "receipt")
			out = append(out, e)
		}
		return out
	}
	if te, ge := events(strings.Split(string(tsOut), "\n")), events(goLines); !reflect.DeepEqual(te, ge) {
		t.Fatalf("streams differ:\nTS %s\nGo %s", must(json.Marshal(te)), must(json.Marshal(ge)))
	}

	// Go proves the fixed code; the TS CLI applies what the Go CLI proved.
	if code, body := goCLI(t, ws, "prove", "P-001", "--cmd", subject("shipping.mjs")); code != 0 {
		t.Fatal(body)
	}
	if code, body := ts("apply", "P-001"); code != 0 {
		t.Fatalf("TS apply of a Go run: %v", body)
	}
	_, a = ts("status")
	_, b = goCLI(t, ws, "status")
	same("status after apply", a, b)
	if code, body := goCLI(t, ws, "regress", "--cmd", subject("shipping.mjs")); code != 0 || len(body["failures"].([]any)) != 0 {
		t.Fatal(body)
	}
}
