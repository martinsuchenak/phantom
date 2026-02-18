package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/logger"
)

// Runner handles running agent commands in overlay contexts
type Runner struct {
	cfg    *config.Config
	gitOps *git.Operations
	log    logger.Logger
}

// NewRunner creates a new agent runner
func NewRunner(cfg *config.Config, log logger.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		gitOps: git.NewOperations(),
		log:    log,
	}
}

// Run executes an agent command in the overlay context
func (r *Runner) Run(ctx context.Context, ovl *api.Overlay, opts *api.RunOptions) (int, error) {
	// Create context with timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Log start
	r.log.Info("Starting agent: %s", opts.Agent)
	r.log.Info("Task: %s", opts.Task)
	r.log.Info("Overlay: %s", ovl.MountPoint)

	// Build the command
	cmd := exec.CommandContext(ctx, "sh", "-c", opts.Agent)
	cmd.Dir = ovl.MountPoint
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	// Set environment variables
	cmd.Env = r.buildEnv(ovl, opts)

	// Run the command
	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Log completion
	r.log.Info("Agent completed in %s with exit code %d", duration.Round(time.Second), exitCode)

	// Handle git operations on completion
	if ovl.Branch != "" && opts.PushOnEnd {
		if exitCode == 0 || r.cfg.Agent.CleanupOnFailure {
			r.handleGitOperations(ctx, ovl, exitCode == 0)
		}
	}

	return exitCode, err
}

// buildEnv builds the environment variables for the agent
func (r *Runner) buildEnv(ovl *api.Overlay, opts *api.RunOptions) []string {
	env := os.Environ()

	// Core overlay variables
	env = append(env,
		fmt.Sprintf("OVERLAY_NAME=%s", ovl.Name),
		fmt.Sprintf("OVERLAY_PATH=%s", ovl.MountPoint),
		fmt.Sprintf("OVERLAY_BASE=%s", ovl.BaseDir),
		fmt.Sprintf("OVERLAY_BRANCH=%s", ovl.Branch),
		fmt.Sprintf("OVERLAY_TASK=%s", opts.Task),
	)

	// Add configured agent env vars
	for _, e := range r.cfg.AgentEnv {
		env = append(env, e)
	}

	return env
}

// handleGitOperations handles git operations after agent completion
func (r *Runner) handleGitOperations(ctx context.Context, ovl *api.Overlay, success bool) {
	// Check for changes
	hasChanges, err := r.gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
	if err != nil {
		r.log.Debug("Failed to check for changes: %v", err)
		return
	}

	if !hasChanges {
		r.log.Debug("No changes to commit")
		return
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Agent changes: %s", ovl.Branch)
	if err := r.gitOps.CommitAll(ctx, ovl.MountPoint, commitMsg); err != nil {
		r.log.Debug("Failed to commit changes: %v", err)
		return
	}

	r.log.Debug("Committed agent changes")

	// Push if requested
	if r.cfg.Git.AutoPushOnStop {
		if err := r.gitOps.PushBranch(ctx, ovl.MountPoint, ovl.Branch, false); err != nil {
			r.log.Debug("Failed to push branch: %v", err)
		} else {
			r.log.Debug("Pushed branch to remote")
		}
	}
}
