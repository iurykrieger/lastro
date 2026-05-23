package usecase

import (
	"errors"
	"strings"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// Validate runs every cross-reference invariant on uc. It accumulates
// failures via errors.Join so authors see every problem on one pass.
// Returns nil if the use case is valid.
//
// Order of checks matches §10 of the design doc; later checks may assume
// earlier ones held.
func Validate(uc *UseCase, store fixture.FixtureStore) error {
	var errs []error

	errs = appendIfErr(errs, checkRequiredFields(uc))
	errs = appendIfErr(errs, checkCharset(uc))
	errs = appendIfErr(errs, checkUniqueness(uc))
	errs = appendIfErr(errs, checkArchetypeScope(uc))

	segs, parseErrs := parseAllSegments(uc)
	errs = append(errs, parseErrs...)
	if len(parseErrs) == 0 {
		errs = appendIfErr(errs, checkRefsAndStore(uc, segs, store))
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// appendIfErr flattens errors.Join results so the top-level error stays a
// flat join rather than a tree.
func appendIfErr(dst []error, err error) []error {
	if err == nil {
		return dst
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		return append(dst, u.Unwrap()...)
	}
	return append(dst, err)
}

func checkRequiredFields(uc *UseCase) error {
	var errs []error
	if uc.Title == "" {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "title is required"})
	}
	if len(uc.ArchetypeScope) == 0 {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "archetype_scope is required"})
	}
	if len(uc.EntryPoints) == 0 {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "entry_points is required"})
	}
	if len(uc.Given) == 0 {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "given is required"})
	}
	if len(uc.When) == 0 {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "when is required"})
	}
	if len(uc.Then) == 0 {
		errs = append(errs, &ValidationError{Code: "USECASE_REQUIRED_FIELD_MISSING", Message: "then is required"})
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func checkCharset(uc *UseCase) error {
	var errs []error
	if err := ValidateID(uc.ID); err != nil {
		errs = append(errs, err)
	}
	for _, ep := range uc.EntryPoints {
		if err := ValidateID(ep.ID); err != nil {
			errs = append(errs, err)
		}
	}
	for _, id := range uc.FixtureIDs {
		if err := ValidateID(id); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func checkUniqueness(uc *UseCase) error {
	var errs []error
	seen := map[string]bool{}
	for _, ep := range uc.EntryPoints {
		if seen[ep.ID] {
			errs = append(errs, &ValidationError{
				Code: "USECASE_DUPLICATE_ID", Message: "duplicate entry_point id: " + ep.ID, Refs: []string{ep.ID},
			})
		}
		seen[ep.ID] = true
	}
	seenFx := map[string]bool{}
	for _, id := range uc.FixtureIDs {
		if seenFx[id] {
			errs = append(errs, &ValidationError{
				Code: "USECASE_DUPLICATE_ID", Message: "duplicate fixture_id: " + id, Refs: []string{id},
			})
		}
		seenFx[id] = true
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

func checkArchetypeScope(uc *UseCase) error {
	scope := map[enums.Archetype]bool{}
	for _, a := range uc.ArchetypeScope {
		scope[a] = true
	}
	var errs []error
	for _, ep := range uc.EntryPoints {
		if !scope[ep.Archetype] {
			errs = append(errs, &ValidationError{
				Code:    "USECASE_ARCHETYPE_OUT_OF_SCOPE",
				Message: "entry_point " + ep.ID + " archetype " + string(ep.Archetype) + " not in archetype_scope",
				Refs:    []string{ep.ID},
			})
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// parseAllSegments returns the flat slice of every parsed segment across
// all given/when/then lines, plus any *ValidationError wrapping a
// template.ParseError. If uc has cached segments (set by the loader),
// reuse them; otherwise reparse.
func parseAllSegments(uc *UseCase) ([]template.Segment, []error) {
	var out []template.Segment
	var errs []error

	collect := func(cached [][]template.Segment, lines []string) {
		if cached != nil {
			for _, segs := range cached {
				out = append(out, segs...)
			}
			return
		}
		for _, line := range lines {
			segs, err := template.Parse(line)
			if err != nil {
				pe, _ := err.(*template.ParseError)
				ve := &ValidationError{
					Code:    "USECASE_TEMPLATE_PARSE",
					Message: err.Error(),
				}
				if pe != nil {
					ve.Location = Position{Line: pe.Pos.Line, Col: pe.Pos.Col, Offset: pe.Pos.Offset}
					if isBadSpecMsg(pe.Msg) {
						ve.Code = "USECASE_TEMPLATE_BAD_SPEC_FIELD"
					}
				}
				errs = append(errs, ve)
				continue
			}
			out = append(out, segs...)
		}
	}

	collect(uc.givenSegs, uc.Given)
	collect(uc.whenSegs, uc.When)
	collect(uc.thenSegs, uc.Then)
	return out, errs
}

// isBadSpecMsg detects parser messages we surface as
// USECASE_TEMPLATE_BAD_SPEC_FIELD rather than the generic PARSE code.
// Matching by substring is fragile but kept here to avoid coupling
// validator code to parser internals beyond what's stable in tests.
func isBadSpecMsg(msg string) bool {
	return strings.Contains(msg, "only accepts '.spec.") ||
		strings.Contains(msg, "spec access is single-key only")
}

func checkRefsAndStore(uc *UseCase, segs []template.Segment, store fixture.FixtureStore) error {
	var errs []error

	declared := map[string]bool{}
	for _, id := range uc.FixtureIDs {
		declared[id] = true
	}
	entryByID := map[string]bool{}
	for _, ep := range uc.EntryPoints {
		entryByID[ep.ID] = true
	}

	usedFixtures := map[string]bool{}

	for _, s := range segs {
		switch v := s.(type) {
		case template.FixtureRef:
			usedFixtures[v.ID] = true
			if !declared[v.ID] {
				errs = append(errs, &ValidationError{
					Code:     "USECASE_FIXTURE_USED_UNDECLARED",
					Message:  "fixture " + v.ID + " referenced in text but not in fixture_ids[]",
					Location: Position{Line: v.Pos.Line, Col: v.Pos.Col, Offset: v.Pos.Offset},
					Refs:     []string{v.ID},
				})
			}
		case template.EntryPointRef:
			if !entryByID[v.ID] {
				errs = append(errs, &ValidationError{
					Code:     "USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT",
					Message:  "entry_point " + v.ID + " referenced in text but not declared",
					Location: Position{Line: v.Pos.Line, Col: v.Pos.Col, Offset: v.Pos.Offset},
					Refs:     []string{v.ID},
				})
			}
		}
	}

	for _, id := range uc.FixtureIDs {
		if _, ok := store.LookupFixture(id); !ok {
			errs = append(errs, &ValidationError{
				Code:    "USECASE_FIXTURE_NOT_IN_STORE",
				Message: "fixture_id " + id + " not found in fixture store",
				Refs:    []string{id},
			})
		}
	}

	for _, id := range uc.FixtureIDs {
		if !usedFixtures[id] {
			errs = append(errs, &ValidationError{
				Code:    "USECASE_FIXTURE_DECLARED_UNUSED",
				Message: "fixture_id " + id + " declared but not referenced in any given/when/then template",
				Refs:    []string{id},
			})
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
