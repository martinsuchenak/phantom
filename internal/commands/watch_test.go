package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewWatchCommand(t *testing.T) {
	cmd := NewWatchCommand()
	if cmd.Name != "watch" {
		t.Errorf("expected command name 'watch', got %q", cmd.Name)
	}
}

func TestScanUpperDir(t *testing.T) {
	tmpDir := t.TempDir()
	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	_ = os.MkdirAll(upperDir, 0755)
	_ = os.MkdirAll(baseDir, 0755)

	// Create some files in upper
	_ = os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("new"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, "modified.txt"), []byte("changed"), 0644)
	// Create a whiteout
	_ = os.WriteFile(filepath.Join(upperDir, ".wh.deleted.txt"), []byte{}, 0644)

	// Create matching file in base (for "modified" detection)
	_ = os.WriteFile(filepath.Join(baseDir, "modified.txt"), []byte("original"), 0644)

	out := make(map[string]fileSnapshot)
	scanUpperDir(upperDir, baseDir, out, nil, "")

	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out))
	}

	if _, ok := out["new.txt"]; !ok {
		t.Error("expected new.txt in scan results")
	}
	if _, ok := out["modified.txt"]; !ok {
		t.Error("expected modified.txt in scan results")
	}
	if snap, ok := out["deleted.txt"]; !ok {
		t.Error("expected deleted.txt (whiteout) in scan results")
	} else if !snap.IsWhiteout {
		t.Error("expected deleted.txt to be marked as whiteout")
	}
}

func TestScanUpperDirSkipsWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(filepath.Join(upperDir, "work"), 0755)
	_ = os.WriteFile(filepath.Join(upperDir, "work", "internal.txt"), []byte("skip"), 0644)
	_ = os.WriteFile(filepath.Join(upperDir, "real.txt"), []byte("keep"), 0644)

	out := make(map[string]fileSnapshot)
	scanUpperDir(upperDir, tmpDir, out, nil, "")

	if len(out) != 1 {
		t.Errorf("expected 1 entry (skipping work/), got %d", len(out))
	}
	if _, ok := out["real.txt"]; !ok {
		t.Error("expected real.txt in results")
	}
}

func TestFileExistsInBase(t *testing.T) {
	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "exists.txt"), []byte("hi"), 0644)

	if !fileExistsInBase(tmpDir, "exists.txt") {
		t.Error("expected file to exist in base")
	}
	if fileExistsInBase(tmpDir, "nope.txt") {
		t.Error("expected file to not exist in base")
	}
}

func TestPollWatchCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	upperDir := filepath.Join(tmpDir, "upper")
	_ = os.MkdirAll(upperDir, 0755)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := pollWatch(ctx, upperDir, tmpDir, 50*time.Millisecond, "simple")
	if err != nil {
		t.Errorf("expected nil error on context cancellation, got %v", err)
	}
}
