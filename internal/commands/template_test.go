package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateInternalFuncs(t *testing.T) {
	if idx := indexOf("hello world", "world"); idx != 6 {
		t.Errorf("indexOf failed, got %d", idx)
	}
	if idx := indexOf("hello world", "baz"); idx != -1 {
		t.Errorf("indexOf failed, got %d", idx)
	}

	if s := trimSpace("  hello \t"); s != "hello" {
		t.Errorf("trimSpace failed, got %q", s)
	}

	parts := splitString("a,b,c", ",")
	if len(parts) != 3 || parts[0] != "a" || parts[1] != "b" || parts[2] != "c" {
		t.Errorf("splitString failed, got %v", parts)
	}

	names := parseInlineAgentNames(" claude , gemini,  ")
	if len(names) != 2 || names[0] != "claude" || names[1] != "gemini" {
		t.Errorf("parseInlineAgentNames failed, got %v", names)
	}

	if templ := findTemplate("claude"); templ == nil {
		t.Error("findTemplate failed for claude")
	}
	if templ := findTemplate("nonexistent"); templ != nil {
		t.Error("findTemplate succeeded for nonexistent")
	}
}

func runCommandWithArgs(t *testing.T, args []string, f func()) {
	oldArgs := os.Args
	os.Args = append([]string{"phantom"}, args...)
	defer func() { os.Args = oldArgs }()
	f()
}

func TestDoTemplateCommands(t *testing.T) {
	oldLog := log
	defer func() {
		log = oldLog
	}()
	log = &MockLogger{}

	// Temporarily redirect stdout? Not strictly necessary for coverage, but nice to avoid noise
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	cmd := NewTemplateCommand()

	// 1. template list
	runCommandWithArgs(t, []string{"list"}, func() {
		if err := cmd.Execute(context.Background()); err != nil {
			t.Logf("Execute list err: %v", err)
		}
	})

	runCommandWithArgs(t, []string{"list", "--format", "json"}, func() {
		_ = cmd.Execute(context.Background())
	})

	// 2. template show
	runCommandWithArgs(t, []string{"show", "claude"}, func() {
		_ = cmd.Execute(context.Background())
	})
	runCommandWithArgs(t, []string{"show", "nonexistent"}, func() {
		_ = cmd.Execute(context.Background())
	})

	// 3. template generate
	tmpFile := filepath.Join(t.TempDir(), "agents.yaml")
	runCommandWithArgs(t, []string{"generate", "--agents", "claude,gemini", "--output", tmpFile}, func() {
		_ = cmd.Execute(context.Background())
	})

	runCommandWithArgs(t, []string{"generate", "--agents", "claude"}, func() {
		_ = cmd.Execute(context.Background())
	})

	runCommandWithArgs(t, []string{"generate", "--agents", "nonexistent"}, func() {
		_ = cmd.Execute(context.Background())
	})
}
