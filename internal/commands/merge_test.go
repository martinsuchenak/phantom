package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanMerge_NoChanges(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(actions))
	}
}

func TestPlanMerge_CopyFiles(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	// Create a file in source
	_ = os.WriteFile(filepath.Join(srcUpper, "new.txt"), []byte("hello"), 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Status != "copy" {
		t.Errorf("expected copy, got %s", actions[0].Status)
	}
	if actions[0].IsConflict {
		t.Error("should not be a conflict")
	}
}

func TestPlanMerge_Conflict(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	// Same file in both
	_ = os.WriteFile(filepath.Join(srcUpper, "shared.txt"), []byte("src"), 0644)
	_ = os.WriteFile(filepath.Join(dstUpper, "shared.txt"), []byte("dst"), 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if !actions[0].IsConflict {
		t.Error("should be a conflict")
	}
}

func TestPlanMerge_Whiteout(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	// Create a whiteout file (deletion marker)
	_ = os.WriteFile(filepath.Join(srcUpper, ".wh.deleted.txt"), []byte{}, 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	if actions[0].Status != "delete" {
		t.Errorf("expected delete, got %s", actions[0].Status)
	}
	if actions[0].RelPath != "deleted.txt" {
		t.Errorf("expected 'deleted.txt', got %q", actions[0].RelPath)
	}
}

func TestPlanMerge_SkipsWorkDir(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	// Create work directory (should be skipped)
	_ = os.MkdirAll(filepath.Join(srcUpper, "work"), 0755)
	_ = os.WriteFile(filepath.Join(srcUpper, "work", "internal.txt"), []byte("skip"), 0644)
	// Also a real file
	_ = os.WriteFile(filepath.Join(srcUpper, "real.txt"), []byte("keep"), 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action (skipping work/), got %d", len(actions))
	}
	if actions[0].RelPath != "real.txt" {
		t.Errorf("expected 'real.txt', got %q", actions[0].RelPath)
	}
}

func TestPrintMergePlan(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	actions := []mergeAction{
		{RelPath: "added.txt", Status: "copy", IsConflict: false},
		{RelPath: "conflict.txt", Status: "copy", IsConflict: true},
		{RelPath: "removed.txt", Status: "delete", IsConflict: false},
	}

	err := printMergePlan(actions, "src", "dst", 1)
	if err != nil {
		t.Fatalf("printMergePlan failed: %v", err)
	}
}

func TestPlanMerge_NestedFiles(t *testing.T) {
	srcUpper := t.TempDir()
	dstUpper := t.TempDir()

	_ = os.MkdirAll(filepath.Join(srcUpper, "sub", "deep"), 0755)
	_ = os.WriteFile(filepath.Join(srcUpper, "sub", "deep", "file.go"), []byte("package main"), 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("planMerge failed: %v", err)
	}
	if len(actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(actions))
	}
	expected := filepath.Join("sub", "deep", "file.go")
	if actions[0].RelPath != expected {
		t.Errorf("expected %q, got %q", expected, actions[0].RelPath)
	}
}
