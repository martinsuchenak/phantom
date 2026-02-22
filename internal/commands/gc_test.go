package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewGCCommand(t *testing.T) {
	cmd := NewGCCommand()
	if cmd.Name != "gc" {
		t.Errorf("expected command name 'gc', got %q", cmd.Name)
	}
}

func TestGcDir(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	dir := filepath.Join(tmpDir, "overlays")
	os.MkdirAll(filepath.Join(dir, "known-overlay"), 0755)
	os.MkdirAll(filepath.Join(dir, "orphaned-overlay"), 0755)

	known := map[string]bool{"known-overlay": true}

	t.Run("dry run", func(t *testing.T) {
		cleaned := gcDir(dir, known, "overlay data", true)
		if cleaned != 1 {
			t.Errorf("expected 1 orphaned, got %d", cleaned)
		}
		// Should still exist (dry run)
		if _, err := os.Stat(filepath.Join(dir, "orphaned-overlay")); os.IsNotExist(err) {
			t.Error("orphaned dir should still exist in dry run")
		}
	})

	t.Run("real run", func(t *testing.T) {
		cleaned := gcDir(dir, known, "overlay data", false)
		if cleaned != 1 {
			t.Errorf("expected 1 cleaned, got %d", cleaned)
		}
		if _, err := os.Stat(filepath.Join(dir, "orphaned-overlay")); !os.IsNotExist(err) {
			t.Error("orphaned dir should be removed")
		}
		// Known should still exist
		if _, err := os.Stat(filepath.Join(dir, "known-overlay")); os.IsNotExist(err) {
			t.Error("known dir should still exist")
		}
	})
}

func TestGcDirEmpty(t *testing.T) {
	cleaned := gcDir("/nonexistent/path", map[string]bool{}, "test", false)
	if cleaned != 0 {
		t.Errorf("expected 0 for nonexistent dir, got %d", cleaned)
	}
}

func TestGcLogs(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "logs")
	os.MkdirAll(logsDir, 0755)
	os.WriteFile(filepath.Join(logsDir, "known.log"), []byte("log"), 0644)
	os.WriteFile(filepath.Join(logsDir, "orphaned.log"), []byte("log"), 0644)
	os.WriteFile(filepath.Join(logsDir, "not-a-log.txt"), []byte("skip"), 0644)

	known := map[string]bool{"known": true}

	t.Run("dry run", func(t *testing.T) {
		cleaned := gcLogs(logsDir, known, true)
		if cleaned != 1 {
			t.Errorf("expected 1 orphaned log, got %d", cleaned)
		}
	})

	t.Run("real run", func(t *testing.T) {
		cleaned := gcLogs(logsDir, known, false)
		if cleaned != 1 {
			t.Errorf("expected 1 cleaned log, got %d", cleaned)
		}
		if _, err := os.Stat(filepath.Join(logsDir, "orphaned.log")); !os.IsNotExist(err) {
			t.Error("orphaned log should be removed")
		}
		if _, err := os.Stat(filepath.Join(logsDir, "known.log")); os.IsNotExist(err) {
			t.Error("known log should still exist")
		}
	})
}

func TestGcSnapshots(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	snapshotsDir := filepath.Join(tmpDir, "snapshots")

	// Known overlay snapshot
	knownSnap := filepath.Join(snapshotsDir, "known-snap")
	os.MkdirAll(knownSnap, 0755)
	meta1, _ := json.Marshal(map[string]string{"overlay": "known"})
	os.WriteFile(filepath.Join(knownSnap, "meta.json"), meta1, 0600)

	// Orphaned overlay snapshot
	orphanSnap := filepath.Join(snapshotsDir, "orphan-snap")
	os.MkdirAll(orphanSnap, 0755)
	meta2, _ := json.Marshal(map[string]string{"overlay": "deleted-overlay"})
	os.WriteFile(filepath.Join(orphanSnap, "meta.json"), meta2, 0600)

	// Broken snapshot (no meta.json)
	brokenSnap := filepath.Join(snapshotsDir, "broken-snap")
	os.MkdirAll(brokenSnap, 0755)

	known := map[string]bool{"known": true}

	t.Run("dry run", func(t *testing.T) {
		cleaned := gcSnapshots(snapshotsDir, known, true)
		if cleaned != 2 { // orphan + broken
			t.Errorf("expected 2 orphaned/broken snapshots, got %d", cleaned)
		}
	})

	t.Run("real run", func(t *testing.T) {
		cleaned := gcSnapshots(snapshotsDir, known, false)
		if cleaned != 2 {
			t.Errorf("expected 2 cleaned, got %d", cleaned)
		}
		if _, err := os.Stat(knownSnap); os.IsNotExist(err) {
			t.Error("known snapshot should still exist")
		}
	})
}
