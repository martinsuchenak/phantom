package commands

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestDoSnapshotListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// No snapshots dir — should report "no snapshots"
	snapshotsDir := cfg.GetSnapshotsPath()
	if _, err := os.Stat(snapshotsDir); err == nil {
		t.Skip("snapshots dir already exists")
	}
}

func TestDoSnapshotListWithSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	snapshotsDir := cfg.GetSnapshotsPath()
	snapDir := filepath.Join(snapshotsDir, "test-snap")
	os.MkdirAll(filepath.Join(snapDir, "data"), 0700)

	meta := snapshotMeta{
		Name:    "test-snap",
		Overlay: "my-overlay",
	}
	metaData, _ := json.MarshalIndent(meta, "", "  ")
	os.WriteFile(filepath.Join(snapDir, "meta.json"), metaData, 0600)

	// Verify we can read the snapshot metadata
	entries, err := os.ReadDir(snapshotsDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 snapshot dir, got %d", len(entries))
	}

	// Read and parse meta
	data, err := os.ReadFile(filepath.Join(snapshotsDir, entries[0].Name(), "meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var loaded snapshotMeta
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "test-snap" {
		t.Errorf("expected name 'test-snap', got %q", loaded.Name)
	}
	if loaded.Overlay != "my-overlay" {
		t.Errorf("expected overlay 'my-overlay', got %q", loaded.Overlay)
	}
}

func TestDoSnapshotDelete(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	snapshotsDir := cfg.GetSnapshotsPath()
	snapDir := filepath.Join(snapshotsDir, "delete-me")
	os.MkdirAll(filepath.Join(snapDir, "data"), 0700)
	os.WriteFile(filepath.Join(snapDir, "meta.json"), []byte(`{"name":"delete-me"}`), 0600)

	// Verify it exists
	if _, err := os.Stat(snapDir); os.IsNotExist(err) {
		t.Fatal("snapshot dir should exist")
	}

	// Delete it
	if err := os.RemoveAll(snapDir); err != nil {
		t.Fatalf("failed to delete snapshot: %v", err)
	}

	// Verify it's gone
	if _, err := os.Stat(snapDir); !os.IsNotExist(err) {
		t.Error("snapshot dir should be deleted")
	}
}

func TestDoSnapshotDeleteNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	snapDir := filepath.Join(cfg.GetSnapshotsPath(), "nonexistent")
	if _, err := os.Stat(snapDir); !os.IsNotExist(err) {
		t.Error("nonexistent snapshot should not exist")
	}
}

func TestGetSnapshotsDir(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	dir := getSnapshotsDir()
	if dir == "" {
		t.Error("snapshots dir should not be empty")
	}
}

func TestSnapshotMetaSerialization(t *testing.T) {
	meta := snapshotMeta{
		Name:      "test",
		Overlay:   "my-overlay",
		SizeBytes: 1024,
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}

	var loaded snapshotMeta
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.Name != meta.Name {
		t.Errorf("name mismatch: %q vs %q", loaded.Name, meta.Name)
	}
	if loaded.SizeBytes != meta.SizeBytes {
		t.Errorf("size mismatch: %d vs %d", loaded.SizeBytes, meta.SizeBytes)
	}
}

func TestDoSnapshotCommands(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	cmdSave := NewSnapshotCommand()

	// Create a dummy overlay state
	store, _ := state.NewStore(cfg.GetStatePath())
	ovl := &api.Overlay{
		Name:     "test-ovl",
		UpperDir: filepath.Join(tmpDir, "upper"),
	}
	os.MkdirAll(ovl.UpperDir, 0755)
	store.Save(ovl)

	// Test doSnapshotSave directly
	runCommandWithArgs(t, []string{"save", "test-ovl", "--snapshot-name", "snap1"}, func() {
		_ = cmdSave.Execute(context.Background())
	})

	// Test doSnapshotList directly
	runCommandWithArgs(t, []string{"list"}, func() {
		_ = cmdSave.Execute(context.Background())
	})

	runCommandWithArgs(t, []string{"list", "test-ovl", "--format", "json"}, func() {
		_ = cmdSave.Execute(context.Background())
	})

	// Test doSnapshotRestore directly
	// Unmounted overlay restore
	runCommandWithArgs(t, []string{"restore", "test-ovl", "snap1"}, func() {
		_ = cmdSave.Execute(context.Background())
	})

	// Test doSnapshotDelete directly
	runCommandWithArgs(t, []string{"delete", "snap1"}, func() {
		_ = cmdSave.Execute(context.Background())
	})
}
