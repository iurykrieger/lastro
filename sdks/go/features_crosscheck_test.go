package lastro_test

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"testing"

	lastro "github.com/iurykrieger/lastro/sdks/go"
)

// The feature map and the snapshot helpers must agree with the TS SDK: same maps, same errors, same text, same
// example inputs and contracts (their files are hashed, so the bytes must match).
const featuresScript = `
import * as L from "./dist/src/index.js";
const cases = JSON.parse(process.argv[1]);
const maps = cases.map(({ features, removed, changed }) => {
  try {
    const m = L.featureMap(features, removed);
    const affected = L.affectedBy(m, changed);
    return { map: m, affected, text: L.renderFeatureMap(m, affected) };
  } catch (e) {
    return { error: e.message, cycle: e.cycle ?? [] };
  }
});
const big = { name: "big", inputs: { a: L.D.int(0, 99), b: L.D.int(0, 99) }, outputs: { y: L.D.enum("x", 2, true) } };
const small = { name: "small", inputs: { member: L.D.bool(), subtotal: L.D.int(0, 3) }, outputs: { fee: L.D.int(0, 15) } };
const examples = {
  big: L.exampleInputs(big, [{ b: 1, a: 2 }]),
  small: L.exampleInputs(small, [{ subtotal: 2, member: true }]),
};
const contracts = { big: L.modelContract(big), small: L.modelContract(small) };
const diff = L.structuralDiff({ a: "int", b: "string", c: "bool" }, { a: "int", b: "number", d: "string" });
const path = [L.snapshotPath("P-006", "after", "examples-a b.json", { feature: "UC-001", intent: "P-006.I-03" }), L.snapshotPath("P-006", "before", "data.json")];
process.stdout.write(JSON.stringify({ maps, examples, contracts, diff, path }));
`

func TestFeatureMapAndSnapshotsMatchTS(t *testing.T) {
	dir := sdkDir(t)
	s := func(kind, name, loc string) lastro.Surface {
		return lastro.Surface{Kind: kind, Name: name, Locator: loc}
	}
	type mapCase struct {
		Features []lastro.Linked `json:"features"`
		Removed  []string        `json:"removed"`
		Changed  []string        `json:"changed"`
	}
	cases := []mapCase{
		{Features: []lastro.Linked{
			{ID: "UC-003", Slug: "checkout", Name: "Checkout", Surfaces: []lastro.Surface{s("screen", "Checkout", "/checkout"), s("api", "Pay", "POST /pay")}, DependsOn: []string{"UC-001", "UC-002"}},
			{ID: "UC-001", Slug: "login", Name: "Login", Surfaces: []lastro.Surface{s("route", "Login", "/login")}},
			{ID: "UC-002", Slug: "cart", Name: "Cart", DependsOn: []string{"UC-001"}},
			{ID: "UC-004", Slug: "receipt", Name: "Receipt", DependsOn: []string{"UC-003"}},
		}, Removed: []string{}, Changed: []string{"UC-001"}},
		{Features: []lastro.Linked{
			{ID: "UC-001", Slug: "a", Name: "A", DependsOn: []string{"UC-002"}},
			{ID: "UC-002", Slug: "b", Name: "B", DependsOn: []string{"UC-003"}},
			{ID: "UC-003", Slug: "c", Name: "C", DependsOn: []string{"UC-002", "UC-009"}},
		}, Removed: []string{}, Changed: []string{}},
		{Features: []lastro.Linked{
			{ID: "UC-001", Slug: "a", Name: "A", Surfaces: []lastro.Surface{s("page", "", "")}},
		}, Removed: []string{}, Changed: []string{}},
		{Features: []lastro.Linked{
			{ID: "UC-001", Slug: "a", Name: "A", DependsOn: []string{"UC-002"}},
			{ID: "UC-002", Slug: "b", Name: "B"},
		}, Removed: []string{"UC-002"}, Changed: []string{"UC-002"}},
		{Features: []lastro.Linked{}, Removed: []string{}, Changed: []string{}},
	}
	in, _ := json.Marshal(cases)
	cmd := exec.Command("node", "--input-type=module", "-e", featuresScript, string(in))
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var ts struct {
		Maps     []json.RawMessage `json:"maps"`
		Examples map[string]struct {
			Inputs     []json.RawMessage `json:"inputs"`
			WholeSpace bool              `json:"wholeSpace"`
		} `json:"examples"`
		Contracts map[string]json.RawMessage `json:"contracts"`
		Diff      json.RawMessage            `json:"diff"`
		Path      []string                   `json:"path"`
	}
	if err := json.Unmarshal(out, &ts); err != nil {
		t.Fatal(err)
	}
	for k, c := range cases {
		var got any
		m, err := lastro.BuildFeatureMap(c.Features, c.Removed)
		var fe *lastro.FeatureMapError
		if errors.As(err, &fe) {
			cycle := fe.Cycle
			if cycle == nil {
				cycle = []string{}
			}
			got = map[string]any{"error": fe.Msg, "cycle": cycle}
		} else if err != nil {
			t.Fatal(err)
		} else {
			affected := lastro.AffectedBy(m, c.Changed)
			got = map[string]any{"map": m, "affected": affected, "text": lastro.RenderFeatureMap(m, affected)}
		}
		if g, w := canonOf(t, got), canonOf(t, ts.Maps[k]); g != w {
			t.Errorf("map case %d:\nTS %s\nGo %s", k, w, g)
		}
	}
	big := lastro.Model{Name: "big", Inputs: lastro.Fields{{Name: "a", Domain: lastro.Int(0, 99)}, {Name: "b", Domain: lastro.Int(0, 99)}}, Outputs: lastro.Fields{{Name: "y", Domain: lastro.Enum("x", 2, true)}}}
	small := lastro.Model{Name: "small", Inputs: lastro.Fields{{Name: "member", Domain: lastro.Bool()}, {Name: "subtotal", Domain: lastro.Int(0, 3)}}, Outputs: lastro.Fields{{Name: "fee", Domain: lastro.Int(0, 15)}}}
	for name, c := range map[string]struct {
		m    lastro.Model
		scen string
	}{"big": {big, `{"b":1,"a":2}`}, "small": {small, `{"subtotal":2,"member":true}`}} {
		inputs, whole, err := lastro.ExampleInputs(c.m, []json.RawMessage{json.RawMessage(c.scen)}, 42)
		if err != nil {
			t.Fatal(err)
		}
		want := ts.Examples[name]
		if whole != want.WholeSpace || len(inputs) != len(want.Inputs) {
			t.Fatalf("%s examples: whole %v/%v, %d/%d inputs", name, whole, want.WholeSpace, len(inputs), len(want.Inputs))
		}
		for i := range inputs {
			// The examples file keeps the key order: compare the exact JSON text.
			if lastro.Stringify(inputs[i]) != lastro.Stringify(want.Inputs[i]) {
				t.Fatalf("%s example %d: TS %s, Go %s", name, i, want.Inputs[i], inputs[i])
			}
		}
		if g, w := lastro.Stringify(lastro.ModelContract(c.m)), lastro.Stringify(ts.Contracts[name]); g != w {
			t.Errorf("%s contract:\nTS %s\nGo %s", name, w, g)
		}
	}
	if g, w := canonOf(t, lastro.StructuralDiff(map[string]string{"a": "int", "b": "string", "c": "bool"}, map[string]string{"a": "int", "b": "number", "d": "string"})), canonOf(t, ts.Diff); g != w {
		t.Errorf("structural diff:\nTS %s\nGo %s", w, g)
	}
	paths := []string{lastro.SnapshotPath("P-006", "after", "examples-a b.json", "UC-001", "P-006.I-03"), lastro.SnapshotPath("P-006", "before", "data.json", "", "")}
	if canonOf(t, paths) != canonOf(t, ts.Path) {
		t.Errorf("snapshot paths: TS %v, Go %v", ts.Path, paths)
	}
}

func canonOf(t *testing.T, v any) string {
	t.Helper()
	s, err := lastro.CanonicalJSON(v)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
