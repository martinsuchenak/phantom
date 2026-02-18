package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// skipIfNoGit skips the test if git is not available
func skipIfNoGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found, skipping test")
	}
}

// createTestRepo creates a temporary git repository for testing
func createTestRepo(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "phantom-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	cmd.Run()

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create initial commit: %v", err)
	}

	return tmpDir
}

func TestIsGitRepo(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	t.Run("valid git repo", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer os.RemoveAll(repoPath)

		isGit, err := gitOps.IsGitRepo(ctx, repoPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !isGit {
			t.Error("expected directory to be a git repo")
		}
	})

	t.Run("non-git directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "phantom-non-git-*")
		if err != nil {
			t.Fatalf("failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		isGit, err := gitOps.IsGitRepo(ctx, tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isGit {
			t.Error("expected directory to not be a git repo")
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		isGit, err := gitOps.IsGitRepo(ctx, "/non/existent/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if isGit {
			t.Error("expected non-existent directory to not be a git repo")
		}
	})
}

func TestGetCurrentBranch(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	branch, err := gitOps.GetCurrentBranch(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Default branch could be main or master
	if branch != "main" && branch != "master" {
		t.Errorf("expected branch to be 'main' or 'master', got %s", branch)
	}
}

func TestCreateAndSwitchBranch(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a new branch
	newBranch := "test-feature"
	if err := gitOps.CreateBranch(ctx, repoPath, newBranch, ""); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Verify we're on the new branch
	currentBranch, err := gitOps.GetCurrentBranch(ctx, repoPath)
	if err != nil {
		t.Fatalf("failed to get current branch: %v", err)
	}
	if currentBranch != newBranch {
		t.Errorf("expected branch %s, got %s", newBranch, currentBranch)
	}

	// Switch back to original branch
	if err := gitOps.SwitchBranch(ctx, repoPath, "main"); err != nil {
		// Try master if main fails
		if err := gitOps.SwitchBranch(ctx, repoPath, "master"); err != nil {
			t.Fatalf("failed to switch branch: %v", err)
		}
	}
}

func TestBranchExists(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a branch
	if err := gitOps.CreateBranch(ctx, repoPath, "existing-branch", ""); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	// Check existing branch
	exists, err := gitOps.BranchExists(ctx, repoPath, "existing-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected branch to exist")
	}

	// Check non-existing branch
	exists, err = gitOps.BranchExists(ctx, repoPath, "non-existing-branch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("expected branch to not exist")
	}
}

func TestHasUncommittedChanges(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// No changes initially
	hasChanges, err := gitOps.HasUncommittedChanges(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes initially")
	}

	// Create a change
	testFile := filepath.Join(repoPath, "new-file.txt")
	if err := os.WriteFile(testFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Should have changes now
	hasChanges, err = gitOps.HasUncommittedChanges(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasChanges {
		t.Error("expected uncommitted changes after creating file")
	}
}

func TestCommitAll(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Create a file
	testFile := filepath.Join(repoPath, "commit-test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// Commit
	if err := gitOps.CommitAll(ctx, repoPath, "test commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	// Should have no uncommitted changes
	hasChanges, err := gitOps.HasUncommittedChanges(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChanges {
		t.Error("expected no uncommitted changes after commit")
	}
}

func TestGetCommitHash(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	hash, err := gitOps.GetCommitHash(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hash) < 7 {
		t.Errorf("expected hash to be at least 7 characters, got %s", hash)
	}
}

func TestGetStatus(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	// Get status when clean
	status, err := gitOps.GetStatus(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "" {
		t.Errorf("expected empty status for clean repo, got %s", status)
	}

	// Create a file
	testFile := filepath.Join(repoPath, "status-test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Get status when dirty
	status, err = gitOps.GetStatus(ctx, repoPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status == "" {
		t.Error("expected non-empty status for dirty repo")
	}
}

func TestGetLog(t *testing.T) {
	skipIfNoGit(t)

	gitOps := NewOperations()
	ctx := context.Background()

	repoPath := createTestRepo(t)
	defer os.RemoveAll(repoPath)

	log, err := gitOps.GetLog(ctx, repoPath, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if log == "" {
		t.Error("expected non-empty log")
	}
}

func TestWithTimeout(t *testing.T) {
	gitOps := NewOperations().WithTimeout(5 * time.Second)

	if gitOps == nil {
		t.Fatal("expected gitOps to not be nil")
	}
}
