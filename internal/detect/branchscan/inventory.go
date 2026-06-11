// Package branchscan is the deterministic branch-inventory engine: it walks
// an application source tree, extracts every logic branch (if / else-if /
// else, switch and select cases, catch/ternary in heuristic languages), and
// emits the branch-inventory.yaml that use cases reference via `covers` and
// the coverage metric scores against.
//
// Go sources are parsed exactly (go/ast, precision "ast"); other supported
// languages are scanned line-by-line (precision "heuristic"). Branch ids
// hash file|kind|condition|ordinal — line numbers are informational so ids
// survive unrelated edits.
package branchscan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
)

// Precision describes how exact a file's branch extraction is.
type Precision string

const (
	PrecisionAST       Precision = "ast"
	PrecisionHeuristic Precision = "heuristic"
)

// Branch is one decision point in the application source.
type Branch struct {
	ID        string           `json:"id"`
	File      string           `json:"file"`
	Line      int              `json:"line"`
	Kind      enums.BranchKind `json:"kind"`
	Condition string           `json:"condition,omitempty"`
	Enclosing string           `json:"enclosing,omitempty"`
}

// FileEntry records one scanned file and its extraction precision.
type FileEntry struct {
	Path      string    `json:"path"`
	Precision Precision `json:"precision"`
}

// Inventory is the engine's output, matching schemas/branch-inventory.yaml.
type Inventory struct {
	SchemaVersion string      `json:"schema_version"`
	SourceRoot    string      `json:"source_root"`
	Files         []FileEntry `json:"files"`
	Branches      []Branch    `json:"branches"`
}

// Marshal renders the inventory as YAML. Output is deterministic: same
// inventory → byte-identical bytes.
func (inv *Inventory) Marshal() ([]byte, error) {
	return yaml.Marshal(inv)
}

// Load parses and validates a branch-inventory YAML document.
func Load(raw []byte) (*Inventory, error) {
	var inv Inventory
	if err := yaml.Unmarshal(raw, &inv); err != nil {
		return nil, fmt.Errorf("branchscan: unmarshal inventory: %w", err)
	}
	if !strings.HasPrefix(inv.SchemaVersion, "1.0.") {
		return nil, fmt.Errorf("branchscan: schema_version %q is not supported (want 1.0.x)", inv.SchemaVersion)
	}
	for _, b := range inv.Branches {
		if !enums.IsValidBranchKind(string(b.Kind)) {
			return nil, fmt.Errorf("branchscan: branch %s has unknown kind %q", b.ID, b.Kind)
		}
	}
	return &inv, nil
}

// IDs returns the set of branch ids in the inventory.
func (inv *Inventory) IDs() map[string]bool {
	out := make(map[string]bool, len(inv.Branches))
	for _, b := range inv.Branches {
		out[b.ID] = true
	}
	return out
}

// assignIDs gives every branch its stable id. The ordinal disambiguates
// branches whose (file, kind, condition) collide, in scan order.
func assignIDs(branches []Branch) {
	counter := map[string]int{}
	for i := range branches {
		b := &branches[i]
		key := b.File + "|" + string(b.Kind) + "|" + b.Condition
		ord := counter[key]
		counter[key]++
		sum := sha256.Sum256([]byte(key + "|" + strconv.Itoa(ord)))
		b.ID = "br-" + hex.EncodeToString(sum[:])[:12]
	}
}
