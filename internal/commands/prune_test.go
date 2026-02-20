package commands

import "testing"

func TestNewPruneCommand(t *testing.T) {
	cmd := NewPruneCommand()
	if cmd.Name != "prune" {
		t.Errorf("expected command name 'prune', got %q", cmd.Name)
	}
}

func TestNewLogsCommand(t *testing.T) {
	cmd := NewLogsCommand()
	if cmd.Name != "logs" {
		t.Errorf("expected command name 'logs', got %q", cmd.Name)
	}
}

func TestNewRestartCommand(t *testing.T) {
	cmd := NewRestartCommand()
	if cmd.Name != "restart" {
		t.Errorf("expected command name 'restart', got %q", cmd.Name)
	}
}

func TestNewCommitCommand(t *testing.T) {
	cmd := NewCommitCommand()
	if cmd.Name != "commit" {
		t.Errorf("expected command name 'commit', got %q", cmd.Name)
	}
}

func TestNewApplyCommand(t *testing.T) {
	cmd := NewApplyCommand()
	if cmd.Name != "apply" {
		t.Errorf("expected command name 'apply', got %q", cmd.Name)
	}
}

func TestNewRunCommand(t *testing.T) {
	cmd := NewRunCommand()
	if cmd.Name != "run" {
		t.Errorf("expected command name 'run', got %q", cmd.Name)
	}
}

func TestNewListCommand(t *testing.T) {
	cmd := NewListCommand()
	if cmd.Name != "list" {
		t.Errorf("expected command name 'list', got %q", cmd.Name)
	}
}

func TestNewStatusCommand(t *testing.T) {
	cmd := NewStatusCommand()
	if cmd.Name != "status" {
		t.Errorf("expected command name 'status', got %q", cmd.Name)
	}
}
