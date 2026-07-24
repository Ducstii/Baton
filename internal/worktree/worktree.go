package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree represents a git worktree managed by Baton.
type Worktree struct {
	path   string
	Branch string
}

// sanitizeBranchSegment replaces slashes in a segment with hyphens to produce
// a git-safe branch name component.
func sanitizeBranchSegment(s string) string {
	return strings.ReplaceAll(s, "/", "-")
}

// Create creates a new git worktree at basePath/{runID}/{workUnitID}.
// It creates a new branch named baton/{runID}/{workUnitID}.
// The parent repository is determined from the current working directory.
func Create(basePath, runID, workUnitID string) (*Worktree, error) {
	worktreePath := filepath.Join(basePath, runID, workUnitID)

	// Check if the worktree path already exists.
	if _, err := os.Stat(worktreePath); err == nil {
		return nil, fmt.Errorf("worktree path already exists: %s", worktreePath)
	}

	branch := fmt.Sprintf("baton/%s/%s",
		sanitizeBranchSegment(runID),
		sanitizeBranchSegment(workUnitID),
	)

	cmd := exec.Command("git", "worktree", "add", worktreePath, "-b", branch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git worktree add failed: %w\noutput: %s", err, string(out))
	}

	return &Worktree{
		path:   worktreePath,
		Branch: branch,
	}, nil
}

// Path returns the filesystem path of the worktree.
func (w *Worktree) Path() string {
	return w.path
}

// Remove removes the worktree directory and deletes its branch.
func (w *Worktree) Remove() error {
	cmd := exec.Command("git", "worktree", "remove", w.path, "--force")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove failed: %w\noutput: %s", err, string(out))
	}
	return nil
}
