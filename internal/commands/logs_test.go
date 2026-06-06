package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessLogs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-logs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	setupTestEnv(t, tmpDir)

	// Create logs dir and a log file
	logsDir := filepath.Join(tmpDir, "state", "logs")
	_ = os.MkdirAll(logsDir, 0755)
	_ = os.WriteFile(filepath.Join(logsDir, "test-overlay.log"), []byte("line1\nline2\nline3\n"), 0644)

	// Test reading full log
	err = processLogs("test-overlay", 0, false)
	if err != nil {
		t.Errorf("processLogs failed: %v", err)
	}

	// Test showing path
	err = processLogs("test-overlay", 0, true)
	if err != nil {
		t.Errorf("processLogs --path failed: %v", err)
	}

	// Test tail
	err = processLogs("test-overlay", 10, false)
	if err != nil {
		t.Errorf("processLogs --tail failed: %v", err)
	}

	// Test non-existent overlay
	err = processLogs("nonexistent", 0, false)
	if err == nil {
		t.Error("expected error for non-existent overlay logs")
	}
}
