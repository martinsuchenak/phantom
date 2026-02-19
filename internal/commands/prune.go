package commands

import (
	"context"
	"fmt"
	"time"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewPruneCommand creates the prune command
func NewPruneCommand() *cli.Command {
	return &cli.Command{
		Name:        "prune",
		Usage:       "Remove stale and expired overlays",
		Description: "Cleans up unmounted overlays and removes overlays older than the configured auto_cleanup_days threshold.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be removed without actually removing",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Also remove mounted but expired overlays (will unmount them)",
			},
		},
		Run: doPrune,
	}
}

func doPrune(ctx context.Context, cmd *cli.Command) error {
	dryRun := cmd.GetBool("dry-run")
	force := cmd.GetBool("force")

	return processPrune(dryRun, force)
}

func processPrune(dryRun, force bool) error {
	log.Debug("Pruning overlays (dry-run=%v, force=%v)", dryRun, force)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	overlays, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load overlays: %w", err)
	}

	maxAge := time.Duration(cfg.Overlay.AutoCleanupDays) * 24 * time.Hour
	now := time.Now()
	pruned := 0
	skipped := 0

	for _, ovl := range overlays {
		// Skip persistent overlays
		if ovl.Persistent {
			log.Debug("Skipping persistent overlay %q", ovl.Name)
			continue
		}

		mounted, _ := mgr.IsMounted(ovl)
		age := now.Sub(ovl.CreatedAt)
		expired := maxAge > 0 && age > maxAge

		// Determine if we should remove this overlay
		reason := ""

		if !mounted {
			// Unmounted overlays: always candidates for pruning
			reason = "unmounted"
			if expired {
				reason = fmt.Sprintf("unmounted, expired (%s old)", formatDuration(age))
			}
		} else if expired && force {
			// Mounted but expired: only with --force
			reason = fmt.Sprintf("expired (%s old, force unmount)", formatDuration(age))
		} else if expired {
			// Mounted and expired but no --force
			log.Warn("Overlay %q is expired (%s old) but still mounted, use --force to remove", ovl.Name, formatDuration(age))
			skipped++
			continue
		} else {
			// Mounted and not expired
			continue
		}

		if dryRun {
			log.Info("[dry-run] Would remove %q (%s)", ovl.Name, reason)
			pruned++
			continue
		}

		log.Info("Removing %q (%s)", ovl.Name, reason)

		// Cleanup the overlay (unmounts if needed)
		if err := mgr.Cleanup(ovl); err != nil {
			log.Warn("Failed to cleanup overlay %q: %v", ovl.Name, err)
			continue
		}

		// Remove state
		if err := store.Delete(ovl.Name); err != nil {
			log.Warn("Failed to delete state for %q: %v", ovl.Name, err)
			continue
		}

		pruned++
	}

	// Also run the platform-level prune for orphaned resources
	if !dryRun {
		if err := mgr.Prune(); err != nil {
			log.Debug("Platform prune: %v", err)
		}
	}

	if pruned == 0 && skipped == 0 {
		log.Info("Nothing to prune")
	} else {
		action := "Pruned"
		if dryRun {
			action = "Would prune"
		}
		log.Info("%s %d overlay(s)", action, pruned)
		if skipped > 0 {
			log.Info("Skipped %d mounted overlay(s), use --force to include them", skipped)
		}
	}

	return nil
}
