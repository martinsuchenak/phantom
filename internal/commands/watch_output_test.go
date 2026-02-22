package commands

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestPrintWatchEvent_Simple(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printWatchEvent("simple", "+", "new-file.txt")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "+") {
		t.Errorf("expected '+' in output, got %q", output)
	}
	if !strings.Contains(output, "new-file.txt") {
		t.Errorf("expected 'new-file.txt' in output, got %q", output)
	}
}

func TestPrintWatchEvent_JSON(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printWatchEvent("json", "+", "added.txt")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"status":"added"`) {
		t.Errorf("expected JSON with status 'added', got %q", output)
	}
	if !strings.Contains(output, `"file":"added.txt"`) {
		t.Errorf("expected JSON with file 'added.txt', got %q", output)
	}
}

func TestPrintWatchEvent_Modified(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printWatchEvent("json", "~", "changed.txt")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"status":"modified"`) {
		t.Errorf("expected status 'modified', got %q", output)
	}
}

func TestPrintWatchEvent_Deleted(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printWatchEvent("json", "-", "removed.txt")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"status":"deleted"`) {
		t.Errorf("expected status 'deleted', got %q", output)
	}
}

func TestPrintWatchEvent_Reset(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printWatchEvent("json", "⊘", "reset.txt")

	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, `"status":"reset"`) {
		t.Errorf("expected status 'reset', got %q", output)
	}
}
