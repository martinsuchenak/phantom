package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewLockCommand creates the lock command
func NewLockCommand() *cli.Command {
	return &cli.Command{
		Name:        "lock",
		Usage:       "Lock an overlay to prevent accidental stop/cleanup/prune",
		Description: "Locked overlays are protected from stop --cleanup, prune, and gc. Use 'phantom unlock' to remove the lock.",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doLock,
	}
}

// NewUnlockCommand creates the unlock command
func NewUnlockCommand() *cli.Command {
	return &cli.Command{
		Name:        "unlock",
		Usage:       "Unlock a previously locked overlay",
		Description: "Removes the lock from an overlay, allowing it to be stopped, cleaned up, or pruned.",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doUnlock,
	}
}

func doLock(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	return setLock(name, true)
}

func doUnlock(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	return setLock(name, false)
}

func setLock(name string, locked bool) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	if ovl.Locked == locked {
		if locked {
			log.Info("Overlay %q is already locked", name)
		} else {
			log.Info("Overlay %q is already unlocked", name)
		}
		return nil
	}

	ovl.Locked = locked
	if err := store.Save(ovl); err != nil {
		return fmt.Errorf("failed to save overlay state: %w", err)
	}

	if locked {
		log.Info("Overlay %q locked — protected from cleanup, prune, and gc", name)
	} else {
		log.Info("Overlay %q unlocked", name)
	}
	return nil
}
