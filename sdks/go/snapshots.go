package lastro

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Before and after snapshots, and what a reviewer must still do before a proposal is proved. A port of the TS
// SDK's snapshots.ts and of the io part of evidence.ts: the same files, byte for byte, so their SHA-256 match.
//
// Every intent gets a before snapshot of each feature it touches, taken when the proposal is extracted, and the
// same snapshot on the new code at the proof run (the after). This port takes the contract and examples snapshots
// of io models; screens and data need the TS SDK's drivers.

// SnapshotRecord is one snapshot file, as the proposal records it.
type SnapshotRecord struct {
	// Intent changed the feature, or is "repository" for a whole-repository snapshot.
	Intent  string `json:"intent"`
	Feature string `json:"feature,omitempty"`
	Kind    string `json:"kind"` // screen | contract | data | examples
	Side    string `json:"side"` // before | after
	// Path is relative to the workspace folder: docs/<proposal>/<feature>/<intent>/<side>/<file>.
	Path   string `json:"path"`
	Sha256 string `json:"sha256"`
}

// FieldChange is a field or column added, removed or changed in type; a removal or a type change is breaking.
type FieldChange struct {
	Path     string `json:"path"`
	Change   string `json:"change"` // added | removed | type
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
	Breaking bool   `json:"breaking"`
}

// ExampleRow is one example input with its before and after outputs (an output object or {"error": ...}).
// Values keep the key order they were written in, as the TS SDK does.
type ExampleRow struct {
	Input  json.RawMessage `json:"input"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

// Comparison pairs an after snapshot with its before.
type Comparison struct {
	Intent      string        `json:"intent"`
	Feature     string        `json:"feature,omitempty"`
	Kind        string        `json:"kind"`
	Name        string        `json:"name"`
	PixelShare  *float64      `json:"pixelShare,omitempty"`
	Diff        []FieldChange `json:"diff,omitempty"`
	Breaking    *bool         `json:"breaking,omitempty"`
	ChangedRows []ExampleRow  `json:"changedRows,omitempty"`
}

// UnexpectedChange is an example whose output changed although no approved clause applies to its input.
type UnexpectedChange struct {
	ID         string          `json:"id"` // "U1"
	Intent     string          `json:"intent"`
	Feature    string          `json:"feature,omitempty"`
	Input      json.RawMessage `json:"input"`
	Before     json.RawMessage `json:"before,omitempty"`
	After      json.RawMessage `json:"after,omitempty"`
	AcceptedBy []string        `json:"acceptedBy"`
}

// SignOff is a reviewer's look at an intent beyond proof for human judgment, after the proof run.
type SignOff struct {
	Intent  string `json:"intent"`
	By      string `json:"by"`
	Verdict string `json:"verdict"` // signed-off | changes-requested
	Note    string `json:"note,omitempty"`
	// References are screenshots the sign-off makes the reference of their scenario step.
	References []string `json:"references,omitempty"`
}

// SnapshotKinds are the kinds of snapshot a feature gets, from its surfaces, its model and its data.
func SnapshotKinds(surfaces []Surface, hasModel, hasData bool) []string {
	has := map[string]bool{}
	for _, s := range surfaces {
		has[s.Kind] = true
	}
	out := []string{}
	if has["screen"] || has["route"] {
		out = append(out, "screen")
	}
	if has["api"] || has["command"] || hasModel {
		out = append(out, "contract")
	}
	if hasData {
		out = append(out, "data")
	}
	if hasModel {
		out = append(out, "examples")
	}
	return out
}

var unsafePathChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SnapshotPath is where a snapshot file lives, relative to the workspace folder; feature and intent are empty for
// a repository snapshot.
func SnapshotPath(proposal, side, file, feature, intent string) string {
	safe := func(s string) string { return unsafePathChars.ReplaceAllString(s, "_") }
	if feature == "" && intent == "" {
		return fmt.Sprintf("docs/%s/repository/%s/%s", proposal, side, safe(file))
	}
	return fmt.Sprintf("docs/%s/%s/%s/%s/%s", proposal, safe(feature), safe(strings.Replace(intent, proposal+".", "", 1)), side, safe(file))
}

// StructuralDiff lists every field added, removed or changed in type, in path order.
func StructuralDiff(before, after map[string]string) []FieldChange {
	paths := []string{}
	for p := range before {
		paths = append(paths, p)
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return utf16Less(paths[i], paths[j]) })
	out := []FieldChange{}
	for _, p := range paths {
		b, hasB := before[p]
		a, hasA := after[p]
		switch {
		case hasB && hasA && a == b:
		case !hasB:
			out = append(out, FieldChange{Path: p, Change: "added", After: a})
		case !hasA:
			out = append(out, FieldChange{Path: p, Change: "removed", Before: b, Breaking: true})
		default:
			out = append(out, FieldChange{Path: p, Change: "type", Before: b, After: a, Breaking: true})
		}
	}
	return out
}

// ModelContract is a model's inputs and outputs with their domains, as a contract (ordered by field path).
func ModelContract(m Model) obj {
	show := func(d Domain) string {
		switch d.Type {
		case "enum":
			vals := make([]string, len(d.Values))
			for i, v := range d.Values {
				vals[i] = jsDisplay(v)
			}
			return "enum" + Stringify(vals)
		case "int":
			bound := func(x *float64) string {
				if x == nil {
					return ""
				}
				return jsNumber(*x)
			}
			return fmt.Sprintf("int[%s..%s]", bound(d.Min), bound(d.Max))
		}
		return d.Type
	}
	out := obj{}
	for _, k := range m.Inputs.sortedNames() {
		d, _ := m.Inputs.Get(k)
		out = append(out, kv{"in." + k, show(d)})
	}
	for _, k := range m.Outputs.sortedNames() {
		d, _ := m.Outputs.Get(k)
		out = append(out, kv{"out." + k, show(d)})
	}
	return out
}

// jsDisplay is String(v) in JavaScript, for the values of an enum domain.
func jsDisplay(v any) string {
	switch x := Normalize(v).(type) {
	case string:
		return x
	case float64:
		return jsNumber(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	}
	return fmt.Sprint(v)
}

// ExamplesLimit: the whole space is used when it has at most this many points; otherwise a fixed-seed sample.
const ExamplesLimit = 1000

// ExampleInputs are the scenarios' inputs (as written), then the model's whole space (up to 1,000 points) or a
// fixed-seed sample of 1,000, without repeats.
func ExampleInputs(m Model, scenarioInputs []json.RawMessage, seed uint32) ([]json.RawMessage, bool, error) {
	seen := map[string]bool{}
	out := []json.RawMessage{}
	add := func(raw json.RawMessage) error {
		k, err := canonicalRaw(raw)
		if err != nil {
			return err
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, raw)
		}
		return nil
	}
	for _, s := range scenarioInputs {
		if err := add(s); err != nil {
			return nil, false, err
		}
	}
	if SpaceSize(m.Inputs) <= ExamplesLimit {
		all, err := EnumerateSpace(m.Inputs)
		if err != nil {
			return nil, false, err
		}
		for _, e := range all {
			if err := add(json.RawMessage(Stringify(sortedEnv(e)))); err != nil {
				return nil, false, err
			}
		}
		return out, true, nil
	}
	next := rng(seed)
	for k := 0; k < ExamplesLimit; k++ {
		if err := add(json.RawMessage(Stringify(sortedEnv(sampleSpace(m.Inputs, next))))); err != nil {
			return nil, false, err
		}
	}
	return out, false, nil
}

// sortedEnv writes an Env with its keys sorted, the order enumerateSpace and sampleSpace give in the TS SDK.
func sortedEnv(e Env) obj {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
	o := obj{}
	for _, k := range keys {
		o = append(o, kv{k, e[k]})
	}
	return o
}

func canonicalRaw(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "null", nil
	}
	return CanonicalJSON(raw)
}

func rawEnv(raw json.RawMessage) Env {
	var e Env
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if dec.Decode(&e) != nil {
		return nil
	}
	return NormalizeEnv(e)
}

func isErrorOutput(raw json.RawMessage) bool {
	var o map[string]json.RawMessage
	if json.Unmarshal(raw, &o) != nil {
		return false
	}
	_, ok := o["error"]
	return ok
}

// ChangedRows are the rows whose output changed between the before and the after code.
func ChangedRows(rows []ExampleRow) []ExampleRow {
	out := []ExampleRow{}
	for _, r := range rows {
		b, _ := canonicalRaw(r.Before)
		a, _ := canonicalRaw(r.After)
		if a != b {
			out = append(out, r)
		}
	}
	return out
}

// UnexpectedChanges are the changed rows no clause explains: none of the clauses has an if-part that holds for
// the input (a clause with no if-part applies to every input). Ids start at U<start>.
func UnexpectedChanges(rows []ExampleRow, requires []*Expr, intent, feature string, start int) []UnexpectedChange {
	out := []UnexpectedChange{}
	for _, r := range ChangedRows(rows) {
		output := Env{}
		if len(r.After) > 0 && !isErrorOutput(r.After) {
			output = rawEnv(r.After)
		}
		input := rawEnv(r.Input)
		explained := false
		for _, req := range requires {
			if req == nil {
				explained = true
				break
			}
			if ok, err := Holds(*req, EvalContext{Input: input, Output: output}); err == nil && ok {
				explained = true
				break
			}
		}
		if !explained {
			out = append(out, UnexpectedChange{ID: fmt.Sprintf("U%d", start+len(out)), Intent: intent, Feature: feature, Input: r.Input, Before: r.Before, After: r.After, AcceptedBy: []string{}})
		}
	}
	return out
}

// ReviewDue lists the sign-offs due, the unexpected changes not accepted yet and the intents a reviewer requested
// changes on. Each needs the policy's number of distinct reviewers.
func ReviewDue(judgment []string, signoffs []SignOff, unexpected []UnexpectedChange, required int) (due, open, changesRequested []string) {
	need := required
	if need < 1 {
		need = 1
	}
	due, open, changesRequested = []string{}, []string{}, []string{}
	for _, i := range judgment {
		requested := false
		by := map[string]bool{}
		for _, s := range signoffs {
			if s.Intent != i {
				continue
			}
			if s.Verdict == "changes-requested" {
				requested = true
			} else if s.Verdict == "signed-off" {
				by[s.By] = true
			}
		}
		if requested {
			changesRequested = append(changesRequested, i)
		} else if len(by) < need {
			due = append(due, i)
		}
	}
	for _, u := range unexpected {
		by := map[string]bool{}
		for _, a := range u.AcceptedBy {
			by[a] = true
		}
		if len(by) < need {
			open = append(open, u.ID)
		}
	}
	return
}

// JudgmentIntents are the forward intents beyond proof for human judgment, which wait for a sign-off.
func JudgmentIntents(p Proposal) []string {
	out := []string{}
	for _, i := range ForwardIntents(p) {
		if i.Provability == "beyond-proof" && i.BeyondProofReason == "human-judgment" {
			out = append(out, i.ID)
		}
	}
	return out
}

// Due is what still blocks "proved" after a run: sign-offs, unexpected changes, intents with changes requested.
type Due struct {
	Signoffs, Unexpected, ChangesRequested []string
}

// DueOf is empty before the first run.
func DueOf(p Proposal) Due {
	if p.LastRun == nil {
		return Due{[]string{}, []string{}, []string{}}
	}
	s, u, c := ReviewDue(JudgmentIntents(p), p.Signoffs, p.Unexpected, p.Policy.Approvals)
	return Due{s, u, c}
}

// SnapshotContext is what taking snapshots needs: the workspace folder, models, subjects and use cases.
type SnapshotContext struct {
	Dir      string
	Models   map[string]Model
	Subjects map[string]Subject
	UseCases []UseCase
}

type takenSnapshot struct {
	record  SnapshotRecord
	content string
}

// TakeSnapshots takes one side's snapshots for every forward intent (at extraction: every intent), writes them
// under Dir and records them; the after side is compared with the before.
func TakeSnapshots(p Proposal, side string, ctx SnapshotContext) (Proposal, error) {
	intents := p.Intents
	if side == "after" {
		intents = ForwardIntents(p)
	}
	taken := []takenSnapshot{}
	write := func(path, content string, rec SnapshotRecord) {
		rec.Path, rec.Sha256 = path, sha256Hex(content)
		taken = append(taken, takenSnapshot{rec, content})
	}
	for _, it := range intents {
		var change *UseCaseChange
		for i := range p.UseCases {
			if p.UseCases[i].UseCase == it.UseCase {
				change = &p.UseCases[i]
				break
			}
		}
		var u *UseCase
		for i := range ctx.UseCases {
			if ctx.UseCases[i].ID == it.UseCase {
				u = &ctx.UseCases[i]
				break
			}
		}
		if side == "before" && u == nil {
			continue // a new feature has no before
		}
		if side == "after" && change != nil && change.Change == "remove" {
			continue // a removed one has no after
		}
		proofs := []ProofEntry{}
		if u != nil {
			proofs = append(proofs, u.Proofs...)
		}
		for _, x := range p.Proofs {
			if x.UseCase == it.UseCase {
				proofs = append(proofs, x)
			}
		}
		var surfaces []Surface
		switch {
		case side == "after" && change != nil && change.Surfaces != nil:
			surfaces = change.Surfaces
		case u != nil && u.Surfaces != nil:
			surfaces = u.Surfaces
		case change != nil:
			surfaces = change.Surfaces
		}
		type pair struct{ model, subject string }
		io := []pair{}
		seen := map[pair]bool{}
		for _, x := range proofs {
			if x.Check.Kind != "clause" && x.Check.Kind != "scenario" {
				continue
			}
			k := pair{x.Model, x.Subject}
			if seen[k] {
				continue
			}
			seen[k] = true
			if _, ok := ctx.Models[x.Model]; !ok {
				continue
			}
			if _, ok := ctx.Subjects[x.Subject]; !ok {
				continue
			}
			io = append(io, k)
		}
		kinds := SnapshotKinds(surfaces, len(io) > 0, false)
		has := func(k string) bool {
			for _, x := range kinds {
				if x == k {
					return true
				}
			}
			return false
		}
		rec := func(kind string) SnapshotRecord {
			return SnapshotRecord{Intent: it.ID, Feature: it.UseCase, Kind: kind, Side: side}
		}
		if has("contract") {
			for _, k := range io {
				body := obj{{"model", k.model}, {"fields", ModelContract(ctx.Models[k.model])}}
				write(SnapshotPath(p.ID, side, "model-"+k.model+".json", it.UseCase, it.ID), StringifyIndent(body)+"\n", rec("contract"))
			}
		}
		if has("examples") {
			for _, k := range io {
				m := ctx.Models[k.model]
				scen := []json.RawMessage{}
				for _, x := range proofs {
					if x.Model == k.model && x.Check.Kind == "scenario" {
						in := x.Check.Input
						if len(in) == 0 {
							in = json.RawMessage("{}")
						}
						scen = append(scen, in)
					}
				}
				inputs, whole, err := ExampleInputs(m, scen, 42)
				if err != nil {
					return p, err
				}
				order := m.Outputs.Names()
				rows := []any{}
				for _, in := range inputs {
					var output any
					if out, err := ctx.Subjects[k.subject](rawEnv(in)); err != nil {
						output = obj{{"error", err.Error()}}
					} else {
						output = envJSON(out, order)
					}
					rows = append(rows, obj{{"input", in}, {side, output}})
				}
				body := obj{{"model", k.model}, {"subject", k.subject}, {"wholeSpace", whole}, {"rows", rows}}
				write(SnapshotPath(p.ID, side, "examples-"+k.model+".json", it.UseCase, it.ID), StringifyIndent(body)+"\n", rec("examples"))
			}
		}
	}
	for _, t := range taken {
		path := filepath.Join(ctx.Dir, filepath.FromSlash(t.record.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return p, err
		}
		if err := os.WriteFile(path, []byte(t.content), 0o644); err != nil {
			return p, err
		}
	}
	next := p
	keep := []SnapshotRecord{}
	for _, x := range p.Snapshots {
		if x.Side != side {
			keep = append(keep, x)
		}
	}
	for _, t := range taken {
		keep = append(keep, t.record)
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].Path < keep[j].Path })
	next.Snapshots = keep
	if side == "after" {
		comparisons, unexpected, err := CompareSnapshots(next, ctx.Dir)
		if err != nil {
			return p, err
		}
		next.Comparisons, next.Unexpected = comparisons, unexpected
	}
	return next, nil
}

func baseName(path string) string { return path[strings.LastIndex(path, "/")+1:] }

// CompareSnapshots compares every after snapshot with its before and collects the unexpected example changes,
// keeping the acceptances of the ones already recorded. Screens are compared by the TS SDK only.
func CompareSnapshots(p Proposal, dir string) ([]Comparison, []UnexpectedChange, error) {
	comparisons := []Comparison{}
	unexpected := []UnexpectedChange{}
	read := func(r SnapshotRecord) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, filepath.FromSlash(r.Path)))
	}
	for _, after := range p.Snapshots {
		if after.Side != "after" {
			continue
		}
		var before *SnapshotRecord
		for i, x := range p.Snapshots {
			if x.Side == "before" && x.Intent == after.Intent && x.Feature == after.Feature && x.Kind == after.Kind && baseName(x.Path) == baseName(after.Path) {
				before = &p.Snapshots[i]
				break
			}
		}
		if before == nil {
			continue
		}
		base := Comparison{Intent: after.Intent, Feature: after.Feature, Kind: after.Kind, Name: baseName(after.Path)}
		b, err := read(*before)
		if err != nil {
			return nil, nil, err
		}
		a, err := read(after)
		if err != nil {
			return nil, nil, err
		}
		switch after.Kind {
		case "screen":
			continue
		case "contract", "data":
			var fb, fa struct {
				Fields map[string]string `json:"fields"`
			}
			if err := json.Unmarshal(b, &fb); err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(a, &fa); err != nil {
				return nil, nil, err
			}
			diff := StructuralDiff(fb.Fields, fa.Fields)
			breaking := false
			for _, d := range diff {
				breaking = breaking || d.Breaking
			}
			base.Diff, base.Breaking = diff, &breaking
			if len(diff) == 0 {
				base.Diff = []FieldChange{}
			}
			comparisons = append(comparisons, base)
		default:
			var fb struct {
				Model string       `json:"model"`
				Rows  []ExampleRow `json:"rows"`
			}
			var fa struct {
				Rows []ExampleRow `json:"rows"`
			}
			if err := json.Unmarshal(b, &fb); err != nil {
				return nil, nil, err
			}
			if err := json.Unmarshal(a, &fa); err != nil {
				return nil, nil, err
			}
			byInput := map[string]json.RawMessage{}
			for _, r := range fb.Rows {
				k, _ := canonicalRaw(r.Input)
				byInput[k] = r.Before
			}
			rows := []ExampleRow{}
			for _, r := range fa.Rows {
				k, _ := canonicalRaw(r.Input)
				if old, ok := byInput[k]; ok {
					rows = append(rows, ExampleRow{Input: r.Input, Before: old, After: r.After})
				}
			}
			changed := ChangedRows(rows)
			base.ChangedRows = changed
			comparisons = append(comparisons, base)
			requires := []*Expr{}
			for _, x := range p.Proofs {
				if x.Model != fb.Model {
					continue
				}
				switch x.Check.Kind {
				case "clause":
					requires = append(requires, x.Check.Requires)
				case "scenario":
					e := equalsInput(x.Check.InputEnv())
					requires = append(requires, &e)
				}
			}
			unexpected = append(unexpected, UnexpectedChanges(changed, requires, after.Intent, after.Feature, len(unexpected)+1)...)
		}
	}
	for i, u := range unexpected {
		ui, _ := canonicalRaw(u.Input)
		ua, _ := canonicalRaw(u.After)
		for _, old := range p.Unexpected {
			oi, _ := canonicalRaw(old.Input)
			oa, _ := canonicalRaw(old.After)
			if old.Intent == u.Intent && oi == ui && oa == ua {
				unexpected[i].AcceptedBy = append([]string{}, old.AcceptedBy...)
				break
			}
		}
	}
	return comparisons, unexpected, nil
}

func equalsInput(input Env) Expr {
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
	parts := []Expr{}
	for _, k := range keys {
		parts = append(parts, op("eq", V("in."+k), C(input[k])))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 0 {
		parts = []Expr{C(true)}
	}
	return And(parts...)
}

// featureMapSection is the receipt's map of the features the proposal changes and the ones depending on them.
func featureMapSection(p Proposal, useCases []UseCase) []string {
	features := []Linked{}
	linked := false
	for _, u := range useCases {
		l := Linked{ID: u.ID, Slug: u.Slug, Name: u.Name, Surfaces: u.Surfaces, DependsOn: u.DependsOn}
		for _, c := range p.UseCases {
			if c.UseCase == u.ID && c.Change == "modify" {
				l.Surfaces, l.DependsOn = c.Surfaces, c.DependsOn
			}
		}
		features = append(features, l)
	}
	removed, changed := []string{}, []string{}
	for _, c := range p.UseCases {
		changed = append(changed, c.UseCase)
		switch c.Change {
		case "add":
			features = append(features, Linked{ID: c.UseCase, Slug: c.Slug, Name: c.Name, Surfaces: c.Surfaces, DependsOn: c.DependsOn})
		case "remove":
			removed = append(removed, c.UseCase)
		}
	}
	for _, f := range features {
		linked = linked || len(f.Surfaces) > 0 || len(f.DependsOn) > 0
	}
	if !linked {
		return nil
	}
	L := []string{"## Feature map", ""}
	m, err := BuildFeatureMap(features, removed)
	if err != nil {
		L = append(L, "The map is invalid: "+err.Error())
	} else {
		affected := AffectedBy(m, changed)
		if len(affected) > 0 {
			L = append(L, "Affected: "+strings.Join(affected, ", ")+" (their proofs and snapshots run again with this proposal's).", "")
		} else {
			L = append(L, "No other feature depends on the ones this proposal changes.", "")
		}
		for _, f := range m.Features {
			if !contains(changed, f.ID) && !contains(affected, f.ID) {
				continue
			}
			surfaces := []string{}
			for _, s := range f.Surfaces {
				surfaces = append(surfaces, s.Kind+" `"+s.Locator+"`")
			}
			text := strings.Join(surfaces, ", ")
			if text == "" {
				text = "no surface"
			}
			deps := ""
			if len(f.DependsOn) > 0 {
				deps = "; depends on " + strings.Join(f.DependsOn, ", ")
			}
			L = append(L, fmt.Sprintf("- %s %s: %s%s", f.ID, f.Name, text, deps))
		}
	}
	return append(L, "")
}

func rawOrNull(r json.RawMessage) string {
	if len(r) == 0 {
		return "null"
	}
	return Stringify(r)
}

// evidenceSections are the receipt's state machines, snapshots, unexpected changes and sign-offs.
func evidenceSections(p Proposal) []string {
	L := []string{}
	names := make([]string, 0, len(p.Statecharts))
	for k := range p.Statecharts {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return utf16Less(names[i], names[j]) })
	for _, n := range names {
		L = append(L, "## State machine: "+n, "", "```mermaid", strings.TrimRight(p.Statecharts[n], " \t\r\n"), "```", "")
	}
	if len(p.Snapshots) > 0 {
		L = append(L, "## Snapshots", "")
		for _, s := range p.Snapshots {
			L = append(L, fmt.Sprintf("- %s %s `%s` sha256:%s", s.Side, s.Kind, s.Path, s.Sha256))
		}
		for _, c := range p.Comparisons {
			var what string
			switch c.Kind {
			case "screen":
				share := 0.0
				if c.PixelShare != nil {
					share = *c.PixelShare
				}
				what = strconv.FormatFloat(share*100, 'f', 2, 64) + "% of pixels changed"
			case "examples":
				what = fmt.Sprintf("%d row(s) changed", len(c.ChangedRows))
			default:
				what = fmt.Sprintf("%d field change(s)", len(c.Diff))
				if c.Breaking != nil && *c.Breaking {
					what += ", breaking"
				}
			}
			feature := ""
			if c.Feature != "" {
				feature = " " + c.Feature
			}
			L = append(L, fmt.Sprintf("- %s%s %s %s: %s", c.Intent, feature, c.Kind, c.Name, what))
			for _, d := range c.Diff {
				line := fmt.Sprintf("  - %s `%s`", d.Change, d.Path)
				if d.Before != "" {
					line += " " + d.Before
				}
				if d.After != "" {
					line += " → " + d.After
				}
				if d.Breaking {
					line += " (breaking)"
				}
				L = append(L, line)
			}
		}
		L = append(L, "")
	}
	if len(p.Unexpected) > 0 {
		L = append(L, "## Unexpected changes", "")
		for _, u := range p.Unexpected {
			accepted := "not accepted"
			if len(u.AcceptedBy) > 0 {
				accepted = "accepted by " + strings.Join(u.AcceptedBy, ", ")
			}
			L = append(L, fmt.Sprintf("- **%s** (%s) `%s`: `%s` → `%s` · %s", u.ID, u.Intent, rawOrNull(u.Input), rawOrNull(u.Before), rawOrNull(u.After), accepted))
		}
		L = append(L, "")
	}
	judgment := JudgmentIntents(p)
	if len(p.Signoffs) > 0 || len(judgment) > 0 {
		L = append(L, "## Sign-offs", "")
		for _, i := range judgment {
			by := []string{}
			for _, s := range p.Signoffs {
				if s.Intent == i {
					x := s.Verdict + " by " + s.By
					if s.Note != "" {
						x += " (" + s.Note + ")"
					}
					by = append(by, x)
				}
			}
			text := "waiting"
			if len(by) > 0 {
				text = strings.Join(by, "; ")
			}
			L = append(L, fmt.Sprintf("- %s: %s", i, text))
		}
		L = append(L, "")
	}
	return L
}

func (r *ProposalReview) judgment(id string) *ProposalIntent {
	it := r.intent(id)
	if it == nil {
		return nil
	}
	if r.p.LastRun == nil {
		r.fail("%s has not been proved yet: sign-offs come after the proof run", r.p.ID)
		return nil
	}
	if !contains(JudgmentIntents(r.p), it.ID) {
		r.fail("%s is not beyond proof for human judgment; it needs no sign-off", it.ID)
		return nil
	}
	return it
}

func (r *ProposalReview) setSignOff(s SignOff) {
	list := []SignOff{}
	for _, x := range r.p.Signoffs {
		if !(x.Intent == s.Intent && x.By == s.By) {
			list = append(list, x)
		}
	}
	r.p.Signoffs = append(list, s)
}

// SignOff signs off an intent beyond proof for human judgment after the proof run, having looked at its before and
// after snapshots. Reference screenshots come from driven scenarios, which only the TS SDK runs.
func (r *ProposalReview) SignOff(intentID string, references ...string) *ProposalReview {
	if r.err != nil {
		return r
	}
	it := r.judgment(intentID)
	if it == nil {
		return r
	}
	if len(references) > 0 {
		return r.fail("%s is not an after screenshot of a driven scenario step of %s", references[0], r.p.ID)
	}
	r.setSignOff(SignOff{Intent: it.ID, By: r.reviewer, Verdict: "signed-off"})
	return r
}

// RequestChanges asks for changes on an intent beyond proof for human judgment, after the proof run.
func (r *ProposalReview) RequestChanges(intentID, note string) *ProposalReview {
	if r.err != nil {
		return r
	}
	it := r.judgment(intentID)
	if it == nil {
		return r
	}
	r.setSignOff(SignOff{Intent: it.ID, By: r.reviewer, Verdict: "changes-requested", Note: note})
	return r
}

// Accept accepts an unexpected change (U1, U2...): its new output is intended.
func (r *ProposalReview) Accept(changeID string) *ProposalReview {
	if r.err != nil {
		return r
	}
	for k := range r.p.Unexpected {
		u := &r.p.Unexpected[k]
		if u.ID == changeID {
			if !contains(u.AcceptedBy, r.reviewer) {
				u.AcceptedBy = append(u.AcceptedBy, r.reviewer)
			}
			return r
		}
	}
	return r.fail("Unknown unexpected change %s", changeID)
}

// Reference makes an after screenshot the reference of its driven scenario step. Only the TS SDK runs driven
// scenarios, so in this port no after snapshot is a screenshot.
func (r *ProposalReview) Reference(ref string) *ProposalReview {
	if r.err != nil {
		return r
	}
	if r.p.LastRun == nil {
		return r.fail("%s has not been proved yet: screenshots are signed off after the proof run", r.p.ID)
	}
	return r.fail("%s is not an after screenshot of a driven scenario step of %s", ref, r.p.ID)
}
