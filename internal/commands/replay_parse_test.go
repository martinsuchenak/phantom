package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLastRun_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "state", "logs")
	os.MkdirAll(logsDir, 0755)
	cfg.Paths.Logs = logsDir

	logContent := `=== Phantom Agent Log ===
Overlay:  test-overlay
Agent:    claude --print
Task:     fix the bug
Started:  2025-01-15 10:30:00
=========================
some output here
`
	os.WriteFile(filepath.Join(logsDir, "test-overlay.log"), []byte(logContent), 0644)

	info, err := parseLastRun("test-overlay")
	if err != nil {
		t.Fatalf("parseLastRun failed: %v", err)
	}
	if info.Agent != "claude --print" {
		t.Errorf("expected 'claude --print', got %q", info.Agent)
	}
	if info.Task != "fix the bug" {
		t.Errorf("expected 'fix the bug', got %q", info.Task)
	}
}

func TestParseLastRun_MultipleRuns(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "state", "logs")
	os.MkdirAll(logsDir, 0755)
	cfg.Paths.Logs = logsDir

	logContent := `=== Phantom Agent Log ===
Agent:    first-agent
Task:     first task
=========================
output 1
=== Phantom Agent Log ===
Agent:    second-agent
Task:     second task
=========================
output 2
`
	os.WriteFile(filepath.Join(logsDir, "multi.log"), []byte(logContent), 0644)

	info, err := parseLastRun("multi")
	if err != nil {
		t.Fatalf("parseLastRun failed: %v", err)
	}
	// Should return the LAST agent/task
	if info.Agent != "second-agent" {
		t.Errorf("expected 'second-agent', got %q", info.Agent)
	}
	if info.Task != "second task" {
		t.Errorf("expected 'second task', got %q", info.Task)
	}
}

func TestParseLastRun_NoLogFile(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "state", "logs")
	os.MkdirAll(logsDir, 0755)
	cfg.Paths.Logs = logsDir

	_, err := parseLastRun("nonexistent")
	if err == nil {
		t.Error("expected error for missing log file")
	}
}

func TestParseLastRun_EmptyLog(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := filepath.Join(tmpDir, "state", "logs")
	os.MkdirAll(logsDir, 0755)
	cfg.Paths.Logs = logsDir

	os.WriteFile(filepath.Join(logsDir, "empty.log"), []byte(""), 0644)

	info, err := parseLastRun("empty")
	if err != nil {
		t.Fatalf("parseLastRun failed: %v", err)
	}
	if info.Agent != "" {
		t.Errorf("expected empty agent, got %q", info.Agent)
	}
}
