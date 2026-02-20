package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", nil},
		{"single line", "hello", []string{"hello"}},
		{"two lines", "hello\nworld", []string{"hello", "world"}},
		{"trailing newline", "hello\nworld\n", []string{"hello", "world"}},
		{"single with newline", "hello\n", []string{"hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("splitLines(%q) = %v (len %d), want %v (len %d)", tt.input, got, len(got), tt.expected, len(tt.expected))
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("splitLines(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestExportDiff(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-export-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Base file
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("original\n"), 0644)
	// Modified in upper
	os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("modified\n"), 0644)
	// New file in upper
	os.WriteFile(filepath.Join(upperDir, "new.txt"), []byte("added\n"), 0644)

	// Export to file
	outPath := filepath.Join(tmpDir, "output.diff")
	err = exportDiff(upperDir, baseDir, outPath)
	if err != nil {
		t.Fatalf("exportDiff failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}
	content := string(data)

	if len(content) == 0 {
		t.Error("expected non-empty diff output")
	}
}

func TestExportDiff_Stdout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-export-stdout-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(baseDir, 0755)

	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644)

	// Empty output = stdout
	err = exportDiff(upperDir, baseDir, "")
	if err != nil {
		t.Fatalf("exportDiff to stdout failed: %v", err)
	}
}

func TestExportTar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-export-tar-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(baseDir, 0755)

	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644)

	outPath := filepath.Join(tmpDir, "export.tar")
	err = exportTar(upperDir, baseDir, outPath)
	if err != nil {
		t.Fatalf("exportTar failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("tar file not created: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty tar file")
	}
}

func TestExportTar_Gzip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-export-targz-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	upperDir := filepath.Join(tmpDir, "upper")
	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(upperDir, 0755)
	os.MkdirAll(baseDir, 0755)

	os.WriteFile(filepath.Join(upperDir, "file.txt"), []byte("content"), 0644)

	outPath := filepath.Join(tmpDir, "export.tar.gz")
	err = exportTar(upperDir, baseDir, outPath)
	if err != nil {
		t.Fatalf("exportTar gz failed: %v", err)
	}

	info, _ := os.Stat(outPath)
	if info.Size() == 0 {
		t.Error("expected non-empty gzip tar file")
	}
}


func TestExportDiff_Whiteout(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "phantom-export-wh-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupTestEnv(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Base has a file that was "deleted" via whiteout
	os.WriteFile(filepath.Join(baseDir, "removed.txt"), []byte("old content\n"), 0644)
	os.WriteFile(filepath.Join(upperDir, ".wh.removed.txt"), []byte{}, 0644)

	outPath := filepath.Join(tmpDir, "output.diff")
	err = exportDiff(upperDir, baseDir, outPath)
	if err != nil {
		t.Fatalf("exportDiff with whiteout failed: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)
	if len(content) == 0 {
		t.Error("expected diff output for whiteout deletion")
	}
}

func TestWriteUnifiedDiff_NewFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "new.txt", "", "new content\n")
	output := buf.String()
	if !strings.Contains(output, "--- /dev/null") {
		t.Error("new file diff should reference /dev/null for old")
	}
	if !strings.Contains(output, "+++ b/new.txt") {
		t.Error("new file diff should show new path")
	}
}

func TestWriteUnifiedDiff_DeletedFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "old.txt", "old content\n", "")
	output := buf.String()
	if !strings.Contains(output, "+++ /dev/null") {
		t.Error("deleted file diff should reference /dev/null for new")
	}
}

func TestWriteUnifiedDiff_ModifiedFile(t *testing.T) {
	var buf bytes.Buffer
	writeUnifiedDiff(&buf, "mod.txt", "old\n", "new\n")
	output := buf.String()
	if !strings.Contains(output, "--- a/mod.txt") {
		t.Error("modified file diff should show old path")
	}
	if !strings.Contains(output, "+++ b/mod.txt") {
		t.Error("modified file diff should show new path")
	}
}
