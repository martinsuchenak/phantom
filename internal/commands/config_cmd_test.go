package commands

import "testing"

func TestNewConfigCommand(t *testing.T) {
	cmd := NewConfigCommand()
	if cmd.Name != "config" {
		t.Errorf("expected command name 'config', got %q", cmd.Name)
	}
	if len(cmd.Commands) != 2 {
		t.Errorf("expected 2 subcommands, got %d", len(cmd.Commands))
	}
}

func TestDoConfigShow(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	// doConfigShow just prints, verify it doesn't panic
	err := doConfigShow(nil, nil)
	if err != nil {
		t.Errorf("doConfigShow failed: %v", err)
	}
}
