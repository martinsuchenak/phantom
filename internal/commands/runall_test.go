package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseInlineAgents(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"echo hello,echo world", 2},
		{"echo hello", 1},
		{"", 0},
		{",,,", 0},
		{" echo a , echo b ", 2},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseInlineAgents(tt.input)
			if len(got) != tt.expected {
				t.Errorf("parseInlineAgents(%q) len = %d, want %d", tt.input, len(got), tt.expected)
			}
		})
	}
}

func TestParseInlineAgents_Names(t *testing.T) {
	agents := parseInlineAgents("echo hello,echo world")
	if agents[0].Name != "agent-1" {
		t.Errorf("expected name 'agent-1', got %q", agents[0].Name)
	}
	if agents[1].Name != "agent-2" {
		t.Errorf("expected name 'agent-2', got %q", agents[1].Name)
	}
	if agents[0].Agent != "echo hello" {
		t.Errorf("expected agent 'echo hello', got %q", agents[0].Agent)
	}
}

func TestLoadAgentsConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-runall-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	yamlContent := `agents:
  - name: test-agent
    agent: "echo hello"
    task: "do something"
    branch: "feature/test"
    timeout: 30
  - agent: "echo world"
    task: "do other"
`
	configPath := filepath.Join(tmpDir, "agents.yaml")
	_ = os.WriteFile(configPath, []byte(yamlContent), 0644)

	agents, err := loadAgentsConfig(configPath)
	if err != nil {
		t.Fatalf("loadAgentsConfig failed: %v", err)
	}

	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	if agents[0].Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", agents[0].Name)
	}
	if agents[0].Timeout != 30 {
		t.Errorf("expected timeout 30, got %d", agents[0].Timeout)
	}
	// Second agent should get auto-generated name
	if agents[1].Name != "agent-2" {
		t.Errorf("expected auto-name 'agent-2', got %q", agents[1].Name)
	}
}

func TestLoadAgentsConfig_MissingAgent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-runall-invalid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	yamlContent := `agents:
  - name: bad-agent
    task: "no agent command"
`
	configPath := filepath.Join(tmpDir, "agents.yaml")
	_ = os.WriteFile(configPath, []byte(yamlContent), 0644)

	_, err = loadAgentsConfig(configPath)
	if err == nil {
		t.Error("expected error for missing agent command")
	}
}

func TestLoadAgentsConfig_NonExistent(t *testing.T) {
	_, err := loadAgentsConfig("/nonexistent/agents.yaml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadAgentsConfig_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-runall-badyaml-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	configPath := filepath.Join(tmpDir, "agents.yaml")
	_ = os.WriteFile(configPath, []byte("{{invalid yaml"), 0644)

	_, err = loadAgentsConfig(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestFilterAgents(t *testing.T) {
	agents := []agentDef{
		{Name: "agent1"},
		{Name: "agent2"},
		{Name: "agent3"},
	}

	// Test no filters
	filtered, err := filterAgents(agents, "", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 agents, got %d", len(filtered))
	}

	// Test --only
	filtered, err = filterAgents(agents, "agent2", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Name != "agent2" {
		t.Errorf("expected only agent2, got %v", filtered)
	}

	// Test --from
	filtered, err = filterAgents(agents, "", "agent2")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(filtered) != 2 || filtered[0].Name != "agent2" {
		t.Errorf("expected starting from agent2, got %v", filtered)
	}

	// Test missing name
	_, err = filterAgents(agents, "missing", "")
	if err == nil {
		t.Errorf("expected error for missing agent in --only")
	}

	_, err = filterAgents(agents, "", "missing")
	if err == nil {
		t.Errorf("expected error for missing agent in --from")
	}

	// Test both flags
	_, err = filterAgents(agents, "agent1", "agent2")
	if err == nil {
		t.Errorf("expected error for both flags")
	}
}
