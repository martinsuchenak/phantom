package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewApplyCommand creates the apply command
func NewApplyCommand() *cli.Command {
	return &cli.Command{
		Name:        "apply",
		Usage:       "Apply overlay changes back to the base directory",
		Description: "Merges the overlay's changes into the base directory. For git repos, performs a git merge from the overlay branch. For non-git directories, copies changed files from the upper directory.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be applied without making changes",
			},
			&cli.BoolFlag{
				Name:  "stop",
				Usage: "Stop the overlay after applying",
			},
			&cli.BoolFlag{
				Name:  "cleanup",
				Usage: "Cleanup overlay data after applying (implies --stop)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay",
				Required: true,
			},
		},
		Run: doApply,
	}
}

func doApply(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}
	dryRun := cmd.GetBool("dry-run")
	doStop := cmd.GetBool("stop")
	doCleanup := cmd.GetBool("cleanup")

	if doCleanup {
		doStop = true
	}

	return processApply(ctx, name, dryRun, doStop, doCleanup)
}

func processApply(ctx context.Context, name string, dryRun, doStop, doCleanup bool) error {
	log.Debug("Applying overlay %q (dry-run=%v, stop=%v, cleanup=%v)", name, dryRun, doStop, doCleanup)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	gitOps := git.NewOperations()
	isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)

	if isGit && ovl.Branch != "" {
		err = applyGit(ctx, ovl, gitOps, dryRun)
	} else {
		err = applyFileCopy(ovl, dryRun)
	}

	if err != nil {
		return err
	}

	// Optionally stop/cleanup the overlay
	if doStop && !dryRun {
		return processStop(ctx, name, doCleanup, false, false)
	}

	return nil
}

// applyGit merges the overlay branch into the base repo's current branch
func applyGit(ctx context.Context, ovl *api.Overlay, gitOps *git.Operations, dryRun bool) error {
	// First, auto-commit any uncommitted changes in the overlay
	hasChanges, err := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
	if err != nil {
		return fmt.Errorf("failed to check overlay changes: %w", err)
	}

	if hasChanges {
		if dryRun {
			log.Info("[dry-run] Would auto-commit uncommitted changes in overlay")
		} else {
			log.Debug("Auto-committing uncommitted changes before merge")
			if err := gitOps.CommitAll(ctx, ovl.MountPoint, fmt.Sprintf("phantom: auto-commit before apply (%s)", ovl.Name)); err != nil {
				return fmt.Errorf("failed to auto-commit overlay changes: %w", err)
			}
		}
	}

	// Get the base repo's current branch
	baseBranch, err := gitOps.GetCurrentBranch(ctx, ovl.BaseDir)
	if err != nil {
		return fmt.Errorf("failed to get base branch: %w", err)
	}

	if dryRun {
		log.Info("[dry-run] Would merge branch %q into %q in %s", ovl.Branch, baseBranch, ovl.BaseDir)
		return nil
	}

	// Merge the overlay branch into the base
	log.Debug("Merging %q into %q", ovl.Branch, baseBranch)
	if err := gitOps.MergeBranch(ctx, ovl.BaseDir, ovl.Branch); err != nil {
		return fmt.Errorf("merge failed: %w (resolve conflicts manually in %s)", err, ovl.BaseDir)
	}

	log.Info("Merged %q into %q", ovl.Branch, baseBranch)
	return nil
}

// applyFileCopy copies changed files from the upper directory to the base
func applyFileCopy(ovl *api.Overlay, dryRun bool) error {
	if ovl.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", ovl.Name)
	}

	copied := 0
	deleted := 0

	err := filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(ovl.UpperDir, path)
		if err != nil || relPath == "." {
			return nil
		}

		// Skip overlayfs work directory internals
		if strings.HasPrefix(relPath, "work/") || relPath == "work" {
			return nil
		}

		destPath := filepath.Join(ovl.BaseDir, relPath)
		baseName := filepath.Base(path)

		// Handle whiteout files (deletions)
		if strings.HasPrefix(baseName, ".wh.") {
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			deletedPath := filepath.Join(filepath.Dir(destPath), deletedName)
			if dryRun {
				log.Info("[dry-run] Would delete %s", relPath)
			} else {
				if err := os.RemoveAll(deletedPath); err != nil {
					log.Warn("Failed to delete %s: %v", deletedPath, err)
				}
			}
			deleted++
			return nil
		}

		// Handle directories
		if info.IsDir() {
			if !dryRun {
				return os.MkdirAll(destPath, info.Mode())
			}
			return nil
		}

		// Copy file
		if dryRun {
			log.Info("[dry-run] Would copy %s", relPath)
		} else {
			if err := copyFile(path, destPath, info.Mode()); err != nil {
				return fmt.Errorf("failed to copy %s: %w", relPath, err)
			}
		}
		copied++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk upper directory: %w", err)
	}

	action := "Applied"
	if dryRun {
		action = "Would apply"
	}
	log.Info("%s %d file(s), %d deletion(s) from overlay %q", action, copied, deleted, ovl.Name)
	return nil
}

// copyFile copies a single file preserving permissions
func copyFile(src, dst string, mode os.FileMode) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
