package healloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fileTransactor snapshots file bytes into memory and restores them on
// Revert. Used when no git repo is detected and as the unit-tested base
// for the in-process transactor seam.
type fileTransactor struct {
	repoRoot string
}

// fileTxHandle records the original bytes and "did not exist" status per
// path. Revert restores or deletes accordingly. Commit is a no-op.
type fileTxHandle struct {
	repoRoot string
	mu       sync.Mutex
	// originals stores the bytes that existed at snapshot time. A path
	// missing from this map means "no file existed".
	originals map[string][]byte
	// created tracks paths that did not exist at snapshot time. On Revert
	// they get os.Remove.
	created map[string]struct{}
}

// Snapshot records each path's current bytes (or marks as created-if-new)
// and returns a TxHandle.
func (t *fileTransactor) Snapshot(_ context.Context, paths []string) (TxHandle, error) {
	h := &fileTxHandle{
		repoRoot:  t.repoRoot,
		originals: make(map[string][]byte, len(paths)),
		created:   make(map[string]struct{}, len(paths)),
	}
	for _, p := range paths {
		abs := filepath.Join(t.repoRoot, p)
		b, err := os.ReadFile(abs)
		switch {
		case err == nil:
			h.originals[p] = b
		case errors.Is(err, os.ErrNotExist):
			h.created[p] = struct{}{}
		default:
			return nil, fmt.Errorf("healloop: snapshot read %q: %w", p, err)
		}
	}
	return h, nil
}

// Apply writes (or deletes) every EditFile in plan under repoRoot. Returns
// the first error encountered; on error, the caller is expected to Revert.
// Missing parent directories are created with 0o755.
func (h *fileTxHandle) Apply(plan EditPlan) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range plan.Files {
		abs := filepath.Join(h.repoRoot, f.Path)
		switch f.Op {
		case OpWrite:
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return fmt.Errorf("healloop: apply mkdirall %q: %w", f.Path, err)
			}
			if err := os.WriteFile(abs, []byte(f.Content), 0o644); err != nil {
				return fmt.Errorf("healloop: apply write %q: %w", f.Path, err)
			}
		case OpDelete:
			if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("healloop: apply remove %q: %w", f.Path, err)
			}
		default:
			return fmt.Errorf("healloop: apply unknown Op %q for %q", f.Op, f.Path)
		}
	}
	return nil
}

// Revert writes originals back and deletes paths that did not exist
// at snapshot time. Errors are joined; partial revert is reported but
// other paths are still processed.
func (h *fileTxHandle) Revert() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	var errs []error
	for path, bytes := range h.originals {
		abs := filepath.Join(h.repoRoot, path)
		if err := os.WriteFile(abs, bytes, 0o644); err != nil {
			errs = append(errs, fmt.Errorf("healloop: revert write %q: %w", path, err))
		}
	}
	for path := range h.created {
		abs := filepath.Join(h.repoRoot, path)
		if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("healloop: revert remove %q: %w", path, err))
		}
	}
	return errors.Join(errs...)
}

// Commit discards the snapshot. No filesystem effect — the working tree
// is whatever the caller wrote since Snapshot.
func (h *fileTxHandle) Commit() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.originals = nil
	h.created = nil
	return nil
}
