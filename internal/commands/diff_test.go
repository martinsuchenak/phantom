package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDiffCommand(t *testing.T) {
	cmd := NewDiffCommand()
	if cmd.Name != "diff" {
		t.Errorf("expected command name 'diff', got %q", cmd.Name)
	}
}

func TestProcessDiff(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.MkdirAll(upperDir, 0755)

	// Create base files
	_ = os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("original"), 0644)

	// Create upper files (changes)
	_ = os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("modified"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, "new-file.txt"), []byte("added"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, ".wh.deleted.txt"), []byte{}, 0644)

	ovl := testOverlay("test-diff", baseDir, filepath.Join(tmpDir, "mnt"), upperDir)
	_ = store.Save(&ovl)

	t.Run("table format", func(t *testing.T) {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := processDiff("test-diff", "table", false)

		_ = w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		if err != nil {
			t.Fatalf("processDiff failed: %v", err)
		}
		if !strings.Contains(output, "existing.txt") {
			t.Errorf("expected 'existing.txt' in output")
		}
		if !strings.Contains(output, "new-file.txt") {
			t.Errorf("expected 'new-file.txt' in output")
		}
	})

	t.Run("json format", func(t *testing.T) {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := processDiff("test-diff", "json", false)

		_ = w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		if err != nil {
			t.Fatalf("processDiff json failed: %v", err)
		}
		if !strings.Contains(output, `"status"`) {
			t.Errorf("expected JSON with status field")
		}
	})

	t.Run("simple format", func(t *testing.T) {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := processDiff("test-diff", "simple", false)

		_ = w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		output := buf.String()

		if err != nil {
			t.Fatalf("processDiff simple failed: %v", err)
		}
		if !strings.Contains(output, "A\t") || !strings.Contains(output, "M\t") || !strings.Contains(output, "D\t") {
			t.Errorf("expected A/M/D prefixes in simple output, got %q", output)
		}
	})

	t.Run("stat only", func(t *testing.T) {
		err := processDiff("test-diff", "table", true)
		if err != nil {
			t.Fatalf("processDiff stat failed: %v", err)
		}
	})
}

func TestProcessDiffNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	createTestStore(t, tmpDir)

	err := processDiff("nonexistent", "table", false)
	if err == nil {
		t.Error("expected error for nonexistent overlay")
	}
}

func TestProcessDiffNoUpperDir(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	ovl := testOverlay("no-upper", tmpDir, tmpDir, "")
	_ = store.Save(&ovl)

	err := processDiff("no-upper", "table", false)
	if err == nil {
		t.Error("expected error for overlay with no upper dir")
	}
}

func TestProcessDiffEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.MkdirAll(upperDir, 0755)

	ovl := testOverlay("empty-diff", baseDir, filepath.Join(tmpDir, "mnt"), upperDir)
	_ = store.Save(&ovl)

	err := processDiff("empty-diff", "table", false)
	if err != nil {
		t.Fatalf("processDiff on empty overlay failed: %v", err)
	}
}

func TestCountFileChanges(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(baseDir, 0755)
	_ = os.MkdirAll(upperDir, 0755)

	// Base has existing file
	_ = os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)

	// Upper has: modified existing, new file, whiteout
	_ = os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("new"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, "added.txt"), []byte("new"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, ".wh.removed.txt"), []byte{}, 0644)

	a, m, d := countFileChanges(upperDir, baseDir)
	if a != 1 {
		t.Errorf("expected 1 added, got %d", a)
	}
	if m != 1 {
		t.Errorf("expected 1 modified, got %d", m)
	}
	if d != 1 {
		t.Errorf("expected 1 deleted, got %d", d)
	}
}

func TestCountFileChangesEmpty(t *testing.T) {
	a, m, d := countFileChanges("", "")
	if a != 0 || m != 0 || d != 0 {
		t.Errorf("expected all zeros for empty upper dir, got %d/%d/%d", a, m, d)
	}
}

func TestPrintDiffStat(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	result := diffResult{
		Name:     "test",
		Added:    3,
		Modified: 2,
		Deleted:  1,
	}
	err := printDiffStat(result)
	if err != nil {
		t.Errorf("printDiffStat failed: %v", err)
	}
}
