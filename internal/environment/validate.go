// internal/environment/validate.go
package environment

import (
	"fmt"
	"sort"
)

// Validate enforces model-shape invariants that the JSON Schema cannot:
// unique node names, every depends_on edge resolves to a declared node, and
// the graph is acyclic. (Grounding against RawFacts is ValidateGrounding.)
func (m EnvironmentModel) Validate() error {
	// Build node set + edge map. Application participates as a source node
	// (id "application") but is never a depends_on target.
	const appID = "application"
	edges := map[string][]string{appID: m.Application.DependsOn}
	nodes := map[string]bool{appID: true}

	for name := range m.Dependencies {
		if nodes[name] {
			return fmt.Errorf("environment: duplicate node name %q", name)
		}
		nodes[name] = true
	}
	for _, s := range m.Setup {
		if nodes[s.ID] {
			return fmt.Errorf("environment: duplicate node name %q", s.ID)
		}
		nodes[s.ID] = true
	}
	for name, d := range m.Dependencies {
		edges[name] = d.DependsOn
	}
	for _, s := range m.Setup {
		edges[s.ID] = s.DependsOn
	}

	// Edge integrity: every target must be a declared node (not application).
	for src, targets := range edges {
		for _, tgt := range targets {
			if tgt == appID {
				return fmt.Errorf("environment: node %q depends_on \"application\", which is the root and can never be a dependency target", src)
			}
			if !nodes[tgt] {
				return fmt.Errorf("environment: node %q depends_on unknown node %q", src, tgt)
			}
		}
	}

	return acyclic(nodes, edges)
}

// acyclic runs Kahn's algorithm; returns an *fmt.Errorf naming "cycle" when one
// remains. Deterministic ordering via sorted keys.
func acyclic(nodes map[string]bool, edges map[string][]string) error {
	indeg := map[string]int{}
	for n := range nodes {
		indeg[n] = 0
	}
	for _, targets := range edges {
		for _, t := range targets {
			indeg[t]++
		}
	}
	var queue []string
	for n := range nodes {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	sort.Strings(queue)
	visited := 0
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		visited++
		next := append([]string{}, edges[n]...)
		sort.Strings(next)
		for _, t := range next {
			indeg[t]--
			if indeg[t] == 0 {
				queue = append(queue, t)
			}
		}
	}
	if visited != len(nodes) {
		return fmt.Errorf("environment: dependency graph has a cycle")
	}
	return nil
}
