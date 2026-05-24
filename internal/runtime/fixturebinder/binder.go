package fixturebinder

import (
	"github.com/iurykrieger/lastro/internal/fixture"
	"github.com/iurykrieger/lastro/internal/sensor"
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

// Bind resolves a step's `uses:` fixture ids against the use case's owned
// fixtures, writes each payload to ScratchDir, and returns a StepBinding.
// See spec §5 for the full behavior contract.
func (b *Binder) Bind(step sensor.Step, owningUseCase *usecase.UseCase, store fixture.FixtureStore) (StepBinding, error) {
	binding := StepBinding{
		Env:      map[string]string{},
		Files:    map[string]string{},
		BoundIDs: []string{},
	}
	if len(step.Uses) == 0 {
		return binding, nil
	}
	// Real binding logic implemented in subsequent tasks.
	return binding, nil
}
