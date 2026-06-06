//go:build darwin

package commands

import (
	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/overlay"
)

// newOverlayManager creates a Darwin (macOS) overlay manager
func newOverlayManager(cfg *config.Config) (overlayManager, error) {
	fuseOptions := cfg.Darwin.FUSEOptions
	if cfg.Darwin.UseFSKit {
		fuseOptions = append([]string{"backend=fskit"}, fuseOptions...)
	}
	return overlay.NewManager(
		cfg.GetStatePath(),
		cfg.Darwin.UnionFSPath,
		fuseOptions,
		false, // Darwin always uses FUSE (unionfs-fuse)
	)
}
