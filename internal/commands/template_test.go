package commands

import "testing"

func TestFindTemplate(t *testing.T) {
	tests := []struct {
		name  string
		found bool
	}{
		{"claude", true},
		{"gemini", true},
		{"aider", true},
		{"vibe", true},
		{"copilot", true},
		{"codex", true},
		{"nonexistent", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := findTemplate(tt.name)
			if tt.found && tmpl == nil {
				t.Errorf("expected to find template %q", tt.name)
			}
			if !tt.found && tmpl != nil {
				t.Errorf("expected template %q to not be found", tt.name)
			}
		})
	}
}

func TestParseInlineAgentNames(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"claude,aider,gemini", 3},
		{"claude", 1},
		{"", 0},
		{",,,", 0},
		{"claude, aider , gemini", 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseInlineAgentNames(tt.input)
			if len(got) != tt.expected {
				t.Errorf("parseInlineAgentNames(%q) = %v (len %d), want len %d", tt.input, got, len(got), tt.expected)
			}
		})
	}
}

func TestSplitAndTrim(t *testing.T) {
	tests := []struct {
		input    string
		sep      string
		expected []string
	}{
		{"a,b,c", ",", []string{"a", "b", "c"}},
		{" a , b , c ", ",", []string{"a", "b", "c"}},
		{"", ",", nil},
		{"  ,  ,  ", ",", nil},
		{"hello", ",", []string{"hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitAndTrim(tt.input, tt.sep)
			if len(got) != len(tt.expected) {
				t.Errorf("splitAndTrim(%q, %q) = %v, want %v", tt.input, tt.sep, got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitAndTrim(%q, %q)[%d] = %q, want %q", tt.input, tt.sep, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestSplitString(t *testing.T) {
	got := splitString("a:b:c", ":")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("splitString failed: %v", got)
	}
}

func TestIndexOf(t *testing.T) {
	if indexOf("hello world", "world") != 6 {
		t.Error("expected index 6")
	}
	if indexOf("hello", "xyz") != -1 {
		t.Error("expected -1 for not found")
	}
}

func TestTrimSpace(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"  hello  ", "hello"},
		{"\thello\t", "hello"},
		{"hello", "hello"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		got := trimSpace(tt.input)
		if got != tt.expected {
			t.Errorf("trimSpace(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
