package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewRestartCommand creates the restart command
func NewRestartCommand() *cli.Command {
	return &cli.Command{
		Name:        "restart",
		Usage:       "Remount an unmounted overlay",
		Description: "Remounts an overlay that was previously unmounted (e.g. after a reboot or crash). The overlay state must still exist.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay to restart",
				Required: true,
			},
		},
		Run: doRestart,
	}
}

func doRestart(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}

	return processRestart(name)
}

func processRestart(name string) error {
	log.Debug("Restarting overlay %q", name)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	// Check if already mounted
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return fmt.Errorf("failed to check mount status: %w", err)
	}
	if mounted {
		log.Info("Overlay %q is already mounted at %s", name, ovl.MountPoint)
		return nil
	}

	// Remount
	if err := mgr.Mount(ovl); err != nil {
		return fmt.Errorf("failed to remount overlay: %w", err)
	}

	// Update state (PID may have changed)
	if err := store.Save(ovl); err != nil {
		log.Warn("Failed to update state: %v", err)
	}

	log.Info("Overlay %q remounted at %s", name, ovl.MountPoint)
	return nil
}
