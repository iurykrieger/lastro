package healloop

import (
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
	// t.TempDir() resolves to /tmp (Linux) or a system temp directory that is
	// not a git work tree. DefaultTransactor must fall back to fileTransactor
	// whether or not git is installed.
	dir := t.TempDir()
	tx := DefaultTransactor(dir)
	if _, ok := tx.(*fileTransactor); !ok {
		t.Errorf("DefaultTransactor type = %T, want *fileTransactor", tx)
	}
}
