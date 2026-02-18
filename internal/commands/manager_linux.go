//go:build linux

package commands

import (
	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/overlay"
)

// newOverlayManager creates a Linux overlay manager
func newOverlayManager(cfg *config.Config) (overlayManager, error) {
	return overlay.NewManager(cfg.GetStatePath(), "", nil)
}
