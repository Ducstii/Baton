package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo creates a temporary git repo with an initial commit.
// The caller's working directory is unchanged.
func setupRepo(t *testing.T) string {
	t.Helper()
	repoDir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repoDir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\noutput: %s", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("# test"), 0644); err != nil {
		t.Fatalf("write README failed: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	return repoDir
}

func TestCreateAndRemove(t *testing.T) {
	repoDir := setupRepo(t)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	defer os.Chdir(origDir) //nolint:errcheck

	basePath := filepath.Join(repoDir, "worktrees")
	runID := "run-abc"
	workUnitID := "unit-123"

	wt, err := Create(basePath, runID, workUnitID)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	expectedPath := filepath.Join(basePath, runID, workUnitID)
	if wt.Path() != expectedPath {
		t.Errorf("Path() = %q, want %q", wt.Path(), expectedPath)
	}
	expectedBranch := "baton/run-abc/unit-123"
	if wt.Branch != expectedBranch {
		t.Errorf("Branch = %q, want %q", wt.Branch, expectedBranch)
	}

	if _, err := os.Stat(wt.Path()); os.IsNotExist(err) {
		t.Fatal("worktree directory was not created")
	}

	out, err := exec.Command("git", "-C", wt.Path(), "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse failed: %v\noutput: %s", err, string(out))
	}
	gotBranch := strings.TrimSpace(string(out))
	if gotBranch != expectedBranch {
		t.Errorf("on branch %q, want %q", gotBranch, expectedBranch)
	}

	if err := wt.Remove(); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, err := os.Stat(wt.Path()); !os.IsNotExist(err) {
		t.Fatal("worktree directory still exists after Remove")
	}
}

func TestCreatePathAlreadyExists(t *testing.T) {
	repoDir := setupRepo(t)

	origDir, _ := os.Getwd() //nolint:errcheck
	os.Chdir(repoDir)        //nolint:errcheck
	defer os.Chdir(origDir)   //nolint:errcheck

	basePath := filepath.Join(repoDir, "worktrees")
	targetPath := filepath.Join(basePath, "run-existing", "unit-existing")
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	_, err := Create(basePath, "run-existing", "unit-existing")
	if err == nil {
		t.Fatal("expected error for existing path, got nil")
	}
}

func TestSanitizeBranchSegment(t *testing.T) {
	repoDir := setupRepo(t)

	origDir, _ := os.Getwd() //nolint:errcheck
	os.Chdir(repoDir)        //nolint:errcheck
	defer os.Chdir(origDir)   //nolint:errcheck

	wt, err := Create(repoDir, "run/a/b", "unit/c")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer wt.Remove() //nolint:errcheck

	expectedBranch := "baton/run-a-b/unit-c"
	if wt.Branch != expectedBranch {
		t.Errorf("Branch = %q, want %q", wt.Branch, expectedBranch)
	}

	out, err := exec.Command("git", "branch", "--list", expectedBranch).CombinedOutput()
	if err != nil {
		t.Fatalf("git branch --list failed: %v", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		t.Errorf("branch %q was not created", expectedBranch)
	}
}
