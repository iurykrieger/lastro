package healloop

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oklog/ulid/v2"
)

// gitTransactor snapshots target paths via `git stash push -u -- <paths>`,
// records the resolved stash SHA so positional refs can't drift, and on
// Revert: discards working-tree changes, removes newly-created files, then
// re-applies the stash and drops it. See spec §7.2.
type gitTransactor struct {
	repoRoot string
}

type gitTxHandle struct {
	repoRoot string
	mu       sync.Mutex
	// stashSHA is empty when nothing was stashed (e.g., the target paths
	// were already clean). In that case Revert/Commit are no-ops for the
	// stash and only the `created` paths are processed.
	stashSHA string
	created  map[string]struct{}
	paths    []string
}

// Apply delegates the same filesystem write semantics as fileTxHandle.Apply.
// Defined on gitTxHandle so the TxHandle interface is satisfied without taking
// a dependency on fileTxHandle's internal map state.
// The mutex is held for the full I/O loop; see TxHandle's contract for
// the concurrency rules.
func (h *gitTxHandle) Apply(plan EditPlan) error {
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

// Snapshot records which target paths don't exist yet (so Revert can delete
// them), then pushes any dirty state for those paths into a uniquely named
// stash entry. Resolves the stash to a SHA immediately so positional refs
// (stash@{0}) can't drift if the stash list changes between Snapshot and
// Revert/Commit.
func (t *gitTransactor) Snapshot(ctx context.Context, paths []string) (TxHandle, error) {
	h := &gitTxHandle{
		repoRoot: t.repoRoot,
		created:  make(map[string]struct{}, len(paths)),
		paths:    append([]string(nil), paths...),
	}
	// Record which paths don't exist yet (newly-created files need deletion on Revert).
	for _, p := range paths {
		abs := filepath.Join(t.repoRoot, p)
		if _, err := os.Stat(abs); errors.Is(err, os.ErrNotExist) {
			h.created[p] = struct{}{}
		}
	}

	// Compose a uniquely identifiable stash message.
	msg := fmt.Sprintf("harness-heal-%s", ulid.MustNew(ulid.Now(), rand.Reader))

	// Only pass paths that already exist to `git stash push`. Non-existent
	// paths (tracked in h.created) aren't tracked by git yet, so including
	// them makes git exit 1 with "did not match any files" even though the
	// stash for existing paths succeeds. We don't need to stash non-existent
	// files — Revert will simply delete them.
	var existingPaths []string
	for _, p := range paths {
		if _, isNew := h.created[p]; !isNew {
			existingPaths = append(existingPaths, p)
		}
	}

	var out string
	var err error
	if len(existingPaths) > 0 {
		args := append([]string{"stash", "push", "-u", "-m", msg, "--"}, existingPaths...)
		out, err = runGit(ctx, t.repoRoot, args...)
	} else {
		// Nothing existing to stash; treat as "no local changes to save".
		out = "No local changes to save"
	}
	if err != nil {
		return nil, fmt.Errorf("healloop: git stash push: %w; output: %s", err, out)
	}

	// If there was nothing to stash, `git stash push` exits 0 and prints
	// "No local changes to save". Detect this and leave stashSHA empty.
	if strings.Contains(out, "No local changes to save") {
		return h, nil
	}

	shaOut, err := runGit(ctx, t.repoRoot, "rev-parse", "stash@{0}")
	if err != nil {
		return nil, fmt.Errorf("healloop: resolve stash SHA: %w; output: %s", err, shaOut)
	}
	h.stashSHA = strings.TrimSpace(shaOut)
	return h, nil
}

// Revert discards working-tree changes to target paths, deletes any files
// that didn't exist at snapshot time, then re-applies and drops the stash
// (if one was created). Errors are joined so all cleanup steps run even
// when an earlier step fails.
func (h *gitTxHandle) Revert() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	ctx := context.Background()
	var errs []error

	// 1. Discard working-tree changes to tracked target paths only.
	//    Mixing tracked + created paths makes `git checkout HEAD --` fail
	//    with "did not match any file" AND skip restoring the tracked ones.
	//    Split paths into tracked-only and run checkout against that subset.
	var tracked []string
	for _, p := range h.paths {
		if _, isNew := h.created[p]; !isNew {
			tracked = append(tracked, p)
		}
	}
	if len(tracked) > 0 {
		args := append([]string{"checkout", "HEAD", "--"}, tracked...)
		if out, err := runGit(ctx, h.repoRoot, args...); err != nil {
			errs = append(errs, fmt.Errorf("healloop: git checkout HEAD: %w; output: %s", err, out))
		}
	}

	// 2. Delete files newly created post-snapshot.
	for p := range h.created {
		abs := filepath.Join(h.repoRoot, p)
		if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("healloop: revert remove %q: %w", p, err))
		}
	}

	// 3. Re-apply the stash (if any) and drop it.
	if h.stashSHA != "" {
		if out, err := runGit(ctx, h.repoRoot, "stash", "apply", h.stashSHA); err != nil {
			errs = append(errs, fmt.Errorf("healloop: git stash apply: %w; output: %s", err, out))
		}
		// `git stash drop` does not accept object SHAs — it requires a positional
		// ref (stash@{n}). Resolve the SHA back to its current ref before dropping.
		if ref, err := stashRefBySHA(ctx, h.repoRoot, h.stashSHA); err != nil {
			errs = append(errs, fmt.Errorf("healloop: locate stash ref: %w", err))
		} else if out, err := runGit(ctx, h.repoRoot, "stash", "drop", ref); err != nil {
			errs = append(errs, fmt.Errorf("healloop: git stash drop: %w; output: %s", err, out))
		}
		h.stashSHA = ""
	}

	return errors.Join(errs...)
}

// Commit discards the stash without restoring working-tree content. The
// edited files remain as-is. If no stash was created (paths were already
// clean at snapshot time), Commit is a no-op.
func (h *gitTxHandle) Commit() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stashSHA == "" {
		return nil
	}
	ctx := context.Background()
	ref, err := stashRefBySHA(ctx, h.repoRoot, h.stashSHA)
	if err != nil {
		return fmt.Errorf("healloop: locate stash ref on commit: %w", err)
	}
	out, err := runGit(ctx, h.repoRoot, "stash", "drop", ref)
	if err != nil {
		return fmt.Errorf("healloop: git stash drop on commit: %w; output: %s", err, out)
	}
	h.stashSHA = ""
	return nil
}

// stashRefBySHA iterates the stash list to find the positional ref (stash@{n})
// whose object SHA matches the given SHA. `git stash drop` only accepts
// positional refs — it rejects raw object SHAs.
func stashRefBySHA(ctx context.Context, repoRoot, sha string) (string, error) {
	// `git stash list --format=%gd %H` prints lines like:
	//   stash@{0} <commit-sha>
	out, err := runGit(ctx, repoRoot, "stash", "list", "--format=%gd %H")
	if err != nil {
		return "", fmt.Errorf("healloop: list stash: %w; output: %s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == sha {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("healloop: stash SHA %s not found in stash list", sha)
}

// runGit shells out to `git -C <repoRoot> <args>` and returns combined output.
func runGit(ctx context.Context, repoRoot string, args ...string) (string, error) {
	full := append([]string{"-C", repoRoot}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
