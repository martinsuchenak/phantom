package commands

import "testing"

func TestValidateBranchName(t *testing.T) {
	tests := []struct {
		name    string
		branch  string
		wantErr bool
	}{
		{"valid simple", "feature/test", false},
		{"valid with prefix", "phantom/my-overlay", false},
		{"starts with dash", "-bad", true},
		{"contains double dot", "feature..test", true},
		{"contains tilde", "feature~test", true},
		{"contains caret", "feature^test", true},
		{"contains colon", "feature:test", true},
		{"contains question", "feature?test", true},
		{"contains star", "feature*test", true},
		{"contains bracket", "feature[test", true},
		{"contains backslash", "feature\\test", true},
		{"contains space", "feature test", true},
		{"contains tab", "feature\ttest", true},
		{"contains newline", "feature\ntest", true},
		{"ends with .lock", "feature.lock", true},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBranchName(tt.branch)
			if tt.wantErr && err == nil {
				t.Errorf("validateBranchName(%q) expected error", tt.branch)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateBranchName(%q) unexpected error: %v", tt.branch, err)
			}
		})
	}
}
