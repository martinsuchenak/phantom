package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/martinsuchenak/phantom/internal/agent"
	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewRunCommand creates the run command
func NewRunCommand() *cli.Command {
	return &cli.Command{
		Name:        "run",
		Usage:       "Run an agent command in an overlay context",
		Description: "Creates an overlay (if needed) and runs the specified agent command within it, setting appropriate environment variables.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "agent",
				Aliases:  []string{"a"},
				Usage:    "Agent command to run",
				EnvVars:  []string{"OVERLAY_AGENT"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "task",
				Aliases:  []string{"t"},
				Usage:    "Task description",
				EnvVars:  []string{"OVERLAY_TASK"},
				Required: true,
			},
			&cli.StringFlag{
				Name:     "base",
				Aliases:  []string{"b"},
				Usage:    "Base directory for the overlay",
				EnvVars:  []string{"OVERLAY_BASE"},
				Required: true,
			},
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Overlay name (auto-generated if not specified)",
				EnvVars: []string{"OVERLAY_NAME"},
			},
			&cli.StringFlag{
				Name:    "branch",
				Usage:   "Git branch name",
				EnvVars: []string{"OVERLAY_BRANCH"},
			},
			&cli.IntFlag{
				Name:         "timeout",
				Usage:        "Timeout for the agent command in minutes (max 1440)",
				DefaultValue: 0, // 0 means use config default
			},
			&cli.BoolFlag{
				Name:  "cleanup",
				Usage: "Cleanup overlay after completion",
			},
			&cli.BoolFlag{
				Name:  "push",
				Usage: "Push branch to remote on completion",
			},
			&cli.BoolFlag{
				Name:    "persist",
				Aliases: []string{"p"},
				Usage:   "Keep overlay mounted after completion",
			},
		},
		Run: doRun,
	}
}

func doRun(ctx context.Context, cmd *cli.Command) error {
	agentCmd := cmd.GetString("agent")
	task := cmd.GetString("task")
	baseDir := cmd.GetString("base")
	name := cmd.GetString("name")
	branch := cmd.GetString("branch")
	timeoutMinutes := cmd.GetInt("timeout")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	persist := cmd.GetBool("persist")

	exitCode, err := processRun(ctx, agentCmd, task, baseDir, name, branch, timeoutMinutes, doCleanup, doPush, persist)
	if err != nil {
		return err
	}

	// Exit with the same code as the agent
	if exitCode != 0 {
		os.Exit(exitCode)
	}

	return nil
}

func processRun(ctx context.Context, agentCmd, task, baseDir, name, branch string, timeoutMinutes int, doCleanup, doPush, persist bool) (int, error) {
	// Determine timeout: use flag if provided, otherwise use config default
	var timeout time.Duration
	if timeoutMinutes > 0 {
		// Validate timeout bounds
		if timeoutMinutes > config.MaxTimeoutMinutes {
			return 0, fmt.Errorf("timeout cannot exceed %d minutes (24 hours)", config.MaxTimeoutMinutes)
		}
		timeout = time.Duration(timeoutMinutes) * time.Minute
	} else if cfg != nil && cfg.Agent.DefaultTimeoutMinutes > 0 {
		timeout = time.Duration(cfg.Agent.DefaultTimeoutMinutes) * time.Minute
	} else {
		timeout = 60 * time.Minute // fallback default
	}

	// Get absolute path for base directory
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve base directory path: %w", err)
	}

	// Generate name if not provided
	if name == "" {
		name = fmt.Sprintf("agent-%d", time.Now().Unix())
	}

	// Validate branch name if provided
	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			return 0, err
		}
	}

	log.Debug("Running agent in overlay %q for %q", name, absBaseDir)
	log.Debug("Agent command: %s", agentCmd)
	log.Debug("Task: %s", task)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return 0, fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Create overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return 0, err
	}

	// Check if overlay already exists
	var ovl *api.Overlay
	if store.Exists(name) {
		ovl, err = store.Load(name)
		if err != nil {
			return 0, err
		}
		log.Debug("Using existing overlay")
	} else {
		// Initialize git operations
		gitOps := git.NewOperations()

		// Check if base directory is a git repo
		isGit, _ := gitOps.IsGitRepo(ctx, absBaseDir)

		// Generate branch name if not provided
		if branch == "" && isGit && cfg.Git.AutoBranch {
			branch = cfg.Git.BranchPrefix + name
		}

		// Create overlay options
		opts := &api.CreateOptions{
			Name:       name,
			BaseDir:    absBaseDir,
			Branch:     branch,
			Persistent: persist,
		}

		// Create the overlay
		ovl, err = mgr.Create(opts)
		if err != nil {
			return 0, err
		}

		// Handle git branch creation if applicable
		if isGit && branch != "" {
			branchExists, _ := gitOps.BranchExists(ctx, absBaseDir, branch)
			if !branchExists {
				if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branch, ""); err != nil {
					log.Warn("Failed to create branch %s: %v", branch, err)
				}
			} else {
				if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branch); err != nil {
					log.Warn("Failed to switch to branch %s: %v", branch, err)
				}
			}
		}

		// Save state
		if err := store.Save(ovl); err != nil {
			mgr.Cleanup(ovl)
			return 0, fmt.Errorf("failed to save overlay state: %w", err)
		}
	}

	// Check if mounted
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return 0, err
	}
	if !mounted {
		return 0, api.NewError(api.ErrOverlayNotMounted, "overlay is not mounted", nil)
	}

	// Create agent runner
	runner := agent.NewRunner(cfg, log)

	// Run options
	runOpts := &api.RunOptions{
		Agent:     agentCmd,
		Task:      task,
		BaseDir:   absBaseDir,
		Name:      name,
		Timeout:   timeout,
		Cleanup:   doCleanup,
		PushOnEnd: doPush,
	}

	// Run the agent
	exitCode, err := runner.Run(ctx, ovl, runOpts)
	if err != nil {
		log.Error("Agent execution failed: %v", err)
	}

	// Cleanup if requested
	if doCleanup {
		log.Debug("Cleaning up overlay")
		if err := mgr.Cleanup(ovl); err != nil {
			log.Error("Failed to cleanup overlay: %v", err)
		}
		store.Delete(name)
	}

	return exitCode, err
}
