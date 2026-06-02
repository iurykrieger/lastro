package fixturebinder

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/usecase"
)

// Binder writes fixture payloads to disk under ScratchDir. The caller owns
// ScratchDir's lifecycle (mkdir + cleanup); the binder neither creates the
// directory nor removes files when Bind returns.
type Binder struct {
	// ScratchDir is the absolute path under which payload files are written.
	// Must exist when Bind is called.
	ScratchDir string
}

// Bind resolves an explicit list of fixture ids against the use case's owned
// fixtures, writes each payload to ScratchDir, and returns a StepBinding.
// ids should be derived from CollectFixtureRefs on the step's run/with fields.
// See the package doc for the spec reference.
func (b *Binder) Bind(ids []string, owningUseCase *usecase.UseCase, store fixture.FixtureStore) (StepBinding, error) {
	binding := StepBinding{
		Env:      map[string]string{},
		Files:    map[string]string{},
		BoundIDs: []string{},
	}
	if len(ids) == 0 {
		return binding, nil
	}

	owned := make(map[string]struct{}, len(owningUseCase.FixtureIDs))
	for _, id := range owningUseCase.FixtureIDs {
		owned[id] = struct{}{}
	}

	// Sort to get deterministic BoundIDs and file-write order.
	// Copy first to avoid mutating the caller's slice.
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)

	for _, id := range sorted {
		if _, ok := owned[id]; !ok {
			return StepBinding{}, &BindError{
				Code: "fixture-not-owned", FixtureID: id, UseCaseID: owningUseCase.ID,
			}
		}
		fx, ok := store.LookupFixture(id)
		if !ok {
			return StepBinding{}, &BindError{
				Code: "fixture-not-found", FixtureID: id, UseCaseID: owningUseCase.ID,
			}
		}
		path := filepath.Join(b.ScratchDir, fx.ID+extensionFor(fx.ContentType))
		if err := os.WriteFile(path, fx.Payload, 0o600); err != nil {
			return StepBinding{}, &BindError{
				Code: "write-failed", FixtureID: id, UseCaseID: owningUseCase.ID, Cause: err,
			}
		}
		binding.Env[normalizeEnvName(fx.ID)] = path
		binding.Files[fx.ID] = path
		binding.BoundIDs = append(binding.BoundIDs, fx.ID)
	}
	return binding, nil
}
