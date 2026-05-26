package sensor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	"github.com/iurykrieger/lastro/internal/enums"
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/persisterror"
	"github.com/iurykrieger/lastro/internal/persisthelp"
	"github.com/iurykrieger/lastro/internal/stack"
)

// Persist validates an LLM-emitted sensor YAML against the on-disk
// stack-manifest, fixtures, and use-cases under harnessDir, patch-bumps
// its schema_version against any prior on-disk version, and atomically
// writes it to <harnessDir>/sensors/<id>.yaml.
//
// Returns a *persisterror.Error on validation failure; nothing is
// written. Cross-entity checks (in order):
//
//	(a) stack-manifest.yaml exists; sensor's top-level uses: ⊆ that
//	    manifest's components[*].id (Grounding via ValidateAgainstStack).
//	(b) sensor.angle ∈ stack-manifest.applicable_angles.
//	(c) use-cases/<sensor.use_case_id>.yaml exists.
//	(d) each step's uses: ⊆ fixtures whose use_case_id matches sensor's.
func Persist(content []byte, harnessDir string) error {
	s, err := LoadSensorBytes(content)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			Message:    err.Error(),
		}
	}

	// (a) Stack manifest must exist; sensor must be grounded against it.
	manifest, err := loadStackOrErr(harnessDir, s.ID)
	if err != nil {
		return err // already a *persisterror.Error
	}
	if err := ValidateAgainstStack(s, manifest); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.Grounding,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    err.Error(),
		}
	}

	// (b) Angle must be in manifest's applicable_angles.
	if !angleInList(s.Angle, manifest.ApplicableAngles) {
		return &persisterror.Error{
			Kind:       persisterror.AngleNotApplicable,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message: fmt.Sprintf("angle %q is not in stack-manifest.applicable_angles %v",
				s.Angle, manifest.ApplicableAngles),
		}
	}

	// (c) Use-case must exist on disk.
	ucPath := filepath.Join(harnessDir, "use-cases", s.UseCaseID+".yaml")
	if _, err := os.Stat(ucPath); errors.Is(err, os.ErrNotExist) {
		return &persisterror.Error{
			Kind:       persisterror.MissingDependency,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("use-case %q not found at %s", s.UseCaseID, ucPath),
		}
	}

	// (d) Step uses ⊆ fixtures with matching use_case_id.
	store, err := loadFixtureStoreOrEmpty(filepath.Join(harnessDir, "fixtures"))
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.FixtureBinding,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("load fixtures: %v", err),
		}
	}
	if err := ValidateAgainstFixtures(s, fixtureOwner{store: store, useCaseID: s.UseCaseID}); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.FixtureBinding,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    err.Error(),
		}
	}

	// Map-based bump (preserves original field order/content faithfully).
	var raw map[string]any
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("re-parse for bump: %v", err),
		}
	}
	targetPath := filepath.Join(harnessDir, "sensors", s.ID+".yaml")
	bumped, err := persisthelp.BumpSchemaVersion(targetPath, s.SchemaVersion)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("schema_version bump: %v", err),
		}
	}
	raw["schema_version"] = bumped

	out, err := yaml.Marshal(raw)
	if err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("marshal: %v", err),
		}
	}
	if err := persisthelp.AtomicWrite(targetPath, out); err != nil {
		return &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   s.ID,
			Message:    fmt.Sprintf("write %s: %v", targetPath, err),
		}
	}
	return nil
}

func loadStackOrErr(harnessDir, sensorID string) (stack.StackManifest, error) {
	path := filepath.Join(harnessDir, "stack-manifest.yaml")
	m, err := stack.Load(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return stack.StackManifest{}, &persisterror.Error{
				Kind:       persisterror.MissingDependency,
				EntityType: "sensor",
				EntityID:   sensorID,
				Message:    fmt.Sprintf("stack-manifest not found at %s", path),
			}
		}
		return stack.StackManifest{}, &persisterror.Error{
			Kind:       persisterror.SchemaViolation,
			EntityType: "sensor",
			EntityID:   sensorID,
			Message:    fmt.Sprintf("load stack-manifest: %v", err),
		}
	}
	return m, nil
}

func loadFixtureStoreOrEmpty(fxDir string) (*fixture.Store, error) {
	if _, err := os.Stat(fxDir); errors.Is(err, os.ErrNotExist) {
		return fixture.NewStore()
	}
	return fixture.LoadDirectory(fxDir)
}

// angleInList reports whether a is in the list.
func angleInList(a enums.ValidationAngle, list []enums.ValidationAngle) bool {
	for _, x := range list {
		if x == a {
			return true
		}
	}
	return false
}

// fixtureOwner adapts a fixture.Store + a use-case id to the
// UseCaseFixtureOwnership interface that ValidateAgainstFixtures expects.
type fixtureOwner struct {
	store     *fixture.Store
	useCaseID string
}

func (o fixtureOwner) OwnedFixtureIDs(useCaseID string) []string {
	if useCaseID != o.useCaseID {
		return nil
	}
	var ids []string
	for _, fx := range o.store.FixturesForUseCase(useCaseID) {
		ids = append(ids, fx.ID)
	}
	return ids
}
