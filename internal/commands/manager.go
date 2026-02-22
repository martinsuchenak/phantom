package commands

import (
	"github.com/martinsuchenak/phantom/pkg/api"
)

// overlayManager is an interface that both Linux and Darwin managers implement
type overlayManager interface {
	Create(opts *api.CreateOptions) (*api.Overlay, error)
	Mount(overlay *api.Overlay) error
	Unmount(overlay *api.Overlay) error
	IsMounted(overlay *api.Overlay) (bool, error)
	GetStatus(overlay *api.Overlay) (*api.OverlayStatus, error)
	Cleanup(overlay *api.Overlay) error
	Prune() error
}

// createOverlayManagerFunc is the function used to create overlay managers.
// It can be overridden in tests to inject a mock.
var createOverlayManagerFunc = defaultCreateOverlayManager

// createOverlayManager creates the platform-specific overlay manager
func createOverlayManager() (overlayManager, error) {
	return createOverlayManagerFunc()
}

func defaultCreateOverlayManager() (overlayManager, error) {
	return newOverlayManager(cfg)
}
