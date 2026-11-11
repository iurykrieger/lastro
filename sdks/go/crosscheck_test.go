package lastro_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
	"github.com/iurykrieger/lastro/sdks/go/examples/login"
)

// The Go SDK must agree with the TypeScript SDK byte for byte where it matters:
// artifact and plan hashes, prompts, and every verification report. This test
// runs the TS SDK (sdks/typescript/dist, built by `npm test -w lastro` or
// `npm run build -w lastro`) on the login example and compares.

const tsScript = `
import * as L from "./dist/src/index.js";
import { BUGS, FIXED_I5, deriveLoginPlan, extractLogin, makeLogin, MODEL } from "./dist/examples/login/login.js";
import { sha256Hex } from "./dist/src/sha256.js";
const frozen = L.freeze(L.review(extractLogin()).approveAll().answer("D1", 0).answer("D2", 0).amend("I5", FIXED_I5).done());
const plan = deriveLoginPlan(frozen);
const reports = {};
for (const bug of ["", ...BUGS.map((b) => b.id)]) reports[bug] = await L.verify(frozen, plan, { subject: makeLogin(bug || undefined) });
const traces = [{ ref: "req-123", input: { sends: "email", accountFound: true, password: "wrong", infra: "ok" }, output: { status: 200, token: true, role: "customer" } }];
reports["trace"] = await L.verify(frozen, plan, { subject: makeLogin(), traces });
// Sampling: same seed, same samples, same verdicts.
const tiny = L.freeze(L.review(L.createArtifact("x doubles", [{ statement: "y = 2x", kind: "behavior", provability: "formal", quote: "x doubles" }])).approveAll().done());
const tinyPlan = { lastro: "proof-plan/v0", artifactHash: tiny.hash, model: { name: "double", inputs: { x: L.D.int(0, 1000000), s: L.D.string("a", "b") }, outputs: { y: L.D.int(-1000, 10000000) } },
  clauses: [{ id: "C1", intentIds: ["I1"], description: "y = 2x", ensures: L.L.eq(L.L.out("y"), { op: "mul", args: [L.L.c(2), L.L.in("x")] }) },
            { id: "C2", intentIds: ["I1"], description: "y small", requires: L.L.eq(L.L.in("s"), "a"), ensures: L.L.lt(L.L.out("y"), 1000000) }],
  assumptions: [], scenarios: [], beyondProof: [] };
const sampled = await L.verify(tiny, tinyPlan, { subject: (i) => ({ y: 2 * i.x }), samples: 500, seed: 7 });
const ask = (step, system, user, schema) => step + "-" + sha256Hex(JSON.stringify({ step, system, user, schema })).slice(0, 12);
const deriveUser = L.deriveUserPrompt(frozen, MODEL);
const receipt = L.renderReceipt(reports["skip-password"], plan);
process.stdout.write(JSON.stringify({
  frozen, plan, planHash: L.hashOf(plan), reports, tiny, tinyPlan, sampled, receipt,
  prompts: { extractSystem: L.EXTRACT_SYSTEM, deriveSystem: L.DERIVE_SYSTEM, gapSystem: L.GAP_REVIEW_SYSTEM,
             extractSchema: JSON.stringify(L.EXTRACT_SCHEMA), deriveSchema: JSON.stringify(L.DERIVE_SCHEMA), gapSchema: JSON.stringify(L.GAP_REVIEW_SCHEMA),
             extractUser: L.extractUserPrompt(frozen.prose), deriveUser,
             extractId: ask("extract", L.EXTRACT_SYSTEM, L.extractUserPrompt(frozen.prose), L.EXTRACT_SCHEMA),
             deriveId: ask("derive", L.DERIVE_SYSTEM, deriveUser, L.DERIVE_SCHEMA) },
}));
`

type tsOut struct {
	Frozen   lastro.IntentArtifact      `json:"frozen"`
	Plan     lastro.ProofPlan           `json:"plan"`
	PlanHash string                     `json:"planHash"`
	Reports  map[string]json.RawMessage `json:"reports"`
	Tiny     lastro.IntentArtifact      `json:"tiny"`
	TinyPlan lastro.ProofPlan           `json:"tinyPlan"`
	Sampled  json.RawMessage            `json:"sampled"`
	Receipt  string                     `json:"receipt"`
	Prompts  map[string]string          `json:"prompts"`
}

func sdkDir(t *testing.T) string {
	dir, err := filepath.Abs(filepath.Join("..", "typescript"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "examples", "login", "login.js")); err != nil {
		t.Skip("TS SDK not built: run `npm run build -w lastro`")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found")
	}
	return dir
}

func runTS(t *testing.T) tsOut {
	cmd := exec.Command("node", "--input-type=module", "-e", tsScript)
	cmd.Dir = sdkDir(t)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var ts tsOut
	if err := json.Unmarshal(out, &ts); err != nil {
		t.Fatal(err)
	}
	return ts
}

// canon re-reads JSON (from either SDK) and prints it canonically.
func canon(t *testing.T, v any) string {
	t.Helper()
	if raw, ok := v.(json.RawMessage); ok {
		var x any
		if err := json.Unmarshal(raw, &x); err != nil {
			t.Fatal(err)
		}
		v = x
	}
	return must(lastro.CanonicalJSON(v))
}

func TestTypeScriptCrossCheck(t *testing.T) {
	ts := runTS(t)

	t.Run("artifact hash", func(t *testing.T) {
		if !lastro.IsIntact(ts.Frozen) {
			t.Fatal("an artifact frozen by the TS SDK is not intact for the Go SDK")
		}
		goFrozen := must(lastro.Freeze(must(lastro.NewReview(extract(t), "").ApproveAll().Answer("D1", 0).Answer("D2", 0).Amend("I5", login.FixedI5).Done())))
		if goFrozen.Hash != ts.Frozen.Hash {
			t.Fatalf("Go froze %s, TS froze %s", goFrozen.Hash, ts.Frozen.Hash)
		}
	})

	t.Run("plan hash", func(t *testing.T) {
		if h := must(lastro.HashOf(ts.Plan)); h != ts.PlanHash {
			t.Fatalf("TS plan read by Go hashes to %s, TS says %s", h, ts.PlanHash)
		}
		if h := must(lastro.HashOf(login.DerivePlan(ts.Frozen))); h != ts.PlanHash {
			t.Fatalf("the Go port of the login plan differs from the TS one: %s vs %s", h, ts.PlanHash)
		}
	})

	t.Run("reports agree on every bug", func(t *testing.T) {
		for _, bug := range append([]string{""}, login.Bugs...) {
			got := must(lastro.Verify(ts.Frozen, ts.Plan, lastro.VerifyOptions{Subject: login.Decide(bug)}))
			if g, w := canon(t, got), canon(t, ts.Reports[bug]); g != w {
				t.Fatalf("bug %q:\n go %s\n ts %s", bug, g, w)
			}
		}
		traces := []lastro.Trace{{Ref: "req-123", Input: lastro.Env{"sends": "email", "accountFound": true, "password": "wrong", "infra": "ok"}, Output: lastro.Env{"status": 200, "token": true, "role": "customer"}}}
		got := must(lastro.Verify(ts.Frozen, ts.Plan, lastro.VerifyOptions{Subject: login.Decide(""), Traces: traces}))
		if g, w := canon(t, got), canon(t, ts.Reports["trace"]); g != w {
			t.Fatalf("trace:\n go %s\n ts %s", g, w)
		}
	})

	t.Run("seeded sampling", func(t *testing.T) {
		seed := uint32(7)
		got := must(lastro.Verify(ts.Tiny, ts.TinyPlan, lastro.VerifyOptions{Samples: 500, Seed: &seed, Subject: func(in lastro.Env) (lastro.Env, error) {
			return lastro.Env{"y": 2 * in["x"].(float64)}, nil
		}}))
		if g, w := canon(t, got), canon(t, ts.Sampled); g != w {
			t.Fatalf("\n go %s\n ts %s", g, w)
		}
	})

	t.Run("receipt", func(t *testing.T) {
		var rep lastro.VerificationReport
		if err := json.Unmarshal(ts.Reports["skip-password"], &rep); err != nil {
			t.Fatal(err)
		}
		plan := ts.Plan
		if got := lastro.RenderReceipt(rep, &plan, nil); got != ts.Receipt {
			t.Fatalf("receipt differs:\n%s\n---\n%s", got, ts.Receipt)
		}
	})

	t.Run("prompts and handoff ids", func(t *testing.T) {
		p := ts.Prompts
		for name, pair := range map[string][2]string{
			"extract system": {lastro.ExtractSystem, p["extractSystem"]}, "derive system": {lastro.DeriveSystem, p["deriveSystem"]},
			"gap system": {lastro.GapReviewSystem, p["gapSystem"]}, "extract schema": {lastro.ExtractSchema, p["extractSchema"]},
			"derive schema": {lastro.DeriveSchema, p["deriveSchema"]}, "gap schema": {lastro.GapReviewSchema, p["gapSchema"]},
			"extract user": {lastro.ExtractUserPrompt(ts.Frozen.Prose, ""), p["extractUser"]},
			"derive user":  {must(lastro.DeriveUserPrompt(ts.Frozen, login.Model)), p["deriveUser"]},
		} {
			if pair[0] != pair[1] {
				t.Errorf("%s differs:\n go %q\n ts %q", name, pair[0], pair[1])
			}
		}
		user := must(lastro.DeriveUserPrompt(ts.Frozen, login.Model))
		if id := lastro.HandoffID(lastro.AskRequest{Step: "derive", System: lastro.DeriveSystem, User: user, Schema: json.RawMessage(lastro.DeriveSchema)}); id != p["deriveId"] {
			t.Errorf("derive handoff id %s, TS %s", id, p["deriveId"])
		}
		if id := lastro.HandoffID(lastro.AskRequest{Step: "extract", System: lastro.ExtractSystem, User: lastro.ExtractUserPrompt(ts.Frozen.Prose, ""), Schema: json.RawMessage(lastro.ExtractSchema)}); id != p["extractId"] {
			t.Errorf("extract handoff id %s, TS %s", id, p["extractId"])
		}
	})

	t.Run("spec from a TS artifact proves in Go", func(t *testing.T) {
		l := lastro.New(lastro.Options{Model: login.Model, Deriver: lastro.DeriverFunc(func(_ context.Context, a lastro.IntentArtifact, _ lastro.Model) (lastro.ProofPlan, error) {
			return login.DerivePlan(a), nil
		})})
		spec := must(l.Spec(context.Background(), ts.Frozen))
		// Like the TS facade, Spec adds the policy when the artifact has none, so the hash moves; it must still be intact.
		if !lastro.IsIntact(spec.Artifact) {
			t.Fatal("not intact")
		}
		if p := must(l.Prove(context.Background(), spec, login.Decide(""), nil)); !p.OK {
			t.Fatal(p.Receipt)
		}
	})
}
