package usecase

import (
	"fmt"
	"regexp"
)

// idPattern matches the kebab-case id format frozen by the schema-freeze
// gate. See schemas/use-case.yaml $defs.Id.
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

const maxIDLen = 128

// ValidationError is the typed error every validator check produces.
// Code is a stable contract for tooling; Location and Refs are best-effort.
type ValidationError struct {
	Code     string
	Message  string
	Location Position
	Refs     []string
}

// Position locates an error inside the loaded YAML. Zero Position means
// the error is not source-attributable (e.g., id charset on the top-level
// id, or a fixture-store miss).
type Position struct {
	Line   int
	Col    int
	Offset int
}

func (e *ValidationError) Error() string {
	return e.Code + ": " + e.Message
}

// ValidateID returns nil if s matches `^[a-z][a-z0-9-]*$` and is 1-128
// chars; otherwise a *ValidationError with code USECASE_ID_CHARSET or
// USECASE_ID_TOO_LONG.
func ValidateID(s string) error {
	if len(s) > maxIDLen {
		return &ValidationError{
			Code:    "USECASE_ID_TOO_LONG",
			Message: fmt.Sprintf("id length %d exceeds max %d", len(s), maxIDLen),
			Refs:    []string{s},
		}
	}
	if !idPattern.MatchString(s) {
		return &ValidationError{
			Code:    "USECASE_ID_CHARSET",
			Message: fmt.Sprintf("id %q must match ^[a-z][a-z0-9-]*$", s),
			Refs:    []string{s},
		}
	}
	return nil
}
