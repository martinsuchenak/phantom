package commands

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

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
				Usage:   "Git branch name (default: phantom/<name>)",
				EnvVars: []string{"OVERLAY_BRANCH"},
			},
			&cli.BoolFlag{
				Name:    "persistent",
				Aliases: []string{"p"},
				Usage:   "Keep overlay data across reboots",
			},
			&cli.BoolFlag{
				Name:    "no-stash",
				Usage:   "Fail if there are uncommitted changes instead of auto-stashing",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "base-dir",
				Usage:    "Base directory to overlay",
				Required: true,
			},
		},
		Run: doStart,
	}
}

func doStart(ctx context.Context, cmd *cli.Command) error {
	baseDir := resolveBaseDir(cmd.GetStringArg("base-dir"))
	if baseDir == "" {
		return fmt.Errorf("base directory is required")
	}
	name := cmd.GetString("name")
	branch := cmd.GetString("branch")
	persistent := cmd.GetBool("persistent")
	noStash := cmd.GetBool("no-stash")

	return processStart(ctx, baseDir, name, branch, persistent, noStash)
}

func processStart(ctx context.Context, baseDir, name, branch string, persistent bool, noStash bool) error {
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

	// Validate branch name if provided (prevent git injection)
	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			return err
		}
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

	// Track if we need to rollback
	var gitErrors []string
	stashApplied := false

	// Handle git branch creation if applicable
	if isGit && branch != "" {
		log.Debug("Creating git branch %q", branch)

		// Check for uncommitted changes
		hasChanges, err := gitOps.HasUncommittedChanges(ctx, absBaseDir)
		if err != nil {
			gitErrors = append(gitErrors, fmt.Sprintf("failed to check uncommitted changes: %v", err))
		}

		if hasChanges {
			if noStash {
				// User explicitly requested no auto-stash, fail with helpful message
				return fmt.Errorf("uncommitted changes detected in %s: please commit or stash manually, or remove --no-stash flag to allow auto-stashing", absBaseDir)
			}

			// Warn user about auto-stashing
			log.Warn("Uncommitted changes detected, auto-stashing (stash name: overlay-auto-stash-%s)", name)

			// Stash changes before creating branch
			if err := gitOps.Stash(ctx, absBaseDir, "overlay-auto-stash-"+name); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to stash changes: %v", err))
			} else {
				stashApplied = true
			}
		}

		// Check if branch already exists
		branchExists, _ := gitOps.BranchExists(ctx, absBaseDir, branch)
		if branchExists {
			// Switch to existing branch
			if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branch); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to switch to branch %s: %v", branch, err))
			}
		} else {
			// Create new branch in the overlay mount
			if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branch, ""); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to create branch %s: %v", branch, err))
			}
		}

		if stashApplied {
			// Pop stash in the overlay
			if err := gitOps.StashPop(ctx, ovl.MountPoint); err != nil {
				gitErrors = append(gitErrors, fmt.Sprintf("failed to pop stash: %v", err))
				log.Warn("Stashed changes remain in base repo, run 'git stash pop' manually if needed")
			}
		}
	}

	// Report git errors at warn level so users see them
	for _, gitErr := range gitErrors {
		log.Warn("Git: %s", gitErr)
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

// validateBranchName checks if a branch name is safe to use with git
func validateBranchName(branch string) error {
	// Reject branch names that could be interpreted as git options
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("invalid branch name %q: cannot start with '-'", branch)
	}

	// Reject branch names with potentially dangerous characters
	invalidChars := []string{"..", "~", "^", ":", "?", "*", "[", "\\", " ", "\t", "\n"}
	for _, char := range invalidChars {
		if strings.Contains(branch, char) {
			return fmt.Errorf("invalid branch name %q: contains invalid character %q", branch, char)
		}
	}

	// Reject empty or whitespace-only names
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch name cannot be empty")
	}

	// Reject names ending with .lock
	if strings.HasSuffix(branch, ".lock") {
		return fmt.Errorf("invalid branch name %q: cannot end with '.lock'", branch)
	}

	return nil
}
