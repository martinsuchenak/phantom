package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGcDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-gc-dir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	dir := filepath.Join(tmpDir, "overlays")
	os.MkdirAll(filepath.Join(dir, "known-overlay"), 0755)
	os.MkdirAll(filepath.Join(dir, "orphan-overlay"), 0755)

	known := map[string]bool{"known-overlay": true}

	// Dry run
	cleaned := gcDir(dir, known, "overlay data", true)
	if cleaned != 1 {
		t.Errorf("dry-run: expected 1 orphan, got %d", cleaned)
	}
	// Orphan should still exist
	if _, err := os.Stat(filepath.Join(dir, "orphan-overlay")); os.IsNotExist(err) {
		t.Error("dry-run should not remove files")
	}

	// Real run
	cleaned = gcDir(dir, known, "overlay data", false)
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}
	if _, err := os.Stat(filepath.Join(dir, "orphan-overlay")); !os.IsNotExist(err) {
		t.Error("orphan should be removed")
	}
	// Known should still exist
	if _, err := os.Stat(filepath.Join(dir, "known-overlay")); os.IsNotExist(err) {
		t.Error("known overlay should not be removed")
	}
}

func TestGcDir_NonExistent(t *testing.T) {
	setupTestEnv(t, os.TempDir())
	cleaned := gcDir("/nonexistent/path", map[string]bool{}, "test", false)
	if cleaned != 0 {
		t.Errorf("expected 0 for nonexistent dir, got %d", cleaned)
	}
}

func TestGcLogs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-gc-logs-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "logs")
	os.MkdirAll(logsDir, 0755)

	os.WriteFile(filepath.Join(logsDir, "known.log"), []byte("log"), 0644)
	os.WriteFile(filepath.Join(logsDir, "orphan.log"), []byte("log"), 0644)
	os.WriteFile(filepath.Join(logsDir, "readme.txt"), []byte("not a log"), 0644)

	known := map[string]bool{"known": true}

	cleaned := gcLogs(logsDir, known, false)
	if cleaned != 1 {
		t.Errorf("expected 1 orphan log cleaned, got %d", cleaned)
	}
}

func TestGcSnapshots(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-gc-snap-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	snapsDir := filepath.Join(tmpDir, "snapshots")

	// Valid snapshot for known overlay
	validDir := filepath.Join(snapsDir, "valid-snap")
	os.MkdirAll(validDir, 0755)
	meta, _ := json.Marshal(map[string]string{"overlay": "known"})
	os.WriteFile(filepath.Join(validDir, "meta.json"), meta, 0644)

	// Orphan snapshot (overlay gone)
	orphanDir := filepath.Join(snapsDir, "orphan-snap")
	os.MkdirAll(orphanDir, 0755)
	meta2, _ := json.Marshal(map[string]string{"overlay": "deleted-overlay"})
	os.WriteFile(filepath.Join(orphanDir, "meta.json"), meta2, 0644)

	// Broken snapshot (no meta.json)
	brokenDir := filepath.Join(snapsDir, "broken-snap")
	os.MkdirAll(brokenDir, 0755)

	known := map[string]bool{"known": true}

	cleaned := gcSnapshots(snapsDir, known, false)
	if cleaned != 2 {
		t.Errorf("expected 2 cleaned (orphan + broken), got %d", cleaned)
	}

	// Valid should remain
	if _, err := os.Stat(validDir); os.IsNotExist(err) {
		t.Error("valid snapshot should not be removed")
	}
}

func TestGcSnapshots_DryRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-gc-snap-dry-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	snapsDir := filepath.Join(tmpDir, "snapshots")
	brokenDir := filepath.Join(snapsDir, "broken")
	os.MkdirAll(brokenDir, 0755)

	cleaned := gcSnapshots(snapsDir, map[string]bool{}, true)
	if cleaned != 1 {
		t.Errorf("expected 1 in dry-run, got %d", cleaned)
	}
	if _, err := os.Stat(brokenDir); os.IsNotExist(err) {
		t.Error("dry-run should not remove files")
	}
}
