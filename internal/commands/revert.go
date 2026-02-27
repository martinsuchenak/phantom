package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewRevertCommand creates a new revert command
func NewRevertCommand() *cli.Command {
	return &cli.Command{
		Name:        "revert",
		Usage:       "Revert a file or directory in an overlay to its base state",
		Description: "Removes modifications and deletions for a specific path in an active overlay, restoring it to the original state from the base repository.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "overlay",
				Usage:    "Name of the overlay",
				Required: true,
			},
			&cli.StringArg{
				Name:     "path",
				Usage:    "Relative path to revert (e.g. config/settings.json)",
				Required: true,
			},
		},
		Run: doRevert,
	}
}

func doRevert(ctx context.Context, cmd *cli.Command) error {
	overlayName := cmd.GetStringArg("overlay")
	targetPath := cmd.GetStringArg("path")

	if overlayName == "" || targetPath == "" {
		return fmt.Errorf("overlay and path are required")
	}

	return processRevert(overlayName, targetPath)
}

func processRevert(overlayName, targetPath string) error {
	log.Debug("Reverting %q in overlay %q", targetPath, overlayName)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(overlayName)
	if err != nil {
		return err
	}

	if ovl.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", overlayName)
	}

	upperPath := filepath.Join(ovl.UpperDir, targetPath)

	// Fast sanity check: make sure we don't accidentally escape upperdir
	rel, err := filepath.Rel(ovl.UpperDir, upperPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("invalid path: %s escapes overlay directory", targetPath)
	}

	// 1. Remove normal modifications or additions
	if err := os.RemoveAll(upperPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to revert changes for %q: %w", targetPath, err)
	}

	// 2. Remove whiteout files (which overlayfs uses to mark deletions)
	// Whiteout files are named " .wh.<basename>" and are located in the parent directory
	// of the deleted item.
	baseName := filepath.Base(targetPath)
	parentDir := filepath.Dir(targetPath)
	whiteoutPath := filepath.Join(ovl.UpperDir, parentDir, ".wh."+baseName)

	if err := os.Remove(whiteoutPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to revert deletion marker for %q: %w", targetPath, err)
	}

	log.Info("Successfully reverted %q in overlay %q", targetPath, overlayName)
	return nil
}
