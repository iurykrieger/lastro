package healloop

import (
	"context"

	"github.com/iurykrieger/lastro/internal/aggregate"
	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
	"github.com/iurykrieger/lastro/internal/sensor"
	upkg "github.com/iurykrieger/lastro/internal/usecase"
)

// Status is the loop-level outcome. Distinct from a bare error, which is
// reserved for infrastructure failures (Snapshot/Apply/Revalidate errors
// and ctx.Done()).
type Status string

const (
	// StatusHealed: re-validate passed after applying an EditPlan, or the
	// input verdict was already passing.
	StatusHealed Status = "healed"
	// StatusExhausted: ran cap iterations, the last attempt was reverted,
	// working tree restored to entry state.
	StatusExhausted Status = "exhausted"
	// StatusAbandoned: first Propose returned an error, an empty plan, or
	// a plan with an invalid path. No snapshot was taken.
	StatusAbandoned Status = "abandoned"
)

// EditOp is the operation an EditFile describes.
type EditOp string

const (
	// OpWrite replaces (or creates) the file with EditFile.Content.
	OpWrite EditOp = "write"
	// OpDelete removes the file. EditFile.Content is ignored.
	OpDelete EditOp = "delete"
)

// EditFile is one file mutation in an EditPlan. Path is repo-root-relative.
type EditFile struct {
	Path    string
	Op      EditOp
	Content string
}

// EditPlan is the contract the LLMClient returns from Propose.
type EditPlan struct {
	Files []EditFile
	// Rationale is the LLM's free-text explanation; surfaced to the caller
	// (CLI/skill) for display, never interpreted by the loop.
	Rationale string
}

// Attempt records one iteration of the loop. The successful attempt has
// Reverted=false; all entries in History have Reverted=true.
type Attempt struct {
	Iteration int
	Plan      EditPlan
	Verdict   usecase.UseCaseVerdict
	Reverted  bool
}

// PromptInput is the structured input the LLMClient consumes. The
// LLMClient impl owns prompt-template formatting; healloop never
// produces a rendered string.
type PromptInput struct {
	UseCase *upkg.UseCase
	Verdict usecase.UseCaseVerdict
	Hints   []usecase.AngleHint // == Verdict.HealHints, hoisted for convenience
	History []Attempt           // prior attempts, oldest first; empty on iteration 1
}

// LLMClient is the seam between the deterministic loop and the LLM-driving
// skill (B5). Sync because heal is the slow path; simplicity wins.
type LLMClient interface {
	Propose(ctx context.Context, in PromptInput) (EditPlan, error)
}

// Transactor snapshots file state before an EditPlan is applied and
// produces a TxHandle that can revert or commit. The default impl
// (DefaultTransactor) auto-detects git vs. file-backup mode.
type Transactor interface {
	Snapshot(ctx context.Context, paths []string) (TxHandle, error)
}

// TxHandle owns the snapshot→apply→revert|commit lifecycle for one
// iteration. Apply is called exactly once between Snapshot and the
// terminal Revert or Commit; tests can substitute a stub that records
// without writing.
type TxHandle interface {
	// Apply writes (or deletes) every EditFile in plan under the
	// transactor's repoRoot. Returns the first error encountered.
	// The caller is responsible for Revert on failure.
	Apply(plan EditPlan) error
	// Revert restores the snapshot. For file mode: writes original bytes
	// back and deletes newly-created files. For git mode: checkout HEAD,
	// delete newly-created files, then stash apply + stash drop.
	Revert() error
	// Commit discards the snapshot. For file mode: no-op. For git mode:
	// git stash drop. The on-disk edits are kept either way.
	Commit() error
}

// Revalidator re-runs every assertion sensor in the use case and returns
// the new UseCaseVerdict. Observational sensors are skipped and their
// original AggregateSignals are carried forward into the re-aggregation.
type Revalidator interface {
	Revalidate(ctx context.Context, useCaseID string) (usecase.UseCaseVerdict, error)
}

// SensorLookup returns the sensors that belong to a use case. *sensor.Store
// satisfies it via the ForUseCase method (adapted at the construction site).
type SensorLookup interface {
	SensorsForUseCase(useCaseID string) []sensor.Sensor
}

// UseCaseLookup returns a use case by id. The concrete repo's use-case
// store will adapt to this interface at the construction site.
type UseCaseLookup interface {
	Lookup(useCaseID string) (*upkg.UseCase, bool)
}

// SensorRunner is the seam over *lifecycle.Lifecycle that lets revalidator
// tests inject a stub. The real *lifecycle.Lifecycle satisfies this via
// its RunSensor method.
type SensorRunner interface {
	RunSensor(ctx context.Context, sensorID string, expectedObs []string) (aggregate.AggregateSignal, error)
}
