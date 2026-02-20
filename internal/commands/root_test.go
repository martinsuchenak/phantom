package commands

import (
	"bytes"
	"fmt"
	"os"
	"testing"
)

func TestCLILogger(t *testing.T) {
	// Test non-verbose logger
	logger := NewCLILogger(false)

	// Info should always print
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Info("hello %s", "world")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "hello world\n" {
		t.Errorf("Info output = %q, want %q", buf.String(), "hello world\n")
	}

	// Debug should NOT print when not verbose
	r2, w2, _ := os.Pipe()
	os.Stdout = w2

	logger.Debug("debug msg")

	w2.Close()
	os.Stdout = old

	var buf2 bytes.Buffer
	buf2.ReadFrom(r2)
	if buf2.String() != "" {
		t.Errorf("Debug should not print when not verbose, got %q", buf2.String())
	}
}

func TestCLILogger_Verbose(t *testing.T) {
	logger := NewCLILogger(true)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Debug("debug %s", "msg")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "[DEBUG] debug msg\n" {
		t.Errorf("Debug output = %q, want %q", buf.String(), "[DEBUG] debug msg\n")
	}
}

func TestCLILogger_Trace(t *testing.T) {
	logger := NewCLILogger(true)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Trace("trace %s", "msg")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "[TRACE] trace msg\n" {
		t.Errorf("Trace output = %q, want %q", buf.String(), "[TRACE] trace msg\n")
	}
}

func TestCLILogger_Warn(t *testing.T) {
	logger := NewCLILogger(false)

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	logger.Warn("warn %s", "msg")

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "[WARN] warn msg\n" {
		t.Errorf("Warn output = %q, want %q", buf.String(), "[WARN] warn msg\n")
	}
}

func TestCLILogger_Error(t *testing.T) {
	logger := NewCLILogger(false)

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	logger.Error("error %s", "msg")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "[ERROR] error msg\n" {
		t.Errorf("Error output = %q, want %q", buf.String(), "[ERROR] error msg\n")
	}
}

func TestCLILogger_With(t *testing.T) {
	logger := NewCLILogger(false)

	// With methods should return the same logger (no-op)
	l2 := logger.With("key", "value")
	if l2 != logger {
		t.Error("With should return same logger")
	}

	l3 := logger.WithError(fmt.Errorf("test"))
	if l3 != logger {
		t.Error("WithError should return same logger")
	}

	l4 := logger.WithGroup("group")
	if l4 != logger {
		t.Error("WithGroup should return same logger")
	}
}

func TestSetVersion(t *testing.T) {
	oldV, oldC, oldD := version, commit, date
	defer func() {
		version = oldV
		commit = oldC
		date = oldD
	}()

	SetVersion("1.0.0", "abc1234", "2025-01-01")
	if version != "1.0.0" {
		t.Errorf("version = %q, want %q", version, "1.0.0")
	}
	if commit != "abc1234" {
		t.Errorf("commit = %q, want %q", commit, "abc1234")
	}
}
