package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewWatchCommand creates the watch command
func NewWatchCommand() *cli.Command {
	return &cli.Command{
		Name:        "watch",
		Usage:       "Watch file changes in an overlay in real time",
		Description: "Monitors the overlay's upper directory for file changes and streams them as they happen.",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:         "interval",
				Aliases:      []string{"i"},
				Usage:        "Poll interval in seconds",
				DefaultValue: 2,
			},
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Output format (simple, json)",
				DefaultValue: "simple",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay to watch",
				Required: true,
			},
		},
		Run: doWatch,
	}
}

// fileSnapshot holds mod time and size for change detection
type fileSnapshot struct {
	ModTime time.Time
	Size    int64
	IsWhiteout bool
}

func doWatch(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	interval := cmd.GetInt("interval")
	format := cmd.GetString("format")

	if interval < 1 {
		interval = 1
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	if ovl.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", name)
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return err
	}
	if !mounted {
		return fmt.Errorf("overlay %q is not mounted", name)
	}

	log.Info("Watching overlay %q (Ctrl+C to stop)", name)

	return pollWatch(ctx, ovl.UpperDir, ovl.BaseDir, time.Duration(interval)*time.Second, format)
}

func pollWatch(ctx context.Context, upperDir, baseDir string, interval time.Duration, format string) error {
	known := make(map[string]fileSnapshot)

	// Initial scan — populate known files silently
	scanUpperDir(upperDir, baseDir, known, nil, "")

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			current := make(map[string]fileSnapshot)
			scanUpperDir(upperDir, baseDir, current, nil, "")

			// Detect new or modified files
			for path, snap := range current {
				old, existed := known[path]
				if !existed {
					status := "+"
					if snap.IsWhiteout {
						status = "-"
					} else if fileExistsInBase(baseDir, path) {
						status = "~"
					}
					printWatchEvent(format, status, path)
				} else if snap.ModTime != old.ModTime || snap.Size != old.Size {
					printWatchEvent(format, "~", path)
				}
			}

			// Detect removed files (were in upper, now gone — overlay reset)
			for path := range known {
				if _, exists := current[path]; !exists {
					printWatchEvent(format, "⊘", path)
				}
			}

			known = current
		}
	}
}

func scanUpperDir(upperDir, baseDir string, out map[string]fileSnapshot, _ map[string]fileSnapshot, _ string) {
	filepath.Walk(upperDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, err := filepath.Rel(upperDir, path)
		if err != nil || relPath == "." {
			return nil
		}
		if strings.HasPrefix(relPath, "work/") || relPath == "work" {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		baseName := filepath.Base(path)
		isWhiteout := strings.HasPrefix(baseName, ".wh.")
		if isWhiteout {
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			relPath = filepath.Join(filepath.Dir(relPath), deletedName)
		}

		out[relPath] = fileSnapshot{
			ModTime:    info.ModTime(),
			Size:       info.Size(),
			IsWhiteout: isWhiteout,
		}
		return nil
	})
}

func fileExistsInBase(baseDir, relPath string) bool {
	_, err := os.Stat(filepath.Join(baseDir, relPath))
	return err == nil
}

func printWatchEvent(format, status, path string) {
	ts := time.Now().Format("15:04:05")
	if format == "json" {
		label := "added"
		switch status {
		case "~":
			label = "modified"
		case "-":
			label = "deleted"
		case "⊘":
			label = "reset"
		}
		fmt.Printf(`{"time":"%s","status":"%s","file":"%s"}`+"\n", ts, label, path)
	} else {
		fmt.Printf("[%s] %s %s\n", ts, status, path)
	}
}
