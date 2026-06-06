package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestSnapshotSaveAndList(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	// Create overlay with upper dir containing files
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(upperDir, 0755)
	_ = os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("snapshot data"), 0644)

	ovl := &api.Overlay{
		Name:       "snap-overlay",
		BaseDir:    tmpDir,
		MountPoint: filepath.Join(tmpDir, "mnt"),
		UpperDir:   upperDir,
		CreatedAt:  time.Now(),
	}
	_ = store.Save(ovl)

	// Manually save a snapshot (simulating doSnapshotSave)
	snapName := "test-snapshot"
	snapDir := filepath.Join(cfg.GetSnapshotsPath(), snapName)
	_ = os.MkdirAll(filepath.Join(snapDir, "data"), 0700)

	if err := copyDir(upperDir, filepath.Join(snapDir, "data")); err != nil {
		t.Fatalf("failed to copy: %v", err)
	}

	meta := snapshotMeta{
		Name:      snapName,
		Overlay:   "snap-overlay",
		CreatedAt: time.Now(),
		SizeBytes: dirSize(filepath.Join(snapDir, "data")),
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	_ = os.WriteFile(filepath.Join(snapDir, "meta.json"), metaData, 0600)

	// Verify snapshot exists
	if _, err := os.Stat(filepath.Join(snapDir, "data", "file.txt")); os.IsNotExist(err) {
		t.Error("snapshot should contain file.txt")
	}

	// Verify meta
	data, _ := os.ReadFile(filepath.Join(snapDir, "meta.json"))
	var loaded snapshotMeta
	_ = json.Unmarshal(data, &loaded)
	if loaded.Name != snapName {
		t.Errorf("expected name %q, got %q", snapName, loaded.Name)
	}
	if loaded.Overlay != "snap-overlay" {
		t.Errorf("expected overlay 'snap-overlay', got %q", loaded.Overlay)
	}
	if loaded.SizeBytes <= 0 {
		t.Error("expected positive size")
	}
}

func TestSnapshotRestore(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// Create snapshot data
	snapDir := filepath.Join(cfg.GetSnapshotsPath(), "restore-snap")
	_ = os.MkdirAll(filepath.Join(snapDir, "data"), 0700)
	_ = os.WriteFile(filepath.Join(snapDir, "data", "restored.txt"), []byte("restored"), 0644)

	// Create target upper dir
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(upperDir, 0755)
	_ = os.WriteFile(filepath.Join(upperDir, "old.txt"), []byte("old"), 0644)

	// Simulate restore: clear upper, copy snapshot data
	_ = os.RemoveAll(upperDir)
	if err := copyDir(filepath.Join(snapDir, "data"), upperDir); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify restored file exists
	data, err := os.ReadFile(filepath.Join(upperDir, "restored.txt"))
	if err != nil {
		t.Fatal("restored.txt should exist")
	}
	if string(data) != "restored" {
		t.Errorf("expected 'restored', got %q", string(data))
	}

	// Old file should be gone
	if _, err := os.Stat(filepath.Join(upperDir, "old.txt")); !os.IsNotExist(err) {
		t.Error("old.txt should be removed after restore")
	}
}

func TestSnapshotListFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	snapshotsDir := cfg.GetSnapshotsPath()

	// Create snapshots for different overlays
	for _, s := range []struct{ name, overlay string }{
		{"snap-a", "overlay-a"},
		{"snap-b", "overlay-b"},
		{"snap-c", "overlay-a"},
	} {
		dir := filepath.Join(snapshotsDir, s.name)
		_ = os.MkdirAll(dir, 0700)
		meta, _ := json.Marshal(snapshotMeta{Name: s.name, Overlay: s.overlay})
		_ = os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0600)
	}

	// Read all snapshots
	entries, _ := os.ReadDir(snapshotsDir)
	var all []snapshotMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, _ := os.ReadFile(filepath.Join(snapshotsDir, e.Name(), "meta.json"))
		var m snapshotMeta
		_ = json.Unmarshal(data, &m)
		all = append(all, m)
	}

	if len(all) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(all))
	}

	// Filter for overlay-a
	var filtered []snapshotMeta
	for _, m := range all {
		if m.Overlay == "overlay-a" {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 snapshots for overlay-a, got %d", len(filtered))
	}
}
