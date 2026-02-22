package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewExportCommand(t *testing.T) {
	cmd := NewExportCommand()
	if cmd.Name != "export" {
		t.Errorf("expected command name 'export', got %q", cmd.Name)
	}
}

func TestExportDiff(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Base has existing file
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old content"), 0644)

	// Upper has modified + new file
	os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("new content"), 0644)
	os.WriteFile(filepath.Join(upperDir, "added.txt"), []byte("brand new"), 0644)

	t.Run("stdout", func(t *testing.T) {
		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := exportDiff(upperDir, baseDir, "")

		w.Close()
		os.Stdout = old
		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		if err != nil {
			t.Fatalf("exportDiff failed: %v", err)
		}
		if !strings.Contains(output, "--- a/existing.txt") {
			t.Error("expected diff header for existing.txt")
		}
		if !strings.Contains(output, "+++ b/added.txt") {
			t.Error("expected diff header for added.txt")
		}
	})

	t.Run("to file", func(t *testing.T) {
		outPath := filepath.Join(tmpDir, "output.diff")
		err := exportDiff(upperDir, baseDir, outPath)
		if err != nil {
			t.Fatalf("exportDiff to file failed: %v", err)
		}

		data, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "---") {
			t.Error("expected diff content in output file")
		}
	})
}

func TestExportDiffWithDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Base has a file that was deleted (whiteout in upper)
	os.WriteFile(filepath.Join(baseDir, "removed.txt"), []byte("gone"), 0644)
	os.WriteFile(filepath.Join(upperDir, ".wh.removed.txt"), []byte{}, 0644)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := exportDiff(upperDir, baseDir, "")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("exportDiff with deletion failed: %v", err)
	}
	if !strings.Contains(output, "+++ /dev/null") {
		t.Error("expected deletion diff header")
	}
}

func TestExportTar(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644)

	outPath := filepath.Join(tmpDir, "export.tar")
	err := exportTar(upperDir, baseDir, outPath)
	if err != nil {
		t.Fatalf("exportTar failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty tar file")
	}
}

func TestExportTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(upperDir, 0755)
	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644)

	outPath := filepath.Join(tmpDir, "export.tar.gz")
	err := exportTar(upperDir, tmpDir, outPath)
	if err != nil {
		t.Fatalf("exportTar gz failed: %v", err)
	}

	info, _ := os.Stat(outPath)
	if info.Size() == 0 {
		t.Error("expected non-empty tar.gz file")
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},
		{"hello\nworld", 2},
		{"hello\nworld\n", 2},
		{"a\nb\nc\n", 3},
	}
	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.expected {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.expected)
		}
	}
}

func TestWriteUnifiedDiff_NewFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "new.txt", "", "line1\nline2\n")
	output := buf.String()

	if !strings.Contains(output, "--- /dev/null") {
		t.Error("expected /dev/null for new file")
	}
	if !strings.Contains(output, "+line1") {
		t.Error("expected +line1")
	}
}

func TestWriteUnifiedDiff_DeletedFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "old.txt", "line1\nline2\n", "")
	output := buf.String()

	if !strings.Contains(output, "+++ /dev/null") {
		t.Error("expected /dev/null for deleted file")
	}
	if !strings.Contains(output, "-line1") {
		t.Error("expected -line1")
	}
}

func TestWriteUnifiedDiff_ModifiedFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "mod.txt", "old\n", "new\n")
	output := buf.String()

	if !strings.Contains(output, "--- a/mod.txt") {
		t.Error("expected --- a/mod.txt")
	}
	if !strings.Contains(output, "+++ b/mod.txt") {
		t.Error("expected +++ b/mod.txt")
	}
}
