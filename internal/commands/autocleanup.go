package commands

import (
	"time"

	"github.com/martinsuchenak/phantom/internal/state"
)

// runAutoCleanup checks for expired overlays and removes non-persistent,
// unmounted ones that exceed the configured auto_cleanup_days threshold.
// This runs silently in the background during start/run commands.
func runAutoCleanup() {
	if cfg.Overlay.AutoCleanupDays <= 0 {
		return
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		log.Debug("Auto-cleanup: failed to init store: %v", err)
		return
	}

	mgr, err := createOverlayManager()
	if err != nil {
		log.Debug("Auto-cleanup: failed to create manager: %v", err)
		return
	}

	overlays, err := store.LoadAll()
	if err != nil {
		log.Debug("Auto-cleanup: failed to load overlays: %v", err)
		return
	}

	maxAge := time.Duration(cfg.Overlay.AutoCleanupDays) * 24 * time.Hour
	now := time.Now()
	cleaned := 0

	for _, ovl := range overlays {
		// Skip persistent overlays
		if ovl.Persistent {
			continue
		}

		// Only auto-clean expired overlays
		if now.Sub(ovl.CreatedAt) <= maxAge {
			continue
		}

		// Only auto-clean unmounted overlays (don't disrupt running work)
		mounted, _ := mgr.IsMounted(ovl)
		if mounted {
			continue
		}

		log.Debug("Auto-cleanup: removing expired overlay %q (age: %s)", ovl.Name, formatDuration(now.Sub(ovl.CreatedAt)))

		if err := mgr.Cleanup(ovl); err != nil {
			log.Debug("Auto-cleanup: failed to cleanup %q: %v", ovl.Name, err)
			continue
		}

		if err := store.Delete(ovl.Name); err != nil {
			log.Debug("Auto-cleanup: failed to delete state for %q: %v", ovl.Name, err)
			continue
		}

		cleaned++
	}

	if cleaned > 0 {
		log.Debug("Auto-cleanup: removed %d expired overlay(s)", cleaned)
	}
}
