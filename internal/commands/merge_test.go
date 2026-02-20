package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewMergeCommand(t *testing.T) {
	cmd := NewMergeCommand()
	if cmd.Name != "merge" {
		t.Errorf("expected command name 'merge', got %q", cmd.Name)
	}
}

func TestPlanMerge(t *testing.T) {
	tmpDir := t.TempDir()
	srcUpper := filepath.Join(tmpDir, "src-upper")
	dstUpper := filepath.Join(tmpDir, "dst-upper")
	os.MkdirAll(srcUpper, 0755)
	os.MkdirAll(dstUpper, 0755)

	t.Run("no conflicts", func(t *testing.T) {
		os.WriteFile(filepath.Join(srcUpper, "a.txt"), []byte("a"), 0644)

		actions, err := planMerge(srcUpper, dstUpper)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(actions) != 1 {
			t.Fatalf("expected 1 action, got %d", len(actions))
		}
		if actions[0].RelPath != "a.txt" || actions[0].Status != "copy" || actions[0].IsConflict {
			t.Errorf("unexpected action: %+v", actions[0])
		}
	})

	t.Run("with conflict", func(t *testing.T) {
		// Both overlays have the same file
		os.WriteFile(filepath.Join(srcUpper, "shared.txt"), []byte("src"), 0644)
		os.WriteFile(filepath.Join(dstUpper, "shared.txt"), []byte("dst"), 0644)

		actions, err := planMerge(srcUpper, dstUpper)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		foundConflict := false
		for _, a := range actions {
			if a.RelPath == "shared.txt" && a.IsConflict {
				foundConflict = true
			}
		}
		if !foundConflict {
			t.Error("expected conflict for shared.txt")
		}
	})

	t.Run("whiteout files", func(t *testing.T) {
		os.WriteFile(filepath.Join(srcUpper, ".wh.removed.txt"), []byte{}, 0644)

		actions, err := planMerge(srcUpper, dstUpper)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		foundDelete := false
		for _, a := range actions {
			if a.RelPath == "removed.txt" && a.Status == "delete" {
				foundDelete = true
			}
		}
		if !foundDelete {
			t.Error("expected delete action for whiteout file")
		}
	})
}

func TestPlanMergeEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	srcUpper := filepath.Join(tmpDir, "src-empty")
	dstUpper := filepath.Join(tmpDir, "dst-empty")
	os.MkdirAll(srcUpper, 0755)
	os.MkdirAll(dstUpper, 0755)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 0 {
		t.Errorf("expected 0 actions for empty dirs, got %d", len(actions))
	}
}

func TestPlanMergeSkipsWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcUpper := filepath.Join(tmpDir, "src")
	dstUpper := filepath.Join(tmpDir, "dst")
	os.MkdirAll(filepath.Join(srcUpper, "work"), 0755)
	os.MkdirAll(dstUpper, 0755)
	os.WriteFile(filepath.Join(srcUpper, "work", "internal.txt"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(srcUpper, "real.txt"), []byte("keep"), 0644)

	actions, err := planMerge(srcUpper, dstUpper)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(actions) != 1 {
		t.Errorf("expected 1 action (skipping work/), got %d", len(actions))
	}
}

func TestPrintMergePlan(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	actions := []mergeAction{
		{RelPath: "new.txt", Status: "copy"},
		{RelPath: "conflict.txt", Status: "copy", IsConflict: true},
		{RelPath: "removed.txt", Status: "delete"},
	}

	err := printMergePlan(actions, "src", "dst", 1)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
