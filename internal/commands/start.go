package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewStartCommand creates the start command
func NewStartCommand() *cli.Command {
	return &cli.Command{
		Name:        "start",
		Usage:       "Create and mount a new overlay filesystem",
		Description: "Creates a new overlay filesystem for the specified base directory and prints the mount point path.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Name for the overlay",
				EnvVars: []string{"OVERLAY_NAME"},
			},
			&cli.StringFlag{
				Name:    "branch",
				Aliases: []string{"b"},
				Usage:   "Git branch name (default: overlay/<name>)",
				EnvVars: []string{"OVERLAY_BRANCH"},
			},
			&cli.BoolFlag{
				Name:    "persistent",
				Aliases: []string{"p"},
				Usage:   "Keep overlay data across reboots",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "base-dir",
				Usage:    "Base directory to overlay",
				Required: true,
			},
		},
		MinArgs: 1,
		MaxArgs: 1,
		Run:     doStart,
	}
}

func doStart(ctx context.Context, cmd *cli.Command) error {
	args := cmd.GetArgs()
	baseDir := args[0]
	name := cmd.GetString("name")
	branch := cmd.GetString("branch")
	persistent := cmd.GetBool("persistent")

	return processStart(ctx, baseDir, name, branch, persistent)
}

func processStart(ctx context.Context, baseDir, name, branch string, persistent bool) error {
	// Get absolute path for base directory
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory path: %w", err)
	}

	// Generate name if not provided
	if name == "" {
		// Use the base directory name
		name = filepath.Base(absBaseDir)
		if name == "." || name == "/" {
			return fmt.Errorf("could not generate overlay name, please specify with -n")
		}
	}

	// Validate name format to prevent path traversal and ensure safety
	// Only allow alphanumeric characters, hyphens, and underscores
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid overlay name %q: must contain only alphanumeric characters, hyphens, and underscores", name)
	}

	log.Debug("Creating overlay %q for %q", name, absBaseDir)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Check if overlay already exists
	if store.Exists(name) {
		return api.NewError(api.ErrAlreadyExists, fmt.Sprintf("overlay %q already exists", name), nil)
	}

	// Initialize git operations
	gitOps := git.NewOperations()

	// Check if base directory is a git repo
	isGit, err := gitOps.IsGitRepo(ctx, absBaseDir)
	if err != nil {
		log.Debug("Git check failed: %v", err)
	}

	// Generate branch name if not provided
	if branch == "" && isGit && cfg.Git.AutoBranch {
		branch = cfg.Git.BranchPrefix + name
	}

	// Create the overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	// Create overlay options
	opts := &api.CreateOptions{
		Name:       name,
		BaseDir:    absBaseDir,
		Branch:     branch,
		Persistent: persistent,
	}

	// Create the overlay
	ovl, err := mgr.Create(opts)
	if err != nil {
		return err
	}

	// Handle git branch creation if applicable
	if isGit && branch != "" {
		log.Debug("Creating git branch %q", branch)

		// Check for uncommitted changes
		hasChanges, err := gitOps.HasUncommittedChanges(ctx, absBaseDir)
		if err != nil {
			log.Debug("Failed to check for uncommitted changes: %v", err)
		}

		if hasChanges {
			// Stash changes before creating branch
			if err := gitOps.Stash(ctx, absBaseDir, "overlay-auto-stash-"+name); err != nil {
				log.Debug("Failed to stash changes: %v", err)
			}
		}

		// Check if branch already exists
		branchExists, _ := gitOps.BranchExists(ctx, absBaseDir, branch)
		if branchExists {
			// Switch to existing branch
			if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branch); err != nil {
				log.Debug("Failed to switch to branch: %v", err)
			}
		} else {
			// Create new branch in the overlay mount
			if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branch, ""); err != nil {
				log.Debug("Failed to create branch: %v", err)
			}
		}

		if hasChanges {
			// Pop stash in the overlay
			if err := gitOps.StashPop(ctx, ovl.MountPoint); err != nil {
				log.Debug("Failed to pop stash: %v", err)
			}
		}
	}

	// Save state
	if err := store.Save(ovl); err != nil {
		// Try to cleanup on failure
		mgr.Cleanup(ovl)
		return fmt.Errorf("failed to save overlay state: %w", err)
	}

	// Output the mount point path (this is what scripts can capture)
	log.Info(ovl.MountPoint)

	return nil
}
