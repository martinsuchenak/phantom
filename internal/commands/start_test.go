package commands

import "testing"

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"valid simple", "feature/auth", false},
		{"valid with prefix", "phantom/my-overlay", false},
		{"starts with dash", "-bad", true},
		{"contains double dot", "feature..bad", true},
		{"contains tilde", "feature~1", true},
		{"contains caret", "feature^2", true},
		{"contains colon", "feature:bad", true},
		{"contains question", "feature?bad", true},
		{"contains star", "feature*bad", true},
		{"contains bracket", "feature[bad", true},
		{"contains backslash", "feature\\bad", true},
		{"contains space", "feature bad", true},
		{"contains tab", "feature\tbad", true},
		{"contains newline", "feature\nbad", true},
		{"ends with .lock", "feature.lock", true},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBranchName(%q) error = %v, wantErr %v", tt.branch, err, tt.wantErr)
			}
		})
	}
}

func TestNewStartCommand(t *testing.T) {
	cmd := NewStartCommand()
	if cmd.Name != "start" {
		t.Errorf("expected command name 'start', got %q", cmd.Name)
	}
}
