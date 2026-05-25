package healloop

import (
	"context"
	"errors"
	"sync"

	usecase "github.com/iurykrieger/lastro/internal/runtime/aggregator/usecase"
)

// stubLLM scripts EditPlan responses. If plans is empty, Propose returns
// (EditPlan{}, err). Otherwise the i-th call returns plans[i % len(plans)].
// onCall, when set, runs on every Propose with the PromptInput just
// received; it can call t.Error / t.Fatal to assert on inputs.
type stubLLM struct {
	plans  []EditPlan
	err    error
	calls  int
	onCall func(in PromptInput)
}

func (s *stubLLM) Propose(_ context.Context, in PromptInput) (EditPlan, error) {
	if s.onCall != nil {
		s.onCall(in)
	}
	s.calls++
	if s.err != nil {
		return EditPlan{}, s.err
	}
	if len(s.plans) == 0 {
		return EditPlan{}, nil
	}
	return s.plans[(s.calls-1)%len(s.plans)], nil
}

// stubRevalidator scripts UseCaseVerdict responses. The i-th call returns
// verdicts[i]; calling beyond the script returns the last entry.
type stubRevalidator struct {
	verdicts []usecase.UseCaseVerdict
	err      error
	calls    int
}

func (s *stubRevalidator) Revalidate(_ context.Context, _ string) (usecase.UseCaseVerdict, error) {
	if s.err != nil {
		return usecase.UseCaseVerdict{}, s.err
	}
	idx := s.calls
	if idx >= len(s.verdicts) {
		idx = len(s.verdicts) - 1
	}
	s.calls++
	if idx < 0 {
		return usecase.UseCaseVerdict{}, nil
	}
	return s.verdicts[idx], nil
}

// recordingTxHandle counts Apply/Revert/Commit calls and reports a scripted
// error from any of them if the corresponding field is set.
type recordingTxHandle struct {
	paths       []string
	applyErr    error
	revertErr   error
	commitErr   error
	applied     bool
	appliedPlan EditPlan
	reverted    bool
	committed   bool
	mu          sync.Mutex
}

func (h *recordingTxHandle) Apply(plan EditPlan) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.applied = true
	h.appliedPlan = plan
	return h.applyErr
}

func (h *recordingTxHandle) Revert() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reverted = true
	return h.revertErr
}

func (h *recordingTxHandle) Commit() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.committed = true
	return h.commitErr
}

// stubTransactor produces recordingTxHandles. If snapErr is non-nil,
// Snapshot returns it and produces no handle. applyErrs/revertErrs/commitErrs
// (if non-empty) wire the i-th handle's Apply/Revert/Commit to return the
// scripted error at that index.
type stubTransactor struct {
	snapErr    error
	applyErrs  []error
	revertErrs []error
	commitErrs []error
	snapshots  []*recordingTxHandle
}

func (t *stubTransactor) Snapshot(_ context.Context, paths []string) (TxHandle, error) {
	if t.snapErr != nil {
		return nil, t.snapErr
	}
	idx := len(t.snapshots)
	h := &recordingTxHandle{paths: append([]string(nil), paths...)}
	if idx < len(t.applyErrs) {
		h.applyErr = t.applyErrs[idx]
	}
	if idx < len(t.revertErrs) {
		h.revertErr = t.revertErrs[idx]
	}
	if idx < len(t.commitErrs) {
		h.commitErr = t.commitErrs[idx]
	}
	t.snapshots = append(t.snapshots, h)
	return h, nil
}

// errStub is a convenience for tests that just want a non-nil error sentinel.
var errStub = errors.New("stub error")
