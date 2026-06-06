package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewReplayCommand(t *testing.T) {
	cmd := NewReplayCommand()
	if cmd.Name != "replay" {
		t.Errorf("expected command name 'replay', got %q", cmd.Name)
	}
}

func TestParseLastRun(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	logsDir := cfg.GetLogsPath()
	_ = os.MkdirAll(logsDir, 0700)

	t.Run("parses agent and task from log", func(t *testing.T) {
		logContent := `=== Phantom Agent Log ===
Overlay:  test-overlay
Agent:    claude --print
Task:     implement auth
Started:  2026-02-20T10:00:00Z
=========================

some output here

=========================
Finished: 2026-02-20T10:05:00Z
Duration: 5m0s
Exit:     0
=== Phantom Agent Log ===
Overlay:  test-overlay
Agent:    aider --message "fix tests"
Task:     fix failing tests
Started:  2026-02-20T11:00:00Z
=========================

more output
`
		_ = os.WriteFile(filepath.Join(logsDir, "test-overlay.log"), []byte(logContent), 0600)

		info, err := parseLastRun("test-overlay")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should get the LAST agent/task (aider, not claude)
		if info.Agent != `aider --message "fix tests"` {
			t.Errorf("expected last agent, got %q", info.Agent)
		}
		if info.Task != "fix failing tests" {
			t.Errorf("expected last task, got %q", info.Task)
		}
	})

	t.Run("no log file", func(t *testing.T) {
		_, err := parseLastRun("nonexistent")
		if err == nil {
			t.Error("expected error for missing log")
		}
	})

	t.Run("empty log", func(t *testing.T) {
		_ = os.WriteFile(filepath.Join(logsDir, "empty.log"), []byte("just some output\n"), 0600)

		info, err := parseLastRun("empty")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Agent != "" {
			t.Errorf("expected empty agent, got %q", info.Agent)
		}
	})
}
