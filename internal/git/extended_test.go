package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitExtended(t *testing.T) {
	skipIfNoGit(t)
	gitOps := NewOperations()
	ctx := context.Background()

	t.Run("stash operations", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer func() { _ = os.RemoveAll(repoPath) }()

		// Create a file and commit it first so it's tracked
		testFile := filepath.Join(repoPath, "stash-test.txt")
		_ = os.WriteFile(testFile, []byte("initial"), 0644)
		_ = exec.Command("git", "-C", repoPath, "add", ".").Run()
		_ = exec.Command("git", "-C", repoPath, "commit", "-m", "add stash-test.txt").Run()

		// Modify it to create a dirty state
		_ = os.WriteFile(testFile, []byte("dirty"), 0644)

		if err := gitOps.Stash(ctx, repoPath, "saving work"); err != nil {
			t.Fatalf("failed to stash: %v", err)
		}

		// Check if clean (should revert to "initial")
		content, _ := os.ReadFile(testFile)
		if string(content) != "initial" {
			t.Error("expected clean repo (content='initial') after stash")
		}

		if err := gitOps.StashPop(ctx, repoPath); err != nil {
			t.Fatalf("failed to pop stash: %v", err)
		}

		// Check if dirty again
		content, _ = os.ReadFile(testFile)
		if string(content) != "dirty" {
			t.Error("expected dirty repo (content='dirty') after pop")
		}
	})

	t.Run("fetch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer func() { _ = os.RemoveAll(repoPath) }()

		// Just run fetch, it might fail if no remote but at least covers the code path
		// To test properly we'd need a remote.
		// Let's create a bare repo as remote
		remoteDir, _ := os.MkdirTemp("", "git-remote-*")
		defer func() { _ = os.RemoveAll(remoteDir) }()
		_ = exec.Command("git", "init", "--bare", remoteDir).Run()

		// Add remote
		_ = exec.Command("git", "-C", repoPath, "remote", "add", "origin", remoteDir).Run()

		if err := gitOps.Fetch(ctx, repoPath); err != nil {
			t.Errorf("fetch failed: %v", err)
		}
	})

	t.Run("push branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer func() { _ = os.RemoveAll(repoPath) }()

		remoteDir, _ := os.MkdirTemp("", "git-remote-*")
		defer func() { _ = os.RemoveAll(remoteDir) }()
		_ = exec.Command("git", "init", "--bare", remoteDir).Run()

		_ = exec.Command("git", "-C", repoPath, "remote", "add", "origin", remoteDir).Run()

		// Get current branch name
		out, _ := exec.Command("git", "-C", repoPath, "branch", "--show-current").Output()
		currentBranch := string(out)
		// trim newline
		if len(currentBranch) > 0 {
			currentBranch = currentBranch[:len(currentBranch)-1]
		}

		if err := gitOps.PushBranch(ctx, repoPath, currentBranch, false); err != nil {
			t.Errorf("push failed: %v", err)
		}
	})

	t.Run("delete branch", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer func() { _ = os.RemoveAll(repoPath) }()

		// Get default branch
		out, _ := exec.Command("git", "-C", repoPath, "branch", "--show-current").Output()
		defaultBranch := string(out)
		if len(defaultBranch) > 0 && defaultBranch[len(defaultBranch)-1] == '\n' {
			defaultBranch = defaultBranch[:len(defaultBranch)-1]
		}

		_ = gitOps.CreateBranch(ctx, repoPath, "to-delete", "")

		// Switch back to default branch
		if err := gitOps.SwitchBranch(ctx, repoPath, defaultBranch); err != nil {
			t.Fatalf("failed to switch back to %s: %v", defaultBranch, err)
		}

		if err := gitOps.DeleteBranch(ctx, repoPath, "to-delete", true); err != nil {
			t.Errorf("delete branch failed: %v", err)
		}
	})

	t.Run("get remote url", func(t *testing.T) {
		repoPath := createTestRepo(t)
		defer func() { _ = os.RemoveAll(repoPath) }()

		// Use example.com instead of github.com to avoid local git config rewrites (ssh vs https)
		targetUrl := "https://example.com/repo.git"
		_ = exec.Command("git", "-C", repoPath, "remote", "add", "origin", targetUrl).Run()

		url, err := gitOps.GetRemoteURL(ctx, repoPath)
		if err != nil {
			t.Fatalf("failed to get remote url: %v", err)
		}
		if url != targetUrl {
			t.Errorf("expected url %s, got %s", targetUrl, url)
		}
	})
}
