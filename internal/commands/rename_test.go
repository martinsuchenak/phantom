package commands

import "testing"

func TestNewRenameCommand(t *testing.T) {
	cmd := NewRenameCommand()
	if cmd.Name != "rename" {
		t.Errorf("expected command name 'rename', got %q", cmd.Name)
	}
	if len(cmd.Arguments) != 2 {
		t.Errorf("expected 2 arguments, got %d", len(cmd.Arguments))
	}
}
