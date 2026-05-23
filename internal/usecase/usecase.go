// Package usecase implements the UseCase entity: schema, loader, template
// resolver wiring, and cross-reference validation. See
// schemas/use-case.yaml for the on-disk shape and
// docs/superpowers/specs/2026-05-22-e4-use-case-design.md for the design.
package usecase

import (
	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// UseCase is the in-memory representation of a loaded use-case YAML. The
// unexported *Segs fields cache parsed template segments so resolver and
// label renderer share the parse.
type UseCase struct {
	SchemaVersion  string                  `yaml:"schema_version"  json:"schema_version"`
	ID             string                  `yaml:"id"              json:"id"`
	Title          string                  `yaml:"title"           json:"title"`
	ArchetypeScope []enums.Archetype       `yaml:"archetype_scope" json:"archetype_scope"`
	EntryPoints    []entrypoint.EntryPoint `yaml:"entry_points"    json:"entry_points"`
	Given          []string                `yaml:"given"           json:"given"`
	When           []string                `yaml:"when"            json:"when"`
	Then           []string                `yaml:"then"            json:"then"`
	SourceRefs     []SourceRef             `yaml:"source_refs"     json:"source_refs,omitempty"`
	FixtureIDs     []string                `yaml:"fixture_ids"     json:"fixture_ids,omitempty"`

	givenSegs [][]template.Segment
	whenSegs  [][]template.Segment
	thenSegs  [][]template.Segment
}

// SourceRef is a pointer to user code that motivated this use case. Pure
// provenance: the loader never resolves these against the filesystem.
type SourceRef struct {
	Path   string `yaml:"path"   json:"path"`
	Symbol string `yaml:"symbol" json:"symbol,omitempty"`
	Reason string `yaml:"reason" json:"reason,omitempty"`
}

// GivenSegments returns the parsed template segments for the i-th `given`
// line. Out-of-range panics, which is intentional for in-process callers.
func (u *UseCase) GivenSegments(i int) []template.Segment { return u.givenSegs[i] }

// WhenSegments returns the parsed template segments for the i-th `when`
// line.
func (u *UseCase) WhenSegments(i int) []template.Segment { return u.whenSegs[i] }

// ThenSegments returns the parsed template segments for the i-th `then`
// line.
func (u *UseCase) ThenSegments(i int) []template.Segment { return u.thenSegs[i] }
