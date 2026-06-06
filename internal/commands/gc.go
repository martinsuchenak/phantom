package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

func NewGCCommand() *cli.Command {
	return &cli.Command{
		Name:  "gc",
		Usage: "Garbage collect orphaned resources",
		Description: "Cleans up orphaned overlay directories, stale mount points, " +
			"empty log files, and broken snapshots that aren't tracked by any overlay state.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "Show what would be removed without removing"},
		},
		Run: doGC,
	}
}

func doGC(ctx context.Context, cmd *cli.Command) error {
	dryRun := cmd.GetBool("dry-run")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	overlays, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load overlays: %w", err)
	}

	// Build set of known overlay names
	known := make(map[string]bool)
	for _, ovl := range overlays {
		known[ovl.Name] = true
	}

	cleaned := 0

	// 1. Orphaned overlay data directories
	cleaned += gcDir(cfg.GetOverlaysPath(), known, "overlay data", dryRun)

	// 2. Orphaned mount point directories
	cleaned += gcDir(cfg.GetMountPath(), known, "mount point", dryRun)

	// 3. Orphaned log files
	cleaned += gcLogs(cfg.GetLogsPath(), known, dryRun)

	// 4. Broken snapshots (overlay no longer exists)
	cleaned += gcSnapshots(cfg.GetSnapshotsPath(), known, dryRun)

	if cleaned == 0 {
		log.Info("Nothing to clean up")
	} else if dryRun {
		log.Info("Would remove %d orphaned resource(s)", cleaned)
	} else {
		log.Info("Removed %d orphaned resource(s)", cleaned)
	}
	return nil
}

func gcDir(dir string, known map[string]bool, label string, dryRun bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cleaned := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if known[e.Name()] {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if dryRun {
			log.Info("[dry-run] Would remove orphaned %s: %s", label, path)
		} else {
			if err := os.RemoveAll(path); err != nil {
				log.Warn("Failed to remove %s: %v", path, err)
				continue
			}
			log.Info("Removed orphaned %s: %s", label, e.Name())
		}
		cleaned++
	}
	return cleaned
}

func gcLogs(dir string, known map[string]bool, dryRun bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cleaned := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		overlayName := strings.TrimSuffix(name, ".log")
		if known[overlayName] {
			continue
		}
		path := filepath.Join(dir, name)
		if dryRun {
			log.Info("[dry-run] Would remove orphaned log: %s", name)
		} else {
			_ = os.Remove(path)
			log.Info("Removed orphaned log: %s", name)
		}
		cleaned++
	}
	return cleaned
}

func gcSnapshots(dir string, known map[string]bool, dryRun bool) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	cleaned := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(dir, e.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			// Broken snapshot (no meta.json)
			path := filepath.Join(dir, e.Name())
			if dryRun {
				log.Info("[dry-run] Would remove broken snapshot: %s", e.Name())
			} else {
				_ = os.RemoveAll(path)
				log.Info("Removed broken snapshot: %s", e.Name())
			}
			cleaned++
			continue
		}
		// Check if the overlay still exists
		var meta struct {
			Overlay string `json:"overlay"`
		}
		if err := json.Unmarshal(data, &meta); err == nil && !known[meta.Overlay] {
			path := filepath.Join(dir, e.Name())
			if dryRun {
				log.Info("[dry-run] Would remove orphaned snapshot: %s (overlay %q gone)", e.Name(), meta.Overlay)
			} else {
				_ = os.RemoveAll(path)
				log.Info("Removed orphaned snapshot: %s (overlay %q gone)", e.Name(), meta.Overlay)
			}
			cleaned++
		}
	}
	return cleaned
}
