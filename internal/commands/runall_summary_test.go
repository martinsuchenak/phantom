package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPrintRunAllSummary_Table(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "agent-1", Agent: "echo hello", ExitCode: 0, Duration: 5 * time.Second},
		{Name: "agent-2", Agent: "echo world", ExitCode: 0, Duration: 10 * time.Second},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printRunAllSummary(results, "table")

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "agent-1") {
		t.Errorf("expected 'agent-1' in output, got %q", output)
	}
	if !strings.Contains(output, "agent-2") {
		t.Errorf("expected 'agent-2' in output, got %q", output)
	}
}

func TestPrintRunAllSummary_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "agent-1", Agent: "echo hello", ExitCode: 0, Duration: 5 * time.Second},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printRunAllSummary(results, "json")

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(output, `"name": "agent-1"`) {
		t.Errorf("expected JSON with agent-1, got %q", output)
	}
}

func TestPrintRunAllSummary_WithFailures(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	results := []agentResult{
		{Name: "agent-1", Agent: "echo hello", ExitCode: 0, Duration: 5 * time.Second},
		{Name: "agent-2", Agent: "false", ExitCode: 1, Duration: 1 * time.Second},
		{Name: "agent-3", Agent: "broken", ExitCode: 0, Duration: 2 * time.Second, Error: "command not found"},
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// printRunAllSummary calls os.Exit(1) on failures, so we test the output format
	// We can't easily test the exit, but we can verify the output
	_ = printRunAllSummary(results, "json")

	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"exit_code": 1`) {
		t.Errorf("expected exit_code 1 in JSON output, got %q", output)
	}
}

func TestGetConfigMode(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("parallel mode", func(t *testing.T) {
		path := tmpDir + "/parallel.yaml"
		_ = os.WriteFile(path, []byte("agents:\n  - agent: echo\n"), 0644)
		mode, _, _ := getConfigMode(path)
		if mode != "parallel" {
			t.Errorf("expected 'parallel', got %q", mode)
		}
	})

	t.Run("sequential mode", func(t *testing.T) {
		path := tmpDir + "/sequential.yaml"
		_ = os.WriteFile(path, []byte("mode: sequential\nname: chain\nbranch: feature/chain\nagents:\n  - agent: echo\n"), 0644)
		mode, name, branch := getConfigMode(path)
		if mode != "sequential" {
			t.Errorf("expected 'sequential', got %q", mode)
		}
		if name != "chain" {
			t.Errorf("expected name 'chain', got %q", name)
		}
		if branch != "feature/chain" {
			t.Errorf("expected branch 'feature/chain', got %q", branch)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		mode, _, _ := getConfigMode("/nonexistent/file.yaml")
		if mode != "parallel" {
			t.Errorf("expected 'parallel' for nonexistent file, got %q", mode)
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := tmpDir + "/invalid.yaml"
		_ = os.WriteFile(path, []byte("{{invalid"), 0644)
		mode, _, _ := getConfigMode(path)
		if mode != "parallel" {
			t.Errorf("expected 'parallel' for invalid yaml, got %q", mode)
		}
	})
}

func TestAgentsToChainSteps(t *testing.T) {
	agents := []agentDef{
		{Name: "a1", Agent: "echo hello", Task: "task1", Timeout: 30},
		{Name: "a2", Agent: "echo world", Task: "task2"},
	}

	steps := agentsToChainSteps(agents)
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Name != "a1" || steps[0].Agent != "echo hello" || steps[0].Task != "task1" || steps[0].Timeout != 30 {
		t.Errorf("step 0 mismatch: %+v", steps[0])
	}
	if steps[1].Name != "a2" || steps[1].Agent != "echo world" {
		t.Errorf("step 1 mismatch: %+v", steps[1])
	}
}
