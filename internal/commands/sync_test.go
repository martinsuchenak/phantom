package commands

import (
	"testing"
)

func TestNewSyncCommand(t *testing.T) {
	cmd := NewSyncCommand()
	if cmd.Name != "sync" {
		t.Errorf("expected command name 'sync', got %q", cmd.Name)
	}
	if len(cmd.Flags) != 2 {
		t.Errorf("expected 2 flags, got %d", len(cmd.Flags))
	}
	if len(cmd.Arguments) != 1 {
		t.Errorf("expected 1 argument, got %d", len(cmd.Arguments))
	}
}
