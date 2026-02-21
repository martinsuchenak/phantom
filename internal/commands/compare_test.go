package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewCompareCommand(t *testing.T) {
	cmd := NewCompareCommand()
	if cmd.Name != "compare" {
		t.Errorf("expected command name 'compare', got %q", cmd.Name)
	}
}

func TestScanChanges(t *testing.T) {
	tmpDir := t.TempDir()
	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(baseDir, 0755)

	// New file (not in base)
	os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("new"), 0644)
	// Modified file (exists in base)
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("changed"), 0644)
	// Deleted file (whiteout)
	os.WriteFile(filepath.Join(upperDir, ".wh.removed.txt"), []byte{}, 0644)

	changes := scanChanges(upperDir, baseDir)

	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d", len(changes))
	}
	if changes["new.txt"].Status != "added" {
		t.Errorf("expected new.txt to be 'added', got %q", changes["new.txt"].Status)
	}
	if changes["existing.txt"].Status != "modified" {
		t.Errorf("expected existing.txt to be 'modified', got %q", changes["existing.txt"].Status)
	}
	if changes["removed.txt"].Status != "deleted" {
		t.Errorf("expected removed.txt to be 'deleted', got %q", changes["removed.txt"].Status)
	}
}

func TestScanChangesSkipsWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(filepath.Join(upperDir, "work"), 0755)
	os.WriteFile(filepath.Join(upperDir, "work", "skip.txt"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(upperDir, "keep.txt"), []byte("keep"), 0644)

	changes := scanChanges(upperDir, tmpDir)
	if len(changes) != 1 {
		t.Errorf("expected 1 change (skipping work/), got %d", len(changes))
	}
}

func TestBuildComparison(t *testing.T) {
	changesA := map[string]fileChange{
		"shared.txt": {Status: "modified", Size: 100},
		"only-a.txt": {Status: "added", Size: 50},
	}
	changesB := map[string]fileChange{
		"shared.txt": {Status: "modified", Size: 200},
		"only-b.txt": {Status: "added", Size: 75},
	}

	result := buildComparison("ovl-a", "ovl-b", changesA, changesB)

	if result.OverlayA != "ovl-a" || result.OverlayB != "ovl-b" {
		t.Errorf("unexpected overlay names: %s, %s", result.OverlayA, result.OverlayB)
	}
	if result.OnlyA != 1 {
		t.Errorf("expected 1 only-A, got %d", result.OnlyA)
	}
	if result.OnlyB != 1 {
		t.Errorf("expected 1 only-B, got %d", result.OnlyB)
	}
	if result.Both != 1 {
		t.Errorf("expected 1 both, got %d", result.Both)
	}
	if len(result.Files) != 3 {
		t.Errorf("expected 3 files, got %d", len(result.Files))
	}

	// Check that shared file is marked as both
	for _, f := range result.Files {
		if f.File == "shared.txt" {
			if !f.Both {
				t.Error("expected shared.txt to be marked as both")
			}
			if f.StatusA != "modified" || f.StatusB != "modified" {
				t.Errorf("unexpected statuses: %s, %s", f.StatusA, f.StatusB)
			}
		}
	}
}

func TestBuildComparisonEmpty(t *testing.T) {
	result := buildComparison("a", "b", map[string]fileChange{}, map[string]fileChange{})
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestPrintCompareTable(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	result := compareResult{
		OverlayA: "ovl-a",
		OverlayB: "ovl-b",
		Files: []compareEntry{
			{File: "shared.txt", StatusA: "modified", StatusB: "modified", SizeA: 100, SizeB: 200, Both: true},
			{File: "only-a.txt", StatusA: "added", SizeA: 50},
			{File: "only-b.txt", StatusB: "deleted"},
		},
		OnlyA: 1,
		OnlyB: 1,
		Both:  1,
	}

	if err := printCompareTable(result); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrintCompareTableEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	result := compareResult{OverlayA: "a", OverlayB: "b"}
	if err := printCompareTable(result); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
