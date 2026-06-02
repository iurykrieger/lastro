package usecase

import (
	"errors"
	"testing"

	"github.com/iurykrieger/lastro/internal/entrypoint"
	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/usecase/internal/fixturestub"
)

// validUseCase returns a UseCase that passes every structural check.
func validUseCase() *UseCase {
	return &UseCase{
		SchemaVersion:  "2.0.0",
		ID:             "valid-uc",
		Title:          "Valid",
		ArchetypeScope: []enums.Archetype{enums.ArchetypeHTTPAPI},
		EntryPoints: []entrypoint.EntryPoint{
			{ID: "ep", Archetype: enums.ArchetypeHTTPAPI, Spec: map[string]any{"method": "GET", "path": "/x"}},
		},
		Given: []string{"g"},
		When:  []string{"w"},
		Then:  []string{"t"},
	}
}

func TestValidateAcceptsMinimallyValid(t *testing.T) {
	uc := validUseCase()
	if err := Validate(uc, fixturestub.New(nil)); err != nil {
		t.Errorf("Validate err: %v", err)
	}
}

func TestValidateRejectsMissingRequiredField(t *testing.T) {
	uc := validUseCase()
	uc.Title = ""
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_REQUIRED_FIELD_MISSING") {
		t.Errorf("want USECASE_REQUIRED_FIELD_MISSING, got %v", err)
	}
}

func TestValidateRejectsBadCharsetID(t *testing.T) {
	uc := validUseCase()
	uc.ID = "Bad_ID"
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_ID_CHARSET") {
		t.Errorf("want USECASE_ID_CHARSET, got %v", err)
	}
}

func TestValidateRejectsDuplicateEntryPointID(t *testing.T) {
	uc := validUseCase()
	uc.EntryPoints = append(uc.EntryPoints, uc.EntryPoints[0])
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_DUPLICATE_ID") {
		t.Errorf("want USECASE_DUPLICATE_ID, got %v", err)
	}
}

func TestValidateRejectsDuplicateFixtureID(t *testing.T) {
	uc := validUseCase()
	uc.FixtureIDs = []string{"fx-a", "fx-a"}
	uc.Given = []string{"see ${{fixtures.fx-a}}"}
	err := Validate(uc, fixturestub.New(map[string]string{"fx-a": `{}`}))
	if !hasCode(err, "USECASE_DUPLICATE_ID") {
		t.Errorf("want USECASE_DUPLICATE_ID, got %v", err)
	}
}

func TestValidateRejectsArchetypeOutOfScope(t *testing.T) {
	uc := validUseCase()
	uc.EntryPoints[0].Archetype = enums.ArchetypeCLI
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_ARCHETYPE_OUT_OF_SCOPE") {
		t.Errorf("want USECASE_ARCHETYPE_OUT_OF_SCOPE, got %v", err)
	}
}

func TestValidateRejectsTemplateParseError(t *testing.T) {
	uc := validUseCase()
	uc.Given = []string{"bad ${{ ${{fixtures.x}} }}"}
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_TEMPLATE_PARSE") {
		t.Errorf("want USECASE_TEMPLATE_PARSE, got %v", err)
	}
}

func TestValidateRejectsTemplateBadSpecField(t *testing.T) {
	uc := validUseCase()
	uc.When = []string{"${{entry_points.ep.archetype}}"}
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_TEMPLATE_BAD_SPEC_FIELD") {
		t.Errorf("want USECASE_TEMPLATE_BAD_SPEC_FIELD, got %v", err)
	}
}

func TestValidateRejectsUnknownEntryPointRef(t *testing.T) {
	uc := validUseCase()
	uc.When = []string{"${{entry_points.ep-missing}}"}
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT") {
		t.Errorf("want USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT, got %v", err)
	}
}

func TestValidateRejectsFixtureUsedButUndeclared(t *testing.T) {
	uc := validUseCase()
	uc.Given = []string{"see ${{fixtures.fx-undeclared}}"}
	err := Validate(uc, fixturestub.New(map[string]string{"fx-undeclared": `{}`}))
	if !hasCode(err, "USECASE_FIXTURE_USED_UNDECLARED") {
		t.Errorf("want USECASE_FIXTURE_USED_UNDECLARED, got %v", err)
	}
}

func TestValidateRejectsFixtureNotInStore(t *testing.T) {
	uc := validUseCase()
	uc.FixtureIDs = []string{"fx-not-in-store"}
	uc.Given = []string{"${{fixtures.fx-not-in-store}}"}
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_FIXTURE_NOT_IN_STORE") {
		t.Errorf("want USECASE_FIXTURE_NOT_IN_STORE, got %v", err)
	}
}

func TestValidateRejectsDeclaredFixtureUnused(t *testing.T) {
	uc := validUseCase()
	uc.FixtureIDs = []string{"fx-orphan"}
	err := Validate(uc, fixturestub.New(map[string]string{"fx-orphan": `{}`}))
	if !hasCode(err, "USECASE_FIXTURE_DECLARED_UNUSED") {
		t.Errorf("want USECASE_FIXTURE_DECLARED_UNUSED, got %v", err)
	}
}

func TestValidateAccumulatesMultipleErrors(t *testing.T) {
	uc := validUseCase()
	uc.Title = ""
	uc.ID = "Bad_ID"
	uc.EntryPoints[0].Archetype = enums.ArchetypeCLI
	err := Validate(uc, fixturestub.New(nil))
	if !hasCode(err, "USECASE_REQUIRED_FIELD_MISSING") {
		t.Error("missing required-field error in joined error")
	}
	if !hasCode(err, "USECASE_ID_CHARSET") {
		t.Error("missing id-charset error in joined error")
	}
	if !hasCode(err, "USECASE_ARCHETYPE_OUT_OF_SCOPE") {
		t.Error("missing archetype-scope error in joined error")
	}
}

// hasCode walks the joined error and returns true if any *ValidationError
// in the tree carries the given code.
func hasCode(err error, code string) bool {
	if err == nil {
		return false
	}
	if u, ok := err.(interface{ Unwrap() []error }); ok {
		for _, c := range u.Unwrap() {
			if hasCode(c, code) {
				return true
			}
		}
		return false
	}
	var ve *ValidationError
	if errors.As(err, &ve) && ve.Code == code {
		return true
	}
	return false
}
