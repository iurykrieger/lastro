// Package template implements the ${{ }} interpolation grammar for
// UseCase given/when/then text. The grammar is defined in
// docs/harness-framework/plan.md §4.1.2 and recapped here:
//
//	template      := "${{" ws? ref ws? "}}"
//	ref           := fixtureRef | entryPointRef | inputRef | stepOutputRef
//	fixtureRef    := "fixtures" "." ID ( "." JSONKEY )*
//	entryPointRef := "entry_points" "." ID ( "." "spec" "." JSONKEY )?
//	inputRef      := "inputs" "." ID
//	stepOutputRef := "steps" "." ID "." "outputs" "." ID
//
// Tokens:
//
//	ID      := [a-z][a-z0-9-]{0,127}      (kebab-case per schema-freeze)
//	JSONKEY := [a-zA-Z_][a-zA-Z0-9_-]*    (allows hyphens for JSON keys)
package template

// Position locates a token within its source string. Line and Col are
// 1-based; Offset is the 0-based byte index into the input.
type Position struct {
	Line   int
	Col    int
	Offset int
}

// Segment is the AST node interface. Concrete types: Literal, FixtureRef,
// EntryPointRef.
type Segment interface{ isSegment() }

// Literal is plain text between or outside {{ }} blocks.
type Literal struct {
	Text string
}

func (Literal) isSegment() {}

// FixtureRef is `${{fixtures.<id>(.<jsonkey>)*}}`. An empty JSONPath means
// the whole payload.
type FixtureRef struct {
	ID       string
	JSONPath []string
	Pos      Position
}

func (FixtureRef) isSegment() {}

// EntryPointRef is `${{entry_points.<id>}}` or
// `${{entry_points.<id>.spec.<key>}}`. An empty SpecKey means the whole
// entry point (rendered as "<archetype>:<id>").
type EntryPointRef struct {
	ID      string
	SpecKey string
	Pos     Position
}

func (EntryPointRef) isSegment() {}

// InputRef is `${{ inputs.<name> }}` — a composed primitive's declared input.
type InputRef struct {
	Name string
	Pos  Position
}

func (InputRef) isSegment() {}

// StepOutputRef is `${{ steps.<id>.outputs.<name> }}` — an output produced by
// a prior step (or by a uses-step that composed a primitive).
type StepOutputRef struct {
	StepID string
	Name   string
	Pos    Position
}

func (StepOutputRef) isSegment() {}
