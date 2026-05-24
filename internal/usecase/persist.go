package usecase

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
)

// Persist validates an LLM-emitted use-case YAML against the on-disk
// fixtures under <harnessDir>/fixtures/, patch-bumps its schema_version,
// and atomically writes it to <harnessDir>/use-cases/<id>.yaml.
//
// Returns a *persisterror.Error on validation failure; nothing is
// written. Cross-entity check: every fixture id referenced in the
// use-case's fixture_ids list must exist as a file in
// <harnessDir>/fixtures/ and be loadable by the fixture store.
func Persist(content []byte, harnessDir string) error {
	// Load on-disk fixtures so the use-case loader can resolve refs.
	fxDir := filepath.Join(harnessDir, "fixtures")
	store, err := loadFixtureStoreOrEmpty(fxDir)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.FixtureBinding,
			EntityType: "use-case",
			Message:    fmt.Sprintf("load fixtures: %v", err),
		}
	}

	uc, loadErr := Load(content, store)
	if loadErr != nil {
		return mapLoadError(uc, content, loadErr)
	}

	// Map-based bump to preserve all original fields exactly (the typed
	// UseCase struct doesn't round-trip entry_points[].spec or the
	// given/when/then text verbatim via yaml.Marshal).
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "use-case",
			EntityID:   uc.ID,
			Message:    fmt.Sprintf("re-parse for bump: %v", err),
		}
	}

	targetPath := filepath.Join(harnessDir, "use-cases", uc.ID+".yaml")
	bumped, err := bumpSchemaVersion(targetPath, uc.SchemaVersion)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "use-case",
			EntityID:   uc.ID,
			Message:    fmt.Sprintf("schema_version bump: %v", err),
		}
	}
	raw["schema_version"] = bumped

	out, err := yaml.Marshal(raw)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "use-case",
			EntityID:   uc.ID,
			Message:    fmt.Sprintf("marshal: %v", err),
		}
	}
	if err := atomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "use-case",
			EntityID:   uc.ID,
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

// loadFixtureStoreOrEmpty returns an empty store if fxDir doesn't exist,
// or the populated store from LoadDirectory otherwise.
func loadFixtureStoreOrEmpty(fxDir string) (fixture.FixtureStore, error) {
	if _, err := os.Stat(fxDir); errors.Is(err, os.ErrNotExist) {
		s, _ := fixture.NewStore()
		return s, nil
	}
	return fixture.LoadDirectory(fxDir)
}

// mapLoadError translates a usecase.Load error into a *persisterror.Error.
// Load may return an errors.Join of multiple *ValidationError values.
// We walk the unwrapped slice to find the most specific Kind: if any
// error is fixture-binding, we return FixtureBinding; if any is a
// template error, TemplateResolution; otherwise SchemaViolation.
//
// Actual error codes found in internal/usecase/validate.go:
//   - USECASE_FIXTURE_NOT_IN_STORE       → FixtureBinding
//   - USECASE_FIXTURE_USED_UNDECLARED    → FixtureBinding
//   - USECASE_FIXTURE_DECLARED_UNUSED    → FixtureBinding
//   - USECASE_TEMPLATE_PARSE            → TemplateResolution
//   - USECASE_TEMPLATE_BAD_SPEC_FIELD   → TemplateResolution
//   - USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT → TemplateResolution
//   - all others (REQUIRED_FIELD_MISSING, ID_CHARSET, etc.) → SchemaViolation
func mapLoadError(uc *UseCase, content []byte, err error) error {
	entityID := ""
	if uc != nil {
		entityID = uc.ID
	} else {
		// Try to extract id from content for the error message even when
		// Load returned nil for the UseCase (e.g., unmarshal failure).
		var raw struct {
			ID string `json:"id" yaml:"id"`
		}
		if e := yaml.Unmarshal(content, &raw); e == nil {
			entityID = raw.ID
		}
	}

	kind := classifyLoadErr(err)
	return &persisterror.Error{
		Kind:       kind,
		EntityType: "use-case",
		EntityID:   entityID,
		Message:    err.Error(),
	}
}

// classifyLoadErr walks an errors.Join tree and returns the most specific
// persisterror.Kind. Priority: FixtureBinding > TemplateResolution > SchemaViolation.
func classifyLoadErr(err error) persisterror.Kind {
	best := persisterror.SchemaViolation
	walkErrors(err, func(e error) {
		var ve *ValidationError
		if !errors.As(e, &ve) {
			return
		}
		switch ve.Code {
		case "USECASE_FIXTURE_NOT_IN_STORE",
			"USECASE_FIXTURE_USED_UNDECLARED",
			"USECASE_FIXTURE_DECLARED_UNUSED":
			best = persisterror.FixtureBinding
		case "USECASE_TEMPLATE_PARSE",
			"USECASE_TEMPLATE_BAD_SPEC_FIELD",
			"USECASE_TEMPLATE_UNKNOWN_ENTRY_POINT":
			if best != persisterror.FixtureBinding {
				best = persisterror.TemplateResolution
			}
		}
	})
	return best
}

// walkErrors calls fn for every leaf in an errors.Join tree (errors
// that implement Unwrap() []error), and for the error itself if it is
// not a join.
func walkErrors(err error, fn func(error)) {
	type joinUnwrapper interface{ Unwrap() []error }
	if j, ok := err.(joinUnwrapper); ok {
		for _, e := range j.Unwrap() {
			walkErrors(e, fn)
		}
		return
	}
	fn(err)
}

// --- helpers inline-copied from internal/stack/persist.go ---
// (Phase 5 Task 5.4 extracts these to internal/persisthelp.)

func bumpSchemaVersion(targetPath, input string) (string, error) {
	existing, err := os.ReadFile(targetPath)
	if errors.Is(err, os.ErrNotExist) {
		return input, nil
	}
	if err != nil {
		return "", err
	}
	var head struct {
		SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	}
	if err := yaml.Unmarshal(existing, &head); err != nil {
		return "", fmt.Errorf("parse existing schema_version: %w", err)
	}
	if head.SchemaVersion == "" {
		return input, nil
	}
	return bumpPatch(head.SchemaVersion)
}

func bumpPatch(v string) (string, error) {
	var maj, min, patch int
	n, err := fmt.Sscanf(v, "%d.%d.%d", &maj, &min, &patch)
	if err != nil || n != 3 {
		return "", fmt.Errorf("not a semver: %q", v)
	}
	return fmt.Sprintf("%d.%d.%d", maj, min, patch+1), nil
}

func atomicWrite(targetPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tmp := targetPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, targetPath)
}
