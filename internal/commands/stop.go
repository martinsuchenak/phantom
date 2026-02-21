package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewStopCommand creates the stop command
func NewStopCommand() *cli.Command {
	return &cli.Command{
		Name:        "stop",
		Usage:       "Unmount and optionally cleanup an overlay",
		Description: "Unmounts the specified overlay and optionally removes all associated data.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "cleanup",
				Usage: "Remove overlay data after unmounting",
			},
			&cli.BoolFlag{
				Name:  "push",
				Usage: "Push branch to remote before stopping",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Force unmount if stuck",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay to stop",
				Required: true,
			},
		},
		Run: doStop,
	}
}

func doStop(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	force := cmd.GetBool("force")

	return processStop(ctx, name, doCleanup, doPush, force)
}

func processStop(ctx context.Context, name string, doCleanup, doPush, force bool) error {
	log.Debug("Stopping overlay %q (cleanup=%v, push=%v, force=%v)", name, doCleanup, doPush, force)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Load overlay state
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	// Check if locked
	if ovl.Locked && doCleanup && !force {
		return fmt.Errorf("overlay %q is locked — use 'phantom unlock %s' first, or --force to override", name, name)
	}

	// Create the overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	// Check if mounted
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		log.Debug("Failed to check mount status: %v", err)
	}

	if mounted {
		// Handle git operations if branch is set
		if ovl.Branch != "" {
			gitOps := git.NewOperations()

			if doPush || cfg.Git.AutoPushOnStop {
				log.Debug("Pushing branch %q to remote", ovl.Branch)
				if err := gitOps.PushBranch(ctx, ovl.MountPoint, ovl.Branch, false); err != nil {
					// Don't fail, just warn
					log.Warn("Failed to push branch: %v", err)
				}
			}
		}

		// Unmount
		if force {
			// Use force unmount
			if err := forceUnmount(mgr, ovl); err != nil {
				return err
			}
		} else {
			if err := mgr.Unmount(ovl); err != nil {
				return err
			}
		}
	}

	// Cleanup if requested
	if doCleanup {
		log.Debug("Cleaning up overlay data")
		if err := mgr.Cleanup(ovl); err != nil {
			return err
		}
	}

	// Remove state
	if doCleanup {
		if err := store.Delete(name); err != nil {
			return fmt.Errorf("failed to remove overlay state: %w", err)
		}
	}

	log.Info("Overlay %q stopped", name)
	return nil
}

// forceUnmount attempts to forcefully unmount an overlay
func forceUnmount(mgr overlayManager, ovl *api.Overlay) error {
	// Try platform-specific force unmount
	switch m := mgr.(type) {
	case interface{ ForceUnmount(*api.Overlay) error }:
		return m.ForceUnmount(ovl)
	default:
		// Fallback to regular unmount
		return mgr.Unmount(ovl)
	}
}
