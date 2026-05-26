package healloop

import (
	"context"
	"os/exec"
)

// DefaultTransactor returns a git-backed Transactor when repoRoot is inside
// a git work tree, else a file-backup Transactor. The detection runs
// `git rev-parse --git-dir` once at construction time.
func DefaultTransactor(repoRoot string) Transactor {
	if isGitRepo(repoRoot) {
		return &gitTransactor{repoRoot: repoRoot}
	}
	return &fileTransactor{repoRoot: repoRoot}
}

// isGitRepo runs `git rev-parse --git-dir` in repoRoot. Returns false
// when git is not installed, repoRoot isn't a git work tree, or the
// command fails for any reason.
func isGitRepo(repoRoot string) bool {
	cmd := exec.CommandContext(context.Background(), "git", "-C", repoRoot, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}
