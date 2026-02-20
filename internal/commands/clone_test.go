package commands

import "testing"

func TestNewCloneCommand(t *testing.T) {
	cmd := NewCloneCommand()
	if cmd.Name != "clone" {
		t.Errorf("expected command name 'clone', got %q", cmd.Name)
	}
	if len(cmd.Arguments) != 2 {
		t.Errorf("expected 2 arguments, got %d", len(cmd.Arguments))
	}
}
