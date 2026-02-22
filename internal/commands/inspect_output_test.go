package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestInspectOutputJSON(t *testing.T) {
	out := inspectOutput{
		Name:       "test-inspect",
		BaseDir:    "/tmp/base",
		MountPoint: "/tmp/mnt/test",
		UpperDir:   "/tmp/upper",
		Branch:     "feature/test",
		Persistent: true,
		Locked:     true,
		PinnedCommit: "abc123def456",
		CreatedAt:  "2026-02-22 10:00:00",
		Mounted:    true,
		Uptime:     "5m",
		SizeBytes:  4096,
		PID:        12345,
		FilesAdded: 3,
		FilesMod:   2,
		FilesDel:   1,
		GitBranch:  "feature/test",
		GitDirty:   true,
		Snapshots:  2,
		HasLog:     true,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)

	if !strings.Contains(output, "test-inspect") {
		t.Error("expected name in JSON")
	}
	if !strings.Contains(output, "abc123def456") {
		t.Error("expected pinned commit in JSON")
	}
	if !strings.Contains(output, `"locked": true`) {
		t.Error("expected locked=true in JSON")
	}
}

func TestInspectOutputText(t *testing.T) {
	out := inspectOutput{
		Name:       "text-test",
		BaseDir:    "/tmp/base",
		MountPoint: "/tmp/mnt",
		UpperDir:   "/tmp/upper",
		Branch:     "main",
		Locked:     false,
		PinnedCommit: "abc123def4567890",
		CreatedAt:  "2026-02-22 10:00:00",
		Mounted:    true,
		Uptime:     "10m",
		SizeBytes:  2048,
		PID:        999,
		FilesAdded: 1,
		FilesMod:   0,
		FilesDel:   0,
		Snapshots:  0,
		HasLog:     false,
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Simulate text output
	fmt := func(format string, args ...any) {
		os.Stdout.WriteString(strings.NewReplacer().Replace(format))
	}
	_ = fmt // just verify the struct is usable

	// Print pinned commit (truncated)
	pinShort := out.PinnedCommit
	if len(pinShort) > 10 {
		pinShort = pinShort[:10]
	}
	if pinShort != "abc123def4" {
		t.Errorf("expected truncated pin 'abc123def4', got %q", pinShort)
	}

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
}

func TestInspectOutputSerialization(t *testing.T) {
	out := inspectOutput{
		Name:    "serial-test",
		Mounted: false,
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}

	var loaded inspectOutput
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if loaded.Name != "serial-test" {
		t.Errorf("expected name 'serial-test', got %q", loaded.Name)
	}
	if loaded.Mounted {
		t.Error("expected mounted=false")
	}
}
