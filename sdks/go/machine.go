package lastro

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// State-machine proofs: a port of the TS SDK's machine.ts (breadth-first exploration, random walks, statecharts).
//
// A machine model has shared state variables, per-actor (local) variables, an initial state and events with finite
// inputs. Actors are copies of the same process: their events interleave in every possible order. The code under
// proof is a step function from a state and an event, raised by an actor, to the next state (nil when the event is
// not enabled there). Exploration is breadth first in one canonical order (actors ascending, events by name, inputs
// in domain order), so every engine visits the same states and finds the same shortest counterexample. A space of at
// most Limit reachable states is visited completely and proves; a larger one is walked at random with a fixed seed
// and only evidences. Proving a larger space by SMT is in the TS SDK only.

// MachineModel is a state machine: shared and per-actor variables, the initial values and the events.
type MachineModel struct {
	Name    string            `json:"name"`
	Kind    string            `json:"kind"`
	State   Fields            `json:"state"`
	Local   Fields            `json:"local,omitempty"`
	Actors  int               `json:"actors,omitempty"`
	Initial Env               `json:"initial"`
	Events  map[string]Fields `json:"events"`
	// Transitions are kept as written (the TS SDK proves large spaces from them by SMT).
	Transitions json.RawMessage `json:"transitions,omitempty"`
}

// IsMachine is true for a model with "kind": "machine".
func IsMachine(raw json.RawMessage) bool {
	var k struct {
		Kind string `json:"kind"`
	}
	return json.Unmarshal(raw, &k) == nil && k.Kind == "machine"
}

// MachineClause is an invariant (Holds), a step clause (Requires, Ensures) or a reachability condition (Condition).
type MachineClause struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // invariant | step | reach
	Description string `json:"description,omitempty"`
	Holds       *Expr  `json:"holds,omitempty"`
	Requires    *Expr  `json:"requires,omitempty"`
	Ensures     *Expr  `json:"ensures,omitempty"`
	Condition   *Expr  `json:"condition,omitempty"`
}

// MachineStep is the code under proof: nil when the event is not enabled in that state.
type MachineStep func(input Env) (Env, error)

// MachineEvent is one event raised by one actor.
type MachineEvent struct {
	Actor int    `json:"actor"`
	Event string `json:"event"`
	Input Env    `json:"input"`
}

// MachineClauseResult is one clause's verdict, with its path when there is one.
type MachineClauseResult struct {
	ClauseID string `json:"clauseId"`
	Kind     string `json:"kind"`
	Verdict  string `json:"verdict"`
	Evidence string `json:"evidence"`
	Method   string `json:"method"` // bfs | random-walk
	// Checked is the states visited (bfs) or steps taken (random walks); for a step clause, the steps it applied to.
	Checked                 int            `json:"checked"`
	Path                    []MachineEvent `json:"path,omitempty"`
	State                   Env            `json:"state,omitempty"`
	FailureRateUpperBound95 *float64       `json:"failureRateUpperBound95,omitempty"`
	Note                    string         `json:"note,omitempty"`
}

// MachineReport is the result of exploring a machine.
type MachineReport struct {
	Model       string                `json:"model"`
	Method      string                `json:"method"`
	States      int                   `json:"states"`
	Transitions int                   `json:"transitions"`
	Complete    bool                  `json:"complete"`
	Clauses     []MachineClauseResult `json:"clauses"`
	Errors      []string              `json:"errors"`
}

// MachineOptions bound the exploration; zero values take the TS SDK's defaults.
type MachineOptions struct {
	Limit, Walks, Depth int
	Seed                *uint32
}

func (m MachineModel) actors() int {
	if m.Actors < 1 {
		return 1
	}
	return m.Actors
}

func (m MachineModel) eventNames() []string {
	names := make([]string, 0, len(m.Events))
	for k := range m.Events {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool { return utf16Less(names[i], names[j]) })
	return names
}

// MachineProblems lists what is wrong with a model: infinite domains, no events, an initial value out of its domain.
func MachineProblems(m MachineModel) []string {
	errs := []string{}
	finite := func(fs Fields, at string) {
		for _, f := range fs {
			if math.IsInf(f.Domain.Size(), 1) {
				errs = append(errs, fmt.Sprintf("%s.%s: the domain must be finite", at, f.Name))
			}
		}
	}
	finite(m.State, "state")
	finite(m.Local, "local")
	for _, e := range m.eventNames() {
		finite(m.Events[e], "events."+e)
	}
	if len(m.Events) == 0 {
		errs = append(errs, "events: a machine needs at least one event")
	}
	for _, f := range append(append(Fields{}, m.State...), m.Local...) {
		if !f.Domain.Contains(m.Initial[f.Name]) {
			v := "undefined"
			if x, ok := m.Initial[f.Name]; ok {
				v = Stringify(x)
			}
			errs = append(errs, fmt.Sprintf("initial.%s: %s is not in its domain", f.Name, v))
		}
	}
	if m.Actors < 0 {
		errs = append(errs, "actors: must be a positive integer")
	}
	for _, f := range m.Local {
		if m.State.Has(f.Name) {
			errs = append(errs, fmt.Sprintf("local.%s: also a shared variable", f.Name))
		}
	}
	return errs
}

func machineInitial(m MachineModel) Env {
	s := Env{}
	for _, k := range m.State.sortedNames() {
		s[k] = Normalize(m.Initial[k])
	}
	for a := 1; a <= m.actors(); a++ {
		for _, v := range m.Local.sortedNames() {
			s[fmt.Sprintf("%s_%d", v, a)] = Normalize(m.Initial[v])
		}
	}
	return s
}

func machineEvents(m MachineModel) []MachineEvent {
	out := []MachineEvent{}
	for actor := 1; actor <= m.actors(); actor++ {
		for _, name := range m.eventNames() {
			inputs, _ := EnumerateSpace(m.Events[name])
			for _, in := range inputs {
				out = append(out, MachineEvent{Actor: actor, Event: name, Input: in})
			}
		}
	}
	return out
}

func machineStepInput(m MachineModel, s Env, e MachineEvent) Env {
	x := Env{"actor": float64(e.Actor), "event": e.Event}
	for k, v := range e.Input {
		x[k] = v
	}
	for _, f := range m.State {
		x[f.Name] = s[f.Name]
	}
	for _, f := range m.Local {
		x[f.Name] = s[fmt.Sprintf("%s_%d", f.Name, e.Actor)]
	}
	return x
}

func showEvent(e MachineEvent) string {
	in := ""
	if len(e.Input) > 0 {
		in, _ = CanonicalJSON(e.Input)
	}
	return fmt.Sprintf("%s%s by actor %d", e.Event, in, e.Actor)
}

func machineNext(m MachineModel, s Env, e MachineEvent, out Env) (Env, string) {
	n := Env{}
	for k, v := range s {
		n[k] = v
	}
	for _, f := range m.State {
		v, ok := out[f.Name]
		if !ok {
			v = s[f.Name]
		}
		v = Normalize(v)
		if !f.Domain.Contains(v) {
			return nil, fmt.Sprintf("%s=%s is outside its domain after %s", f.Name, Stringify(v), showEvent(e))
		}
		n[f.Name] = v
	}
	for _, f := range m.Local {
		own := fmt.Sprintf("%s_%d", f.Name, e.Actor)
		v, ok := out[f.Name]
		if !ok {
			v = s[own]
		}
		v = Normalize(v)
		if !f.Domain.Contains(v) {
			return nil, fmt.Sprintf("%s=%s is outside its domain after %s", f.Name, Stringify(v), showEvent(e))
		}
		n[own] = v
	}
	return n, ""
}

func machineStepOutput(m MachineModel, n Env, actor int) Env {
	o := Env{}
	for _, f := range m.State {
		o[f.Name] = n[f.Name]
	}
	for _, f := range m.Local {
		o[f.Name] = n[fmt.Sprintf("%s_%d", f.Name, actor)]
	}
	return o
}

func stateKey(s Env) string {
	k, _ := CanonicalJSON(s)
	return k
}

type machineNode struct {
	state  Env
	parent int
	via    *MachineEvent
}

func machinePath(nodes []machineNode, i int) []MachineEvent {
	path := []MachineEvent{}
	for k := i; k >= 0 && nodes[k].via != nil; k = nodes[k].parent {
		path = append([]MachineEvent{*nodes[k].via}, path...)
	}
	return path
}

// ExploreMachine explores a machine against its clauses with the code's step function.
func ExploreMachine(m MachineModel, step MachineStep, clauses []MachineClause, opts MachineOptions) MachineReport {
	errs := MachineProblems(m)
	report := func(method string, states, transitions int, complete bool, results []MachineClauseResult) MachineReport {
		return MachineReport{Model: m.Name, Method: method, States: states, Transitions: transitions, Complete: complete, Clauses: results, Errors: nonNil(errs)}
	}
	results := make([]MachineClauseResult, len(clauses))
	for i, c := range clauses {
		results[i] = MachineClauseResult{ClauseID: c.ID, Kind: c.Kind, Verdict: "not-run", Evidence: "none", Method: "bfs"}
	}
	if len(errs) > 0 {
		for i := range results {
			results[i].Note = "the model or the clause is invalid"
		}
		return report("bfs", 0, 0, false, results)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 100_000
	}
	events := machineEvents(m)
	applied := map[string]int{}
	failEval := func(r *MachineClauseResult, err error) {
		if r.Verdict == "not-run" {
			r.Verdict, r.Evidence, r.Note = "violated", "none", "evaluation error: "+err.Error()
		}
	}
	check := func(r *MachineClauseResult, e Expr, ctx EvalContext) (bool, bool) {
		ok, err := Holds(e, ctx)
		if err != nil {
			failEval(r, err)
			return false, false
		}
		return ok, true
	}
	onState := func(nodes []machineNode, i int, method string) {
		s := nodes[i].state
		ctx := EvalContext{Input: s, Output: Env{}}
		for k, c := range clauses {
			r := &results[k]
			if r.Verdict != "not-run" {
				continue
			}
			switch c.Kind {
			case "invariant":
				if ok, valid := check(r, *c.Holds, ctx); valid && !ok {
					r.Verdict, r.Evidence, r.Method, r.Path, r.State = "violated", "counterexample", method, machinePath(nodes, i), s
				}
			case "reach":
				if ok, valid := check(r, *c.Condition, ctx); valid && ok {
					r.Verdict, r.Evidence, r.Method, r.Path, r.State, r.Note = "proved", "formal", method, machinePath(nodes, i), s, "reached: the path is a witness"
				}
			}
		}
	}
	onStep := func(nodes []machineNode, from int, e MachineEvent, next Env, method string) {
		for k, c := range clauses {
			if c.Kind != "step" {
				continue
			}
			r := &results[k]
			if r.Verdict != "not-run" {
				continue
			}
			ctx := EvalContext{Input: machineStepInput(m, nodes[from].state, e), Output: machineStepOutput(m, next, e.Actor)}
			if c.Requires != nil {
				if ok, valid := check(r, *c.Requires, ctx); !valid || !ok {
					continue
				}
			}
			applied[c.ID]++
			if ok, valid := check(r, *c.Ensures, ctx); valid && !ok {
				r.Verdict, r.Evidence, r.Method, r.Path, r.State = "violated", "counterexample", method, append(machinePath(nodes, from), e), next
			}
		}
	}
	run := func(s Env, e MachineEvent) (Env, string) {
		out, err := step(machineStepInput(m, s, e))
		if err != nil {
			return nil, fmt.Sprintf("the step function threw on %s: %s", showEvent(e), err.Error())
		}
		if out == nil {
			return nil, ""
		}
		return machineNext(m, s, e, out)
	}
	addErr := func(msg string) {
		for _, x := range errs {
			if x == msg {
				return
			}
		}
		errs = append(errs, msg)
	}

	nodes := []machineNode{{state: machineInitial(m), parent: -1}}
	seen := map[string]bool{stateKey(nodes[0].state): true}
	onState(nodes, 0, "bfs")
	transitions, complete := 0, true
	for i := 0; i < len(nodes) && complete; i++ {
		for _, e := range events {
			n, problem := run(nodes[i].state, e)
			if problem != "" {
				addErr(problem)
				continue
			}
			if n == nil {
				continue
			}
			transitions++
			onStep(nodes, i, e, n, "bfs")
			k := stateKey(n)
			if seen[k] {
				continue
			}
			if len(nodes) >= limit {
				complete = false
				break
			}
			seen[k] = true
			ev := e
			nodes = append(nodes, machineNode{state: n, parent: i, via: &ev})
			onState(nodes, len(nodes)-1, "bfs")
		}
	}
	if complete {
		for k, c := range clauses {
			r := &results[k]
			if c.Kind == "step" {
				r.Checked = applied[c.ID]
			} else {
				r.Checked = len(nodes)
			}
			if r.Verdict != "not-run" {
				continue
			}
			switch {
			case c.Kind == "reach":
				r.Verdict, r.Evidence, r.Note = "violated", "formal", fmt.Sprintf("no reachable state satisfies the condition (%d states visited)", len(nodes))
			case c.Kind == "step" && applied[c.ID] == 0:
				r.Verdict, r.Note = "vacuous", "no reachable step makes the if-part true: the clause proves nothing"
			default:
				r.Verdict, r.Evidence = "proved", "formal"
			}
		}
		if len(errs) > 0 {
			for k := range results {
				if results[k].Verdict == "proved" {
					results[k].Verdict, results[k].Evidence, results[k].Note = "violated", "none", errs[0]
				}
			}
		}
		return report("bfs", len(nodes), transitions, true, results)
	}

	// Too many states: random walks with a fixed seed. A clause that holds is evidenced, never proved.
	seed := uint32(42)
	if opts.Seed != nil {
		seed = *opts.Seed
	}
	walks, depth := opts.Walks, opts.Depth
	if walks <= 0 {
		walks = 200
	}
	if depth <= 0 {
		depth = 50
	}
	next := rng(seed)
	steps := 0
	for w := 0; w < walks; w++ {
		walk := []machineNode{{state: machineInitial(m), parent: -1}}
		for d := 0; d < depth; d++ {
			here := walk[len(walk)-1].state
			type option struct {
				e MachineEvent
				n Env
			}
			enabled := []option{}
			for _, e := range events {
				if n, problem := run(here, e); problem == "" && n != nil {
					enabled = append(enabled, option{e, n})
				}
			}
			if len(enabled) == 0 {
				break
			}
			pick := enabled[int(math.Floor(next()*float64(len(enabled))))]
			steps++
			onStep(walk, len(walk)-1, pick.e, pick.n, "random-walk")
			ev := pick.e
			walk = append(walk, machineNode{state: pick.n, parent: len(walk) - 1, via: &ev})
			onState(walk, len(walk)-1, "random-walk")
		}
	}
	for k, c := range clauses {
		r := &results[k]
		if r.Verdict != "not-run" {
			continue
		}
		r.Method, r.Checked = "random-walk", steps
		switch {
		case c.Kind == "reach":
			r.Verdict, r.Evidence, r.Note = "evidenced", "none", "not reached by the walks; the space is too large to show it unreachable"
		case c.Kind == "step" && applied[c.ID] == 0:
			r.Verdict, r.Note = "vacuous", "no step of the walks makes the if-part true"
		default:
			n := steps
			if c.Kind == "step" {
				n = applied[c.ID]
			}
			bound := math.Min(1, 3/math.Max(1, float64(n)))
			r.Verdict, r.Evidence, r.FailureRateUpperBound95 = "evidenced", "empirical", &bound
			r.Note = fmt.Sprintf("more than %d reachable states: explored by %d random walks of up to %d steps (seed %d)", limit, walks, depth, seed)
		}
	}
	return report("random-walk", len(nodes), transitions, false, results)
}

// TableStep answers from a recorded step table (each step input with the next state, or nil when not enabled).
func TableStep(rows []StepRow) MachineStep {
	by := map[string]Env{}
	has := map[string]bool{}
	for _, r := range rows {
		k := stateKey(NormalizeEnv(r.Input))
		by[k], has[k] = r.Output, true
	}
	return func(in Env) (Env, error) {
		k := stateKey(NormalizeEnv(in))
		if !has[k] {
			return nil, fmt.Errorf("the table has no row for %s", k)
		}
		if by[k] == nil {
			return nil, nil
		}
		return NormalizeEnv(by[k]), nil
	}
}

// StepRow is one step the code was asked about: the step input and the next state it returned (nil: not enabled).
type StepRow struct {
	Input  Env `json:"input"`
	Output Env `json:"output"`
}

// ---------------------------------------------------------------------------
// Statechart
// ---------------------------------------------------------------------------

// StatechartState is one valuation of a region's variables.
type StatechartState struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Initial   bool   `json:"initial"`
	Forbidden bool   `json:"forbidden"`
}

// StatechartTransition is one transition of a region, labelled with its event.
type StatechartTransition struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Event  string `json:"event"`
	OnPath bool   `json:"onPath"`
}

// StatechartRegion is the shared state ("shared") or one actor's own variables ("actor <k>").
type StatechartRegion struct {
	Name        string                 `json:"name"`
	States      []StatechartState      `json:"states"`
	Transitions []StatechartTransition `json:"transitions"`
}

// Statechart is the machine as the user sees it, one region for the shared state and one per actor.
type Statechart struct {
	Model   string             `json:"model"`
	Regions []StatechartRegion `json:"regions"`
}

var actorSuffix = regexp.MustCompile(`_\d+$`)

// jsString is String(v) in JavaScript.
func jsStringOf(v any) string {
	switch x := Normalize(v).(type) {
	case nil:
		return "undefined"
	case string:
		return x
	case float64:
		return jsNumber(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return fmt.Sprint(v)
}

// MachineStatechart draws the machine: states forbidden by an invariant are marked and the path's edges flagged.
func MachineStatechart(m MachineModel, step MachineStep, clauses []MachineClause, path []MachineEvent, limit int) Statechart {
	if limit <= 0 {
		limit = 100_000
	}
	type region struct {
		name  string
		actor int // 0: shared
		vars  []string
	}
	regions := []region{}
	if len(m.State) > 0 {
		regions = append(regions, region{"shared", 0, m.State.sortedNames()})
	}
	if len(m.Local) > 0 {
		for a := 1; a <= m.actors(); a++ {
			vars := []string{}
			for _, v := range m.Local.sortedNames() {
				vars = append(vars, fmt.Sprintf("%s_%d", v, a))
			}
			regions = append(regions, region{fmt.Sprintf("actor %d", a), a, vars})
		}
	}
	out := make([]StatechartRegion, len(regions))
	for i, r := range regions {
		out[i] = StatechartRegion{Name: r.name, States: []StatechartState{}, Transitions: []StatechartTransition{}}
	}
	forbidden := func(s Env) bool {
		for _, c := range clauses {
			if c.Kind != "invariant" {
				continue
			}
			if ok, err := Holds(*c.Holds, EvalContext{Input: s, Output: Env{}}); err == nil && !ok {
				return true
			}
		}
		return false
	}
	addState := func(r int, s Env, initial bool) string {
		vals := []any{}
		labels := []string{}
		for _, v := range regions[r].vars {
			val, ok := s[v]
			if ok {
				vals = append(vals, val)
			} else {
				vals = append(vals, nil)
			}
			labels = append(labels, actorSuffix.ReplaceAllString(v, "")+"="+jsStringOf(val))
		}
		ids, _ := CanonicalJSON(vals)
		id := "s" + ids
		label := strings.Join(labels, ", ")
		if label == "" {
			label = "(no state)"
		}
		for k := range out[r].States {
			if out[r].States[k].ID == id {
				if forbidden(s) {
					out[r].States[k].Forbidden = true
				}
				return id
			}
		}
		out[r].States = append(out[r].States, StatechartState{ID: id, Label: label, Initial: initial, Forbidden: forbidden(s)})
		return id
	}
	events := machineEvents(m)
	queue := []Env{machineInitial(m)}
	seen := map[string]bool{stateKey(queue[0]): true}
	for r := range regions {
		addState(r, queue[0], true)
	}
	onPath := map[string]bool{}
	s := queue[0]
	for _, e := range path {
		o, err := step(machineStepInput(m, s, e))
		if err != nil || o == nil {
			break
		}
		n, problem := machineNext(m, s, e, o)
		if problem != "" {
			break
		}
		for k, r := range regions {
			if r.actor != 0 && r.actor != e.Actor {
				continue
			}
			onPath[fmt.Sprintf("%d|%s|%s|%s", k, addState(k, s, false), addState(k, n, false), e.Event)] = true
		}
		s = n
	}
	sameShared := func(a, b Env) bool {
		for _, v := range m.State.sortedNames() {
			if !same(a[v], b[v]) {
				return false
			}
		}
		return true
	}
	for i := 0; i < len(queue) && len(queue) <= limit; i++ {
		s := queue[i]
		for _, e := range events {
			o, err := step(machineStepInput(m, s, e))
			if err != nil || o == nil {
				continue
			}
			n, problem := machineNext(m, s, e, o)
			if problem != "" {
				continue
			}
			for k, r := range regions {
				if r.actor != 0 && r.actor != e.Actor {
					continue
				}
				from, to := addState(k, s, false), addState(k, n, false)
				if from == to && r.actor == 0 && sameShared(s, n) && len(regions) > 1 {
					continue
				}
				dup := false
				for _, t := range out[k].Transitions {
					dup = dup || (t.From == from && t.To == to && t.Event == e.Event)
				}
				if !dup {
					out[k].Transitions = append(out[k].Transitions, StatechartTransition{From: from, To: to, Event: e.Event, OnPath: onPath[fmt.Sprintf("%d|%s|%s|%s", k, from, to, e.Event)]})
				}
			}
			if k := stateKey(n); !seen[k] {
				seen[k] = true
				queue = append(queue, n)
			}
		}
	}
	for k := range out {
		st := out[k].States
		sort.SliceStable(st, func(i, j int) bool { return utf16Less(st[i].ID, st[j].ID) })
		tr := out[k].Transitions
		key := func(t StatechartTransition) string { s, _ := CanonicalJSON([]string{t.From, t.To, t.Event}); return s }
		sort.SliceStable(tr, func(i, j int) bool { return utf16Less(key(tr[i]), key(tr[j])) })
	}
	return Statechart{Model: m.Name, Regions: out}
}

// MermaidStatechart is the statechart in Mermaid (stateDiagram-v2), one concurrent region per actor.
func MermaidStatechart(c Statechart) string {
	ids := map[string]string{}
	count := 0
	id := func(region int, s string) string {
		k := fmt.Sprintf("%d|%s", region, s)
		if _, ok := ids[k]; !ok {
			ids[k] = fmt.Sprintf("r%d_%d", region, count)
			count++
		}
		return ids[k]
	}
	lines := []string{"stateDiagram-v2", fmt.Sprintf("  state \"%s\" as machine {", c.Model)}
	for k, r := range c.Regions {
		if k > 0 {
			lines = append(lines, "    --")
		}
		lines = append(lines, "    %% "+r.Name)
		for _, s := range r.States {
			lines = append(lines, fmt.Sprintf("    state \"%s\" as %s", strings.ReplaceAll(s.Label, `"`, "'"), id(k, s.ID)))
			if s.Initial {
				lines = append(lines, "    [*] --> "+id(k, s.ID))
			}
		}
		for _, t := range r.Transitions {
			mark := ""
			if t.OnPath {
				mark = " (counterexample)"
			}
			lines = append(lines, fmt.Sprintf("    %s --> %s : %s%s", id(k, t.From), id(k, t.To), t.Event, mark))
		}
	}
	lines = append(lines, "  }")
	bad := []string{}
	for k, r := range c.Regions {
		for _, s := range r.States {
			if s.Forbidden {
				bad = append(bad, id(k, s.ID))
			}
		}
	}
	if len(bad) > 0 {
		lines = append(lines, "  classDef forbidden fill:#fdd,stroke:#c00", "  class "+strings.Join(bad, ",")+" forbidden")
	}
	return strings.Join(lines, "\n") + "\n"
}
