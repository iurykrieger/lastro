package usecase

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/usecase/template"
)

// supportedSchemaVersion is the only schema_version this loader accepts.
// Mismatch produces USECASE_SCHEMA_VERSION_UNSUPPORTED. The frozen
// schemas/use-case.yaml is at 2.0.0.
const supportedSchemaVersion = "2.0.0"

// Load deserializes a use-case YAML, parses every template line, and
// runs Validate. Returns the fully-validated *UseCase or a non-nil
// error. The loader never returns a partially-constructed UseCase.
func Load(data []byte, store fixture.FixtureStore) (*UseCase, error) {
	uc, err := UnmarshalOnly(data)
	if err != nil {
		return nil, err
	}

	if uc.SchemaVersion != supportedSchemaVersion {
		return nil, &ValidationError{
			Code:    "USECASE_SCHEMA_VERSION_UNSUPPORTED",
			Message: "schema_version " + uc.SchemaVersion + " is not supported (want " + supportedSchemaVersion + ")",
		}
	}

	uc.givenSegs = parseLines(uc.Given)
	uc.whenSegs = parseLines(uc.When)
	uc.thenSegs = parseLines(uc.Then)

	if err := Validate(uc, store); err != nil {
		return nil, err
	}
	return uc, nil
}

// UnmarshalOnly deserializes the YAML into a *UseCase without running
// validation. Exposed for tests that need the raw structure.
func UnmarshalOnly(data []byte) (*UseCase, error) {
	var uc UseCase
	if err := yaml.Unmarshal(data, &uc); err != nil {
		return nil, fmt.Errorf("usecase: yaml unmarshal: %w", err)
	}
	return &uc, nil
}

// parseLines parses each string; lines that fail to parse become a nil
// entry so slice index alignment with Given/When/Then is preserved.
// Validate's own re-parse surfaces ParseError; here we just populate the
// cache for the resolver/label renderer.
func parseLines(lines []string) [][]template.Segment {
	out := make([][]template.Segment, len(lines))
	for i, line := range lines {
		segs, err := template.Parse(line)
		if err != nil {
			out[i] = nil
			continue
		}
		out[i] = segs
	}
	return out
}
