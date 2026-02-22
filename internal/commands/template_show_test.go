package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDoTemplateListTable(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Directly test the output logic
	if len(builtinTemplates) == 0 {
		t.Fatal("expected builtin templates to be non-empty")
	}

	// Verify all templates have required fields
	for _, tmpl := range builtinTemplates {
		if tmpl.Name == "" {
			t.Error("template name should not be empty")
		}
		if tmpl.Agent == "" {
			t.Error("template agent should not be empty")
		}
		if tmpl.TaskMode == "" {
			t.Error("template task_mode should not be empty")
		}
	}

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
}

func TestDoTemplateShow(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Find claude template and verify its fields
	tmpl := findTemplate("claude")
	if tmpl == nil {
		t.Fatal("expected to find 'claude' template")
	}
	if !strings.HasPrefix(tmpl.Agent, "claude --print") {
		t.Errorf("expected agent to start with 'claude --print', got %q", tmpl.Agent)
	}
	if tmpl.TaskMode != "stdin" {
		t.Errorf("expected task_mode 'stdin', got %q", tmpl.TaskMode)
	}

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
}

func TestDoTemplateShowNotFound(t *testing.T) {
	tmpl := findTemplate("nonexistent-template")
	if tmpl != nil {
		t.Error("expected nil for nonexistent template")
	}
}

func TestDoTemplateGenerate(t *testing.T) {
	names := parseInlineAgentNames("claude,aider,gemini")
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}

	// Verify all names resolve to templates
	for _, name := range names {
		tmpl := findTemplate(name)
		if tmpl == nil {
			t.Errorf("template %q not found", name)
		}
	}
}

func TestDoTemplateGenerateOutput(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	names := parseInlineAgentNames("claude,aider")
	var agents []agentDef
	for _, name := range names {
		tmpl := findTemplate(name)
		if tmpl != nil {
			agents = append(agents, agentDef{
				Name:   tmpl.Name + "-agent",
				Agent:  tmpl.Agent,
				Task:   "TODO: describe your task",
				Branch: "feature/" + tmpl.Name,
			})
		}
	}

	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "claude-agent" {
		t.Errorf("expected 'claude-agent', got %q", agents[0].Name)
	}
	if !strings.Contains(agents[0].Agent, "claude") {
		t.Errorf("expected agent to contain 'claude', got %q", agents[0].Agent)
	}
}

func TestBuiltinTemplateCount(t *testing.T) {
	if len(builtinTemplates) < 6 {
		t.Errorf("expected at least 6 builtin templates, got %d", len(builtinTemplates))
	}
}
