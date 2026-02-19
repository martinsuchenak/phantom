package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

// Operations provides git operations for overlay management
type Operations struct {
	timeout time.Duration
}

// NewOperations creates a new git operations handler
func NewOperations() *Operations {
	return &Operations{
		timeout: 30 * time.Second,
	}
}

// WithTimeout sets a custom timeout for git operations
func (g *Operations) WithTimeout(timeout time.Duration) *Operations {
	g.timeout = timeout
	return g
}

// runGit executes a git command with context
func (g *Operations) runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// IsGitRepo checks if a directory is a git repository
func (g *Operations) IsGitRepo(ctx context.Context, repoPath string) (bool, error) {
	_, err := g.runGit(ctx, repoPath, "rev-parse", "--git-dir")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// GetCurrentBranch returns the current branch name
func (g *Operations) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return g.runGit(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates a new branch from the current HEAD or specified base
func (g *Operations) CreateBranch(ctx context.Context, repoPath, branchName string, baseBranch string) error {
	args := []string{"checkout", "-b", branchName}
	if baseBranch != "" {
		args = append(args, baseBranch)
	}

	_, err := g.runGit(ctx, repoPath, args...)
	if err != nil {
		return api.NewError(api.ErrGitFailed, fmt.Sprintf("failed to create branch %s", branchName), err)
	}
	return nil
}

// SwitchBranch switches to an existing branch
func (g *Operations) SwitchBranch(ctx context.Context, repoPath, branchName string) error {
	_, err := g.runGit(ctx, repoPath, "checkout", branchName)
	if err != nil {
		return api.NewError(api.ErrGitFailed, fmt.Sprintf("failed to switch to branch %s", branchName), err)
	}
	return nil
}

// DeleteBranch deletes a branch
func (g *Operations) DeleteBranch(ctx context.Context, repoPath, branchName string, force bool) error {
	args := []string{"branch"}
	if force {
		args = append(args, "-D")
	} else {
		args = append(args, "-d")
	}
	args = append(args, branchName)

	_, err := g.runGit(ctx, repoPath, args...)
	if err != nil {
		return api.NewError(api.ErrGitFailed, fmt.Sprintf("failed to delete branch %s", branchName), err)
	}
	return nil
}

// HasUncommittedChanges checks if there are uncommitted changes
func (g *Operations) HasUncommittedChanges(ctx context.Context, repoPath string) (bool, error) {
	output, err := g.runGit(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(output) != "", nil
}

// PushBranch pushes a branch to remote
func (g *Operations) PushBranch(ctx context.Context, repoPath, branchName string, force bool) error {
	args := []string{"push", "-u", "origin", branchName}
	if force {
		args = []string{"push", "-u", "--force", "origin", branchName}
	}

	_, err := g.runGit(ctx, repoPath, args...)
	if err != nil {
		return api.NewError(api.ErrGitFailed, fmt.Sprintf("failed to push branch %s", branchName), err)
	}
	return nil
}

// CommitAll stages all changes and creates a commit
func (g *Operations) CommitAll(ctx context.Context, repoPath, message string) error {
	// Stage all changes
	_, err := g.runGit(ctx, repoPath, "add", "-A")
	if err != nil {
		return api.NewError(api.ErrGitFailed, "failed to stage changes", err)
	}

	// Commit
	_, err = g.runGit(ctx, repoPath, "commit", "-m", message)
	if err != nil {
		return api.NewError(api.ErrGitFailed, "failed to commit", err)
	}
	return nil
}

// GetRemoteURL returns the remote URL for the repository
func (g *Operations) GetRemoteURL(ctx context.Context, repoPath string) (string, error) {
	return g.runGit(ctx, repoPath, "remote", "get-url", "origin")
}

// BranchExists checks if a branch exists
func (g *Operations) BranchExists(ctx context.Context, repoPath, branchName string) (bool, error) {
	_, err := g.runGit(ctx, repoPath, "rev-parse", "--verify", branchName)
	return err == nil, nil
}

// GetCommitHash returns the current commit hash
func (g *Operations) GetCommitHash(ctx context.Context, repoPath string) (string, error) {
	return g.runGit(ctx, repoPath, "rev-parse", "HEAD")
}

// Stash saves uncommitted changes to stash
func (g *Operations) Stash(ctx context.Context, repoPath, message string) error {
	args := []string{"stash", "push"}
	if message != "" {
		args = append(args, "-m", message)
	}
	_, err := g.runGit(ctx, repoPath, args...)
	return err
}

// StashPop restores stashed changes
func (g *Operations) StashPop(ctx context.Context, repoPath string) error {
	_, err := g.runGit(ctx, repoPath, "stash", "pop")
	return err
}

// Fetch fetches from remote
func (g *Operations) Fetch(ctx context.Context, repoPath string) error {
	_, err := g.runGit(ctx, repoPath, "fetch", "--all")
	return err
}

// GetStatus returns the git status output
func (g *Operations) GetStatus(ctx context.Context, repoPath string) (string, error) {
	return g.runGit(ctx, repoPath, "status", "--short")
}

// GetLog returns recent commit log
func (g *Operations) GetLog(ctx context.Context, repoPath string, count int) (string, error) {
	return g.runGit(ctx, repoPath, "log", "--oneline", fmt.Sprintf("-%d", count))
}
// MergeBranch merges the specified branch into the current branch
func (g *Operations) MergeBranch(ctx context.Context, repoPath, branchName string) error {
	_, err := g.runGit(ctx, repoPath, "merge", branchName)
	if err != nil {
		return api.NewError(api.ErrGitFailed, fmt.Sprintf("failed to merge branch %s", branchName), err)
	}
	return nil
}
