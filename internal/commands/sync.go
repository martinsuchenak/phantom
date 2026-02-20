package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewSyncCommand creates the sync command
func NewSyncCommand() *cli.Command {
	return &cli.Command{
		Name:  "sync",
		Usage: "Sync overlay with latest base directory changes",
		Description: "Pulls latest changes from the base directory into a running overlay. " +
			"For git repos, this rebases the overlay branch onto the updated base branch. " +
			"For non-git dirs, the overlay is remounted to pick up base changes.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would happen without making changes",
			},
			&cli.BoolFlag{
				Name:  "stash",
				Usage: "Stash uncommitted overlay changes before sync, restore after",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay to sync",
				Required: true,
			},
		},
		Run: doSync,
	}
}

func doSync(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	dryRun := cmd.GetBool("dry-run")
	doStash := cmd.GetBool("stash")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return err
	}
	if !mounted {
		return fmt.Errorf("overlay %q is not mounted — start or restart it first", name)
	}

	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)

	if isGit {
		return syncGit(ctx, ovl, mgr, gitOps, dryRun, doStash)
	}
	return syncNonGit(ctx, ovl, mgr, store, dryRun)
}

func syncGit(ctx context.Context, ovl *api.Overlay, mgr overlayManager, gitOps *git.Operations, dryRun, doStash bool) error {
	// Get current branch info
	baseBranch, err := gitOps.GetCurrentBranch(ctx, ovl.BaseDir)
	if err != nil {
		return fmt.Errorf("failed to get base branch: %w", err)
	}

	overlayBranch, err := gitOps.GetCurrentBranch(ctx, ovl.MountPoint)
	if err != nil {
		return fmt.Errorf("failed to get overlay branch: %w", err)
	}

	// Fetch latest from remote in base
	log.Info("Fetching latest changes in base directory...")
	if err := gitOps.Fetch(ctx, ovl.BaseDir); err != nil {
		log.Warn("Fetch failed (may be offline): %v", err)
	}

	// Check for uncommitted changes in overlay
	hasChanges, _ := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)

	if dryRun {
		fmt.Printf("Sync plan for %q:\n", ovl.Name)
		fmt.Printf("  Base branch:    %s\n", baseBranch)
		fmt.Printf("  Overlay branch: %s\n", overlayBranch)
		fmt.Printf("  Uncommitted:    %v\n", hasChanges)
		fmt.Printf("  Action:         rebase %s onto %s\n", overlayBranch, baseBranch)
		if hasChanges && doStash {
			fmt.Printf("  Stash:          yes (will stash and restore)\n")
		}
		return nil
	}

	// Stash if needed
	if hasChanges && doStash {
		log.Info("Stashing uncommitted changes...")
		if err := gitOps.Stash(ctx, ovl.MountPoint, "phantom-sync-"+ovl.Name); err != nil {
			return fmt.Errorf("failed to stash changes: %w", err)
		}
	} else if hasChanges {
		// Auto-commit to avoid losing work during rebase
		log.Info("Auto-committing overlay changes before sync...")
		if err := gitOps.CommitAll(ctx, ovl.MountPoint, "phantom: auto-commit before sync"); err != nil {
			return fmt.Errorf("failed to auto-commit: %w", err)
		}
	}

	// Rebase overlay branch onto base branch
	log.Info("Rebasing %s onto %s...", overlayBranch, baseBranch)
	if err := rebaseOnto(ctx, gitOps, ovl.MountPoint, baseBranch); err != nil {
		return fmt.Errorf("rebase failed: %w — you may need to resolve conflicts manually in %s", err, ovl.MountPoint)
	}

	// Restore stash if we stashed
	if hasChanges && doStash {
		log.Info("Restoring stashed changes...")
		if err := gitOps.StashPop(ctx, ovl.MountPoint); err != nil {
			log.Warn("Failed to restore stash: %v — run 'git stash pop' manually in %s", err, ovl.MountPoint)
		}
	}

	log.Info("Overlay %q synced with base (%s)", ovl.Name, baseBranch)
	return nil
}

func syncNonGit(ctx context.Context, ovl *api.Overlay, mgr overlayManager, store *state.Store, dryRun bool) error {
	if dryRun {
		fmt.Printf("Sync plan for %q (non-git):\n", ovl.Name)
		fmt.Printf("  Action: remount overlay to pick up base directory changes\n")
		fmt.Printf("  Note:   overlay writes are preserved, base reads are refreshed\n")
		return nil
	}

	// For non-git overlays, remounting refreshes the lower (base) layer
	log.Info("Remounting overlay to refresh base layer...")
	if err := mgr.Unmount(ovl); err != nil {
		return fmt.Errorf("failed to unmount: %w", err)
	}
	if err := mgr.Mount(ovl); err != nil {
		return fmt.Errorf("failed to remount: %w", err)
	}

	log.Info("Overlay %q synced (remounted)", ovl.Name)
	return nil
}

// rebaseOnto performs a git rebase of the current branch onto the target branch
func rebaseOnto(ctx context.Context, gitOps *git.Operations, repoPath, targetBranch string) error {
	return gitOps.Rebase(ctx, repoPath, targetBranch)
}
