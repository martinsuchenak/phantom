package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

func NewRenameCommand() *cli.Command {
	return &cli.Command{
		Name:  "rename",
		Usage: "Rename an overlay",
		Description: "Renames an overlay, updating state, directories, and mount point. " +
			"The overlay must be stopped (unmounted) first.",
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "old-name", Usage: "Current overlay name", Required: true},
			&cli.StringArg{Name: "new-name", Usage: "New overlay name", Required: true},
		},
		Run: doRename,
	}
}

func doRename(ctx context.Context, cmd *cli.Command) error {
	oldName := cmd.GetStringArg("old-name")
	newName := cmd.GetStringArg("new-name")

	if oldName == newName {
		return fmt.Errorf("old and new names are the same")
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(oldName)
	if err != nil {
		return err
	}

	if store.Exists(newName) {
		return api.NewError(api.ErrAlreadyExists, fmt.Sprintf("overlay %q already exists", newName), nil)
	}

	// Must be unmounted
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return err
	}
	if mounted {
		return fmt.Errorf("overlay must be stopped before renaming (use: phantom stop %s)", oldName)
	}

	// Rename overlay directory
	oldOverlayDir := filepath.Join(cfg.GetOverlaysPath(), oldName)
	newOverlayDir := filepath.Join(cfg.GetOverlaysPath(), newName)
	if _, err := os.Stat(oldOverlayDir); err == nil {
		if err := os.Rename(oldOverlayDir, newOverlayDir); err != nil {
			return fmt.Errorf("failed to rename overlay directory: %w", err)
		}
	}

	// Rename mount point directory
	oldMountDir := filepath.Join(cfg.GetMountPath(), oldName)
	newMountDir := filepath.Join(cfg.GetMountPath(), newName)
	if _, err := os.Stat(oldMountDir); err == nil {
		if err := os.Rename(oldMountDir, newMountDir); err != nil {
			return fmt.Errorf("failed to rename mount point: %w", err)
		}
	}

	// Rename log file
	oldLog := filepath.Join(cfg.GetLogsPath(), oldName+".log")
	newLog := filepath.Join(cfg.GetLogsPath(), newName+".log")
	if _, err := os.Stat(oldLog); err == nil {
		_ = os.Rename(oldLog, newLog) // best effort
	}

	// Update state
	ovl.Name = newName
	ovl.UpperDir = filepath.Join(newOverlayDir, "upper")
	ovl.MountPoint = newMountDir

	if err := store.Save(ovl); err != nil {
		return fmt.Errorf("failed to save new state: %w", err)
	}
	_ = store.Delete(oldName)

	log.Info("Renamed %q -> %q", oldName, newName)
	return nil
}
