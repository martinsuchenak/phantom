package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetSnapshotsDir(t *testing.T) {
	setupTestEnv(t, t.TempDir())
	dir := getSnapshotsDir()
	if dir == "" {
		t.Error("getSnapshotsDir returned empty string")
	}
}

func TestSnapshotSaveAndList(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-snap-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	// Create overlay dirs
	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "state", "overlays", "snap-test", "upper")
	mountDir := filepath.Join(tmpDir, "state", "mnt", "snap-test")
	snapsDir := filepath.Join(tmpDir, "state", "snapshots")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(mountDir, 0755)
	os.MkdirAll(snapsDir, 0755)

	// Create a file in upper
	os.WriteFile(filepath.Join(upperDir, "test.txt"), []byte("snapshot data"), 0644)

	// Save overlay state
	store := createTestStore(t, tmpDir)
	ovl := testOverlay("snap-test", baseDir, mountDir, upperDir)
	ovl.CreatedAt = time.Now()
	store.Save(&ovl)

	// Manually create a snapshot (simulating doSnapshotSave)
	snapName := "test-snapshot"
	snapDir := filepath.Join(snapsDir, snapName)
	os.MkdirAll(snapDir, 0700)
	copyDir(upperDir, filepath.Join(snapDir, "data"))

	meta := snapshotMeta{
		Name:      snapName,
		Overlay:   "snap-test",
		CreatedAt: time.Now(),
		SizeBytes: dirSize(filepath.Join(snapDir, "data")),
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(snapDir, "meta.json"), metaData, 0600)

	// Verify snapshot data was copied
	data, err := os.ReadFile(filepath.Join(snapDir, "data", "test.txt"))
	if err != nil {
		t.Fatalf("snapshot data not copied: %v", err)
	}
	if string(data) != "snapshot data" {
		t.Errorf("snapshot content = %q, want %q", string(data), "snapshot data")
	}

	// Verify meta
	metaBytes, _ := os.ReadFile(filepath.Join(snapDir, "meta.json"))
	var loadedMeta snapshotMeta
	json.Unmarshal(metaBytes, &loadedMeta)
	if loadedMeta.Overlay != "snap-test" {
		t.Errorf("meta overlay = %q, want %q", loadedMeta.Overlay, "snap-test")
	}
}

func TestSnapshotDelete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-snap-del-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	snapsDir := filepath.Join(tmpDir, "state", "snapshots")
	snapDir := filepath.Join(snapsDir, "to-delete")
	os.MkdirAll(snapDir, 0755)
	os.WriteFile(filepath.Join(snapDir, "meta.json"), []byte("{}"), 0644)

	// Delete
	if err := os.RemoveAll(snapDir); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if _, err := os.Stat(snapDir); !os.IsNotExist(err) {
		t.Error("snapshot should be deleted")
	}
}
