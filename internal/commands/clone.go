package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewCloneCommand creates the clone command
func NewCloneCommand() *cli.Command {
	return &cli.Command{
		Name:  "clone",
		Usage: "Clone an overlay to a new name",
		Description: "Duplicates an overlay's changes to a new overlay with its own mount and branch. " +
			"Useful for branching experiments from a known state.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "branch",
				Aliases: []string{"b"},
				Usage:   "Git branch for the clone (default: phantom/<new-name>)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "source", Usage: "Source overlay name", Required: true},
			&cli.StringArg{Name: "target", Usage: "New overlay name", Required: true},
		},
		Run: doClone,
	}
}

func doClone(ctx context.Context, cmd *cli.Command) error {
	sourceName := cmd.GetStringArg("source")
	targetName := cmd.GetStringArg("target")
	branch := cmd.GetString("branch")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Load source overlay
	srcOvl, err := store.Load(sourceName)
	if err != nil {
		return err
	}

	// Check target doesn't exist
	if store.Exists(targetName) {
		return api.NewError(api.ErrAlreadyExists, fmt.Sprintf("overlay %q already exists", targetName), nil)
	}

	// Create new overlay from same base
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, srcOvl.BaseDir)

	if branch == "" && isGit && cfg.Git.AutoBranch {
		branch = cfg.Git.BranchPrefix + targetName
	}

	opts := &api.CreateOptions{
		Name:    targetName,
		BaseDir: srcOvl.BaseDir,
		Branch:  branch,
	}

	newOvl, err := mgr.Create(opts)
	if err != nil {
		return fmt.Errorf("failed to create target overlay: %w", err)
	}

	// Copy source upper dir contents to new upper dir
	if err := copyDir(srcOvl.UpperDir, newOvl.UpperDir); err != nil {
		_ = mgr.Cleanup(newOvl)
		return fmt.Errorf("failed to copy overlay data: %w", err)
	}

	// Handle git branch
	if isGit && branch != "" {
		branchExists, _ := gitOps.BranchExists(ctx, srcOvl.BaseDir, branch)
		if !branchExists {
			if err := gitOps.CreateBranch(ctx, newOvl.MountPoint, branch, ""); err != nil {
				log.Warn("Failed to create branch %s: %v", branch, err)
			}
		} else {
			if err := gitOps.SwitchBranch(ctx, newOvl.MountPoint, branch); err != nil {
				log.Warn("Failed to switch to branch %s: %v", branch, err)
			}
		}
	}

	if err := store.Save(newOvl); err != nil {
		_ = mgr.Cleanup(newOvl)
		return fmt.Errorf("failed to save state: %w", err)
	}

	log.Info("Cloned %q -> %q", sourceName, targetName)
	log.Info(newOvl.MountPoint)
	return nil
}
