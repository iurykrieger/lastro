package lastro

import (
	"fmt"
	"sort"
	"strings"
)

// The feature map: how a user or client reaches each feature (its surfaces) and which features it needs
// (dependsOn). A port of the TS SDK's features.ts: same checks, same order, same messages.

// SurfaceKinds are the kinds of surface a feature can have.
var SurfaceKinds = []string{"screen", "route", "command", "api"}

// Surface is one way to reach a feature: its kind, a name in the user's words and a locator a driver understands.
type Surface struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Locator string `json:"locator"`
}

// FeatureMapError is a malformed surface, a dependency on a missing or removed feature, or a dependency cycle.
type FeatureMapError struct {
	Msg string
	// Cycle lists the features of the dependency cycle, from the smallest id; empty when there is none.
	Cycle []string
}

func (e *FeatureMapError) Error() string { return e.Msg }

// Linked is what the map needs of a feature.
type Linked struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Surfaces  []Surface `json:"surfaces,omitempty"`
	DependsOn []string  `json:"dependsOn,omitempty"`
}

// LinkedOf turns use cases into map entries.
func LinkedOf(useCases []UseCase) []Linked {
	out := make([]Linked, len(useCases))
	for i, u := range useCases {
		out[i] = Linked{ID: u.ID, Slug: u.Slug, Name: u.Name, Surfaces: u.Surfaces, DependsOn: u.DependsOn}
	}
	return out
}

// SurfaceProblems lists the problems with one feature's surfaces.
func SurfaceProblems(id string, surfaces []Surface) []string {
	errs := []string{}
	for i, s := range surfaces {
		at := fmt.Sprintf("%s.surfaces[%d]", id, i)
		ok := false
		for _, k := range SurfaceKinds {
			ok = ok || s.Kind == k
		}
		if !ok {
			errs = append(errs, at+": kind must be one of "+strings.Join(SurfaceKinds, ", "))
		}
		if strings.TrimSpace(s.Name) == "" {
			errs = append(errs, at+": name is required")
		}
		if strings.TrimSpace(s.Locator) == "" {
			errs = append(errs, at+": locator is required")
		}
	}
	return errs
}

// MapEntry is one feature in the map.
type MapEntry struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	Surfaces  []Surface `json:"surfaces"`
	Reachable bool      `json:"reachable"`
	DependsOn []string  `json:"dependsOn"`
	// Dependents are the features that depend on this one directly.
	Dependents []string `json:"dependents"`
}

// FeatureMap lists every feature in id order.
type FeatureMap struct {
	Features []MapEntry `json:"features"`
}

// BuildFeatureMap checks the features and builds the map; every problem is reported at once.
func BuildFeatureMap(features []Linked, removed []string) (FeatureMap, error) {
	problems := []string{}
	for _, f := range features {
		problems = append(problems, SurfaceProblems(f.ID, f.Surfaces)...)
	}
	if len(problems) > 0 {
		return FeatureMap{}, &FeatureMapError{Msg: strings.Join(problems, "; ")}
	}
	gone := map[string]bool{}
	for _, r := range removed {
		gone[r] = true
	}
	live := []Linked{}
	for _, f := range features {
		if !gone[f.ID] {
			live = append(live, f)
		}
	}
	sort.SliceStable(live, func(i, j int) bool { return utf16Less(live[i].ID, live[j].ID) })
	ids := map[string]bool{}
	for _, f := range live {
		ids[f.ID] = true
	}
	for _, f := range live {
		deps := append([]string{}, f.DependsOn...)
		sort.Slice(deps, func(i, j int) bool { return utf16Less(deps[i], deps[j]) })
		for _, d := range deps {
			if gone[d] {
				problems = append(problems, fmt.Sprintf("%s depends on %s, which is removed", f.ID, d))
			} else if !ids[d] {
				problems = append(problems, fmt.Sprintf("%s depends on %s, which does not exist", f.ID, d))
			}
		}
	}
	deps := map[string][]string{}
	order := []string{}
	for _, f := range live {
		seen := map[string]bool{}
		ds := []string{}
		for _, d := range f.DependsOn {
			if ids[d] && !seen[d] {
				seen[d] = true
				ds = append(ds, d)
			}
		}
		sort.Slice(ds, func(i, j int) bool { return utf16Less(ds[i], ds[j]) })
		deps[f.ID] = ds
		order = append(order, f.ID)
	}
	cycle := findCycle(order, deps)
	if cycle != nil {
		problems = append(problems, "dependency cycle: "+strings.Join(append(append([]string{}, cycle...), cycle[0]), " -> "))
	}
	if len(problems) > 0 {
		return FeatureMap{}, &FeatureMapError{Msg: strings.Join(problems, "; "), Cycle: nonNil(cycle)}
	}
	out := FeatureMap{Features: []MapEntry{}}
	for _, f := range live {
		surfaces := append([]Surface{}, f.Surfaces...)
		sort.SliceStable(surfaces, func(i, j int) bool {
			return utf16Less(surfaces[i].Kind+"\x00"+surfaces[i].Locator, surfaces[j].Kind+"\x00"+surfaces[j].Locator)
		})
		dependents := []string{}
		for _, g := range live {
			for _, d := range deps[g.ID] {
				if d == f.ID {
					dependents = append(dependents, g.ID)
				}
			}
		}
		out.Features = append(out.Features, MapEntry{ID: f.ID, Slug: f.Slug, Name: f.Name, Surfaces: surfaces, Reachable: len(f.Surfaces) > 0, DependsOn: deps[f.ID], Dependents: dependents})
	}
	return out, nil
}

func findCycle(ids []string, deps map[string][]string) []string {
	state := map[string]int{}
	stack := []string{}
	var visit func(id string) []string
	visit = func(id string) []string {
		state[id] = 1
		stack = append(stack, id)
		for _, d := range deps[id] {
			if state[d] == 1 {
				at := 0
				for i, x := range stack {
					if x == d {
						at = i
					}
				}
				loop := append([]string{}, stack[at:]...)
				min := 0
				for i, x := range loop {
					if utf16Less(x, loop[min]) {
						min = i
					}
				}
				return append(append([]string{}, loop[min:]...), loop[:min]...)
			}
			if state[d] == 0 {
				if c := visit(d); c != nil {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		return nil
	}
	for _, id := range ids {
		if state[id] == 0 {
			if c := visit(id); c != nil {
				return c
			}
		}
	}
	return nil
}

// AffectedBy lists every feature that depends, directly or not, on one of changed; the changed ones are left out.
func AffectedBy(m FeatureMap, changed []string) []string {
	dependents := map[string][]string{}
	for _, f := range m.Features {
		dependents[f.ID] = f.Dependents
	}
	seen := map[string]bool{}
	queue := append([]string{}, changed...)
	sort.Slice(queue, func(i, j int) bool { return utf16Less(queue[i], queue[j]) })
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		for _, d := range dependents[head] {
			if !seen[d] {
				seen[d] = true
				queue = append(queue, d)
			}
		}
	}
	for _, c := range changed {
		delete(seen, c)
	}
	out := []string{}
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return utf16Less(out[i], out[j]) })
	return out
}

// RenderFeatureMap is the map as text, the output of `lastro map`.
func RenderFeatureMap(m FeatureMap, affected []string) string {
	isAffected := map[string]bool{}
	for _, a := range affected {
		isAffected[a] = true
	}
	var b strings.Builder
	for _, f := range m.Features {
		b.WriteString(f.ID + " " + f.Name)
		if isAffected[f.ID] {
			b.WriteString("  (affected)")
		}
		b.WriteString("\n")
		if !f.Reachable {
			b.WriteString("  not reachable by the driver: no surface\n")
		}
		for _, s := range f.Surfaces {
			b.WriteString(fmt.Sprintf("  %s %s: %s\n", s.Kind, s.Locator, s.Name))
		}
		if len(f.DependsOn) > 0 {
			b.WriteString("  depends on " + strings.Join(f.DependsOn, ", ") + "\n")
		}
		if len(f.Dependents) > 0 {
			b.WriteString("  needed by " + strings.Join(f.Dependents, ", ") + "\n")
		}
	}
	if len(m.Features) == 0 {
		return "\n" // as the TS SDK: the lines joined, then a newline
	}
	return b.String()
}

// LinksChange is what a modify does to a feature's surfaces and dependencies.
type LinksChange struct {
	AddSurfaces     []Surface `json:"addSurfaces"`
	RemoveSurfaces  []string  `json:"removeSurfaces"`
	AddDependsOn    []string  `json:"addDependsOn"`
	RemoveDependsOn []string  `json:"removeDependsOn"`
}

// MergeLinks applies a change to existing links; removing what the feature does not have is refused.
func MergeLinks(surfaces []Surface, dependsOn []string, c LinksChange) ([]Surface, []string, error) {
	out := append([]Surface{}, surfaces...)
	for _, loc := range c.RemoveSurfaces {
		k := -1
		for i, s := range out {
			if s.Locator == loc {
				k = i
				break
			}
		}
		if k < 0 {
			return nil, nil, &FeatureMapError{Msg: fmt.Sprintf("cannot remove surface %s: the feature does not have it", loc)}
		}
		out = append(out[:k], out[k+1:]...)
	}
	for _, s := range c.AddSurfaces {
		has := false
		for _, x := range out {
			has = has || x.Locator == s.Locator
		}
		if !has {
			out = append(out, s)
		}
	}
	deps := append([]string{}, dependsOn...)
	for _, d := range c.RemoveDependsOn {
		k := -1
		for i, x := range deps {
			if x == d {
				k = i
				break
			}
		}
		if k < 0 {
			return nil, nil, &FeatureMapError{Msg: fmt.Sprintf("cannot remove dependency on %s: the feature does not have it", d)}
		}
		deps = append(deps[:k], deps[k+1:]...)
	}
	for _, d := range c.AddDependsOn {
		has := false
		for _, x := range deps {
			has = has || x == d
		}
		if !has {
			deps = append(deps, d)
		}
	}
	return out, deps, nil
}

// linksOf is the surfaces and dependencies of one extracted change. An add takes the ones proposed; a modify keeps
// the use case's own and only adds or removes those the change names; a remove has none. Problems are collected.
func linksOf(change string, l *LinksChange, existing *UseCase, idOf func(string) string, problems *[]string, at string) ([]Surface, []string) {
	if l == nil || change == "remove" {
		if change == "remove" || existing == nil {
			return nil, nil
		}
		return nilIfEmpty(existing.Surfaces), nilIfEmpty(existing.DependsOn)
	}
	if change == "add" && (len(l.RemoveSurfaces) > 0 || len(l.RemoveDependsOn) > 0) {
		*problems = append(*problems, at+": an add has no surfaces or dependencies to remove")
		return nil, nil
	}
	ids := func(slugs []string) []string {
		out := []string{}
		for _, s := range slugs {
			id := idOf(s)
			if id == "" {
				*problems = append(*problems, fmt.Sprintf("%s: depends on unknown use case %q", at, s))
				id = s
			}
			out = append(out, id)
		}
		return out
	}
	var surfaces []Surface
	var deps []string
	if change != "add" && existing != nil {
		surfaces, deps = existing.Surfaces, existing.DependsOn
	}
	addDeps, removeDeps := ids(l.AddDependsOn), ids(l.RemoveDependsOn)
	s, d, err := MergeLinks(surfaces, deps, LinksChange{AddSurfaces: l.AddSurfaces, RemoveSurfaces: l.RemoveSurfaces, AddDependsOn: addDeps, RemoveDependsOn: removeDeps})
	if err != nil {
		*problems = append(*problems, at+": "+err.Error())
		return nil, nil
	}
	*problems = append(*problems, SurfaceProblems(at, s)...)
	return nilIfEmpty(s), nilIfEmpty(d)
}

func nilIfEmpty[T any](s []T) []T {
	if len(s) == 0 {
		return nil
	}
	return s
}
