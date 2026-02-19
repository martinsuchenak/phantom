package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

type snapshotMeta struct {
	Name      string    `json:"name"`
	Overlay   string    `json:"overlay"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
}

func getSnapshotsDir() string {
	return cfg.GetSnapshotsPath()
}

func doSnapshotSave(ctx context.Context, cmd *cli.Command) error {
	overlayName := cmd.GetStringArg("name")
	snapName := cmd.GetString("snapshot-name")
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	ovl, err := store.Load(overlayName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(ovl.UpperDir); os.IsNotExist(err) {
		return fmt.Errorf("upper directory does not exist: %s", ovl.UpperDir)
	}
	if snapName == "" {
		snapName = fmt.Sprintf("%s-%s", overlayName, time.Now().Format("20060102-150405"))
	}
	snapDir := filepath.Join(getSnapshotsDir(), snapName)
	if _, err := os.Stat(snapDir); err == nil {
		return fmt.Errorf("snapshot %q already exists", snapName)
	}
	if err := os.MkdirAll(snapDir, 0700); err != nil {
		return fmt.Errorf("failed to create snapshot directory: %w", err)
	}
	upperDataDir := filepath.Join(snapDir, "data")
	if err := copyDir(ovl.UpperDir, upperDataDir); err != nil {
		os.RemoveAll(snapDir)
		return fmt.Errorf("failed to copy overlay data: %w", err)
	}
	size := dirSize(upperDataDir)
	meta := snapshotMeta{
		Name: snapName, Overlay: overlayName,
		CreatedAt: time.Now(), SizeBytes: size,
	}
	metaData, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		os.RemoveAll(snapDir)
		return err
	}
	if err := os.WriteFile(filepath.Join(snapDir, "meta.json"), metaData, 0600); err != nil {
		os.RemoveAll(snapDir)
		return err
	}
	log.Info("Snapshot %q saved (%s)", snapName, formatSize(size))
	return nil
}

func doSnapshotRestore(ctx context.Context, cmd *cli.Command) error {
	overlayName := cmd.GetStringArg("name")
	snapName := cmd.GetStringArg("snapshot")
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	ovl, err := store.Load(overlayName)
	if err != nil {
		return err
	}
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	mounted, err := mgr.IsMounted(ovl)
	if err != nil {
		return fmt.Errorf("failed to check mount status: %w", err)
	}
	if mounted {
		return fmt.Errorf("overlay %q must be unmounted before restoring (use: phantom stop %s)", overlayName, overlayName)
	}
	snapDataDir := filepath.Join(getSnapshotsDir(), snapName, "data")
	if _, err := os.Stat(snapDataDir); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %q not found", snapName)
	}
	if err := os.RemoveAll(ovl.UpperDir); err != nil {
		return fmt.Errorf("failed to clear upper directory: %w", err)
	}
	if err := copyDir(snapDataDir, ovl.UpperDir); err != nil {
		return fmt.Errorf("failed to restore snapshot data: %w", err)
	}
	log.Info("Restored overlay %q from snapshot %q", overlayName, snapName)
	log.Info("Run 'phantom restart %s' to remount", overlayName)
	return nil
}

func doSnapshotList(ctx context.Context, cmd *cli.Command) error {
	overlayFilter := cmd.GetStringArg("name")
	format := cmd.GetString("format")
	snapshotsDir := getSnapshotsDir()
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("No snapshots found")
			return nil
		}
		return fmt.Errorf("failed to read snapshots directory: %w", err)
	}
	var snapshots []snapshotMeta
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(snapshotsDir, entry.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var meta snapshotMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if overlayFilter != "" && meta.Overlay != overlayFilter {
			continue
		}
		snapshots = append(snapshots, meta)
	}
	if len(snapshots) == 0 {
		log.Info("No snapshots found")
		return nil
	}
	if format == "json" {
		data, err := json.MarshalIndent(snapshots, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SNAPSHOT\tOVERLAY\tCREATED\tSIZE")
	for _, s := range snapshots {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, s.Overlay,
			s.CreatedAt.Format("2006-01-02 15:04"), formatSize(s.SizeBytes))
	}
	return w.Flush()
}

func doSnapshotDelete(ctx context.Context, cmd *cli.Command) error {
	snapName := cmd.GetStringArg("snapshot")
	snapDir := filepath.Join(getSnapshotsDir(), snapName)
	if _, err := os.Stat(snapDir); os.IsNotExist(err) {
		return fmt.Errorf("snapshot %q not found", snapName)
	}
	if err := os.RemoveAll(snapDir); err != nil {
		return fmt.Errorf("failed to delete snapshot: %w", err)
	}
	log.Info("Deleted snapshot %q", snapName)
	return nil
}
