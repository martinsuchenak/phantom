package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFuseAvailable(t *testing.T) {
	// Just verify it doesn't panic — result depends on system
	_ = checkFuseAvailable()
}

func TestIsProcessAlive(t *testing.T) {
	// isProcessAlive sends signal 0 (nil) which may not work as expected on all systems
	// Just verify it doesn't panic
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Skip("cannot find own process")
	}
	_ = isProcessAlive(proc)
}

func TestNewHealthCommand(t *testing.T) {
	cmd := NewHealthCommand()
	if cmd.Name != "health" {
		t.Errorf("expected command name 'health', got %q", cmd.Name)
	}
}

func TestPrintHealthReport_Table(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	report := healthReport{
		Platform: "darwin",
		FuseOK:   true,
		Overlays: 2,
		Healthy:  1,
		Issues: []healthIssue{
			{Overlay: "test", Kind: "stale_mount", Message: "not mounted"},
		},
	}

	err := printHealthReport(report, "table")
	if err != nil {
		t.Errorf("printHealthReport table failed: %v", err)
	}
}

func TestPrintHealthReport_JSON(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	report := healthReport{
		Platform: "darwin",
		FuseOK:   true,
		Overlays: 1,
		Healthy:  1,
	}

	err := printHealthReport(report, "json")
	if err != nil {
		t.Errorf("printHealthReport json failed: %v", err)
	}
}

func TestPrintHealthReport_NoIssues(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	report := healthReport{
		Platform: "darwin",
		FuseOK:   false,
		Overlays: 0,
		Healthy:  0,
	}

	err := printHealthReport(report, "table")
	if err != nil {
		t.Errorf("printHealthReport no issues failed: %v", err)
	}
}

func TestDoHealth(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	cmd := NewHealthCommand()

	// mock basic setup
	os.MkdirAll(filepath.Join(tmpDir, "state"), 0755)

	runCommandWithArgs(t, []string{"health"}, func() {
		err := doHealth(context.Background(), cmd)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	runCommandWithArgs(t, []string{"health", "--format", "json", "--fix"}, func() {
		err := doHealth(context.Background(), cmd)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
