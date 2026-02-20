package commands

import "testing"

func TestNewInspectCommand(t *testing.T) {
	cmd := NewInspectCommand()
	if cmd.Name != "inspect" {
		t.Errorf("expected command name 'inspect', got %q", cmd.Name)
	}
}
