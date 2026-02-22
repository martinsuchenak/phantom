package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestStatusJSONOutputFormat(t *testing.T) {
	statuses := []struct {
		Name    string `json:"name"`
		Mounted bool   `json:"mounted"`
	}{
		{Name: "overlay-1", Mounted: true},
		{Name: "overlay-2", Mounted: false},
	}

	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "overlay-1") {
		t.Error("expected overlay-1 in JSON output")
	}
}

func TestPrintStatusJSON_Full(t *testing.T) {
	tmpDir := t.TempDir()
	baseDir := filepath.Join(tmpDir, "base")
	upperDir := filepath.Join(tmpDir, "upper")
	os.MkdirAll(baseDir, 0755)
	os.MkdirAll(upperDir, 0755)

	// Create some files for change counting
	os.WriteFile(filepath.Join(baseDir, "existing.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(upperDir, "existing.txt"), []byte("new"), 0644)
	os.WriteFile(filepath.Join(upperDir, "added.txt"), []byte("new"), 0644)

	ovl := &api.Overlay{
		Name:       "json-test",
		MountPoint: filepath.Join(tmpDir, "mnt"),
		BaseDir:    baseDir,
		UpperDir:   upperDir,
		Branch:     "feature/test",
		Persistent: true,
		CreatedAt:  time.Now(),
	}
	status := &api.OverlayStatus{
		Mounted:   true,
		Uptime:    10 * time.Minute,
		SizeBytes: 4096,
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printStatusJSON(ovl, status)

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("printStatusJSON failed: %v", err)
	}

	// Parse the JSON output
	var result statusJSONOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	if result.Name != "json-test" {
		t.Errorf("expected name 'json-test', got %q", result.Name)
	}
	if !result.Mounted {
		t.Error("expected mounted=true")
	}
	if !result.Persistent {
		t.Error("expected persistent=true")
	}
	if result.SizeBytes != 4096 {
		t.Errorf("expected size 4096, got %d", result.SizeBytes)
	}
	if result.Modified != 1 {
		t.Errorf("expected 1 modified, got %d", result.Modified)
	}
	if result.Added != 1 {
		t.Errorf("expected 1 added, got %d", result.Added)
	}
}
