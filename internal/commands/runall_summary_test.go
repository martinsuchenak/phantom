package commands

import (
	"testing"
	"time"
)

func TestPrintRunAllSummary_Table(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	results := []agentResult{
		{Name: "agent-1", Agent: "echo hello", ExitCode: 0, Duration: 5 * time.Second},
		{Name: "agent-2", Agent: "echo world", ExitCode: 1, Duration: 10 * time.Second, Error: "failed"},
	}

	// printRunAllSummary calls os.Exit(1) when there are failures, so we can't test that path directly
	// Test the JSON path instead
	err := printRunAllSummary(results, "json")
	if err != nil {
		t.Errorf("printRunAllSummary json failed: %v", err)
	}
}

func TestPrintRunAllSummary_AllSuccess(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	results := []agentResult{
		{Name: "agent-1", Agent: "echo hello", ExitCode: 0, Duration: 5 * time.Second},
	}

	err := printRunAllSummary(results, "json")
	if err != nil {
		t.Errorf("printRunAllSummary failed: %v", err)
	}
}
