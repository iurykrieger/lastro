package healloop

import (
	"os/exec"
	"testing"
)

func TestDefaultTransactor_ReturnsGitWhenInGitRepo(t *testing.T) {
	requireGit(t)
	dir := gitTempRepo(t)

	tx := DefaultTransactor(dir)
	if _, ok := tx.(*gitTransactor); !ok {
		t.Errorf("DefaultTransactor type = %T, want *gitTransactor", tx)
	}
}

func TestDefaultTransactor_ReturnsFileWhenNotInGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		// We need git absent or repoRoot outside any git repo. Use a temp
		// dir that's not git-initialized; even with git installed, this
		// should fall back to file mode.
	}
	dir := t.TempDir()
	tx := DefaultTransactor(dir)
	if _, ok := tx.(*fileTransactor); !ok {
		t.Errorf("DefaultTransactor type = %T, want *fileTransactor", tx)
	}
}
