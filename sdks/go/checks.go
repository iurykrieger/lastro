package lastro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// checkMore is what a machine, driven or property proof needs beyond a clause: the answer's extra fields and the
// state-machine model, when the proof names one.
type checkMore struct {
	ProofExtras
	machine *MachineModel
}

func machineOf(setup ProofSetup, name string) *MachineModel {
	if m, ok := setup.Machines[name]; ok {
		return &m
	}
	return nil
}

var surfaceKinds = []string{"screen", "route", "command", "api"}

// otherCheck parses a machine, driven or property proof (done = true), as the TS SDK's checkFromAnswer does; any other
// kind is left to the clause and scenario parsing (done = false). A clause or scenario over a machine is refused.
func otherCheck(pid, kind string, ensuresRaw, requiresRaw json.RawMessage, ex checkMore, problems *[]string) (*ProofCheck, bool) {
	text := func(where string, raw json.RawMessage) (json.RawMessage, bool) {
		if len(raw) == 0 {
			*problems = append(*problems, fmt.Sprintf("%s.%s: not valid JSON: ", pid, where))
			return nil, false
		}
		t, _ := jsonText(raw)
		var probe any
		if json.Unmarshal([]byte(t), &probe) != nil {
			*problems = append(*problems, fmt.Sprintf("%s.%s: not valid JSON: %s", pid, where, t))
			return nil, false
		}
		var buf bytes.Buffer
		_ = json.Compact(&buf, []byte(t))
		return buf.Bytes(), true
	}
	expr := func(where string, raw json.RawMessage) *Expr {
		b, ok := text(where, raw)
		if !ok {
			return nil
		}
		var e Expr
		if json.Unmarshal(b, &e) != nil {
			*problems = append(*problems, fmt.Sprintf("%s.%s: not valid JSON: %s", pid, where, b))
			return nil
		}
		return &e
	}
	switch kind {
	case "machine":
		if ex.machine == nil {
			*problems = append(*problems, pid+": a machine proof needs a state-machine model")
			return nil, true
		}
		clause := ex.ClauseKind
		if clause != "invariant" && clause != "step" && clause != "reach" {
			*problems = append(*problems, pid+": clauseKind must be invariant, step or reach")
			return nil, true
		}
		var requires *Expr
		if t, _ := jsonText(requiresRaw); clause == "step" && len(requiresRaw) > 0 && t != "" && t != "null" {
			requires = expr("requires", requiresRaw)
		}
		ensures := expr("ensures", ensuresRaw)
		if ensures == nil {
			return nil, true
		}
		c := ProofCheck{Kind: "machine", Clause: clause, Requires: requires, Ensures: ensures}
		errs := MachineClauseProblems(*ex.machine, ProofEntry{ID: pid, Check: c}.MachineClause())
		for _, e := range errs {
			*problems = append(*problems, pid+": "+e)
		}
		if len(errs) > 0 {
			return nil, true
		}
		return &c, true
	case "driven", "property":
		surface, ok := text("surface", ex.SurfaceJSON)
		var sf struct {
			Kind    string `json:"kind"`
			Locator string `json:"locator"`
		}
		if !ok || json.Unmarshal(surface, &sf) != nil || !contains(surfaceKinds, sf.Kind) || sf.Locator == "" {
			*problems = append(*problems, pid+".surface: needs a kind (screen, route, command or api) and a locator")
			return nil, true
		}
		if kind == "driven" {
			steps, ok := text("steps", ex.StepsJSON)
			var list []map[string]json.RawMessage
			valid := ok && json.Unmarshal(steps, &list) == nil && len(list) > 0
			for _, st := range list {
				valid = valid && isObject(st["input"]) && isObject(st["expect"])
			}
			if !valid {
				*problems = append(*problems, pid+".steps: a list of {input, expect} objects")
				return nil, true
			}
			tol := 0.0
			if ex.Tolerance != nil {
				tol = *ex.Tolerance
			}
			if !(tol >= 0 && tol <= 1) {
				*problems = append(*problems, pid+".tolerance: a share between 0 and 1")
			}
			return &ProofCheck{Kind: "driven", Surface: surface, Steps: steps, Tolerance: tol}, true
		}
		actions, ok := text("actions", ex.ActionsJSON)
		var list []json.RawMessage
		if !ok || json.Unmarshal(actions, &list) != nil || len(list) == 0 {
			*problems = append(*problems, pid+".actions: a non-empty list of actions")
			ok = false
		}
		invariant := expr("ensures", ensuresRaw)
		if !ok || invariant == nil {
			return nil, true
		}
		return &ProofCheck{Kind: "property", Surface: surface, Actions: actions, Invariant: invariant}, true
	case "clause", "scenario":
		if ex.machine != nil {
			*problems = append(*problems, fmt.Sprintf("%s: a %s proof needs an input/output model", pid, kind))
			return nil, true
		}
	}
	return nil, false
}

func isObject(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return len(raw) > 0 && json.Unmarshal(raw, &m) == nil && m != nil
}

// DrivenOutcome is the result of one driven scenario or property.
type DrivenOutcome struct {
	Verdict        string
	Passed         bool
	Counterexample *ProofCounterexample
	Note           string
}

// DriveRunner runs driven scenarios and properties (see Drivers in drive.go).
type DriveRunner interface {
	Run(ctx context.Context, pr ProofEntry) (DrivenOutcome, error)
}

// machineStateOrder is the key order of a whole machine state, as the TS engine builds it.
func machineStateOrder(m MachineModel) []string {
	names := func(fs Fields) []string {
		out := []string{}
		for _, f := range fs {
			out = append(out, f.Name)
		}
		sort.Slice(out, func(i, j int) bool { return utf16Less(out[i], out[j]) })
		return out
	}
	order := names(m.State)
	for a := 1; a <= m.actors(); a++ {
		for _, v := range names(m.Local) {
			order = append(order, fmt.Sprintf("%s_%d", v, a))
		}
	}
	return order
}

// pathText writes a counterexample path as "read@1 write{\"v\":1}@2", as the TS engine does.
func pathText(path []MachineEvent) string {
	parts := make([]string, len(path))
	for i, e := range path {
		in := ""
		if len(e.Input) > 0 {
			in, _ = CanonicalJSON(e.Input)
		}
		parts[i] = fmt.Sprintf("%s%s@%d", e.Event, in, e.Actor)
	}
	return strings.Join(parts, " ")
}

// parsePath reads a path written by pathText back into events.
func parsePath(s string) []MachineEvent {
	out := []MachineEvent{}
	for _, t := range strings.Fields(s) {
		at := strings.LastIndex(t, "@")
		head := t[:at]
		var actor int
		fmt.Sscanf(t[at+1:], "%d", &actor)
		e := MachineEvent{Actor: actor, Event: head, Input: Env{}}
		if b := strings.Index(head, "{"); b >= 0 {
			e.Event = head[:b]
			var in Env
			dec := json.NewDecoder(strings.NewReader(head[b:]))
			dec.UseNumber()
			_ = dec.Decode(&in)
			e.Input = NormalizeEnv(in)
		}
		out = append(out, e)
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
