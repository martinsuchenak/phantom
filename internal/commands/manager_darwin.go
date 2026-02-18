//go:build darwin

package commands

import (
	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/overlay"
)

// newOverlayManager creates a Darwin (macOS) overlay manager
func newOverlayManager(cfg *config.Config) (overlayManager, error) {
	return overlay.NewManager(cfg.GetStatePath(), cfg.Darwin.UnionFSPath, cfg.Darwin.FUSEOptions)
}
