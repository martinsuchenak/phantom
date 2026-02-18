package overlay

import (
	"github.com/martinsuchenak/phantom/pkg/api"
)

// Manager provides an interface for managing overlay filesystems
type Manager interface {
	// Create creates and mounts a new overlay filesystem
	Create(opts *api.CreateOptions) (*api.Overlay, error)

	// Mount mounts an existing overlay
	Mount(overlay *api.Overlay) error

	// Unmount unmounts an overlay
	Unmount(overlay *api.Overlay) error

	// IsMounted checks if an overlay is currently mounted
	IsMounted(overlay *api.Overlay) (bool, error)

	// GetStatus returns the current status of an overlay
	GetStatus(overlay *api.Overlay) (*api.OverlayStatus, error)

	// Cleanup removes all overlay data (upper dir, work dir)
	Cleanup(overlay *api.Overlay) error

	// Prune removes stale/unused overlay resources
	Prune() error
}
