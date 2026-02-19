package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewCommitCommand creates the commit command
func NewCommitCommand() *cli.Command {
	return &cli.Command{
		Name:        "commit",
		Usage:       "Commit changes in an overlay",
		Description: "Stages all changes in the overlay's mount point and creates a git commit. Optionally pushes to remote.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "message",
				Aliases:  []string{"m"},
				Usage:    "Commit message",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "push",
				Usage: "Push to remote after committing",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay",
				Required: true,
			},
		},
		Run: doCommit,
	}
}

func doCommit(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}
	message := cmd.GetString("message")
	if message == "" {
		return fmt.Errorf("commit message is required")
	}
	push := cmd.GetBool("push")

	return processCommit(ctx, name, message, push)
}

func processCommit(ctx context.Context, name, message string, push bool) error {
	log.Debug("Committing changes in overlay %q", name)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	// Verify overlay is mounted
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return fmt.Errorf("failed to check mount status: %w", err)
	}
	if !mounted {
		return fmt.Errorf("overlay %q is not mounted", name)
	}

	gitOps := git.NewOperations()

	// Verify it's a git repo
	isGit, err := gitOps.IsGitRepo(ctx, ovl.MountPoint)
	if err != nil || !isGit {
		return fmt.Errorf("overlay %q is not a git repository", name)
	}

	// Check for changes
	hasChanges, err := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
	if err != nil {
		return fmt.Errorf("failed to check for changes: %w", err)
	}
	if !hasChanges {
		log.Info("No changes to commit in overlay %q", name)
		return nil
	}

	// Commit
	if err := gitOps.CommitAll(ctx, ovl.MountPoint, message); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	branch, _ := gitOps.GetCurrentBranch(ctx, ovl.MountPoint)
	log.Info("Committed changes on branch %q", branch)

	// Push if requested
	if push {
		if branch == "" {
			return fmt.Errorf("cannot push: unable to determine current branch")
		}
		if err := gitOps.PushBranch(ctx, ovl.MountPoint, branch, false); err != nil {
			return fmt.Errorf("failed to push: %w", err)
		}
		log.Info("Pushed branch %q to remote", branch)
	}

	return nil
}
