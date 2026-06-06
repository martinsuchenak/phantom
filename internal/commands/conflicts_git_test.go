package commands

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

func runGitCommand(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_DISCOVERY_ACROSS_FILESYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
}

func TestConflicts_GitConfidenceScore(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base_repo")
	_ = os.MkdirAll(baseDir, 0755)

	// Initialize git repo
	runGitCommand(t, baseDir, "init")
	runGitCommand(t, baseDir, "config", "user.email", "test@example.com")
	runGitCommand(t, baseDir, "config", "user.name", "Test User")

	// Create initial file
	initialContent := "line1\nline2\nline3\nline4\nline5\n"
	err := os.WriteFile(filepath.Join(baseDir, "shared_clean.txt"), []byte(initialContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(baseDir, "shared_hard.txt"), []byte("initial\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, baseDir, "add", ".")
	runGitCommand(t, baseDir, "commit", "-m", "init")

	// Create Overlay 1
	ovl1Name := "ovl1"
	upper1 := filepath.Join(tmpDir, "upper1")
	_ = os.MkdirAll(upper1, 0755)
	runGitCommand(t, baseDir, "checkout", "-b", "phantom/ovl1")

	// Mod 1: Clean (line 1)
	err = os.WriteFile(filepath.Join(upper1, "shared_clean.txt"), []byte("line1_mod1\nline2\nline3\nline4\nline5\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	// Mod 2: Hard
	err = os.WriteFile(filepath.Join(upper1, "shared_hard.txt"), []byte("mod1\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	runGitCommand(t, baseDir, "checkout", "main")

	// Create Overlay 2
	ovl2Name := "ovl2"
	upper2 := filepath.Join(tmpDir, "upper2")
	_ = os.MkdirAll(upper2, 0755)
	runGitCommand(t, baseDir, "checkout", "-b", "phantom/ovl2")

	// Mod 1: Clean (line 5)
	err = os.WriteFile(filepath.Join(upper2, "shared_clean.txt"), []byte("line1\nline2\nline3\nline4\nline5_mod2\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	// Mod 2: Hard
	err = os.WriteFile(filepath.Join(upper2, "shared_hard.txt"), []byte("mod2\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	ovl1 := &api.Overlay{Name: ovl1Name, BaseDir: baseDir, UpperDir: upper1, CreatedAt: time.Now()}
	ovl2 := &api.Overlay{Name: ovl2Name, BaseDir: baseDir, UpperDir: upper2, CreatedAt: time.Now()}
	_ = store.Save(ovl1)
	_ = store.Save(ovl2)

	// Run conflicts command
	app := &cli.Command{
		Name: "phantom",
		Commands: []*cli.Command{
			NewConflictsCommand(),
		},
	}
	os.Args = []string{"phantom", "conflicts", "--format", "json", "ovl1 ovl2"}

	ctx := context.Background()

	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = app.Execute(ctx)

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("expected no err, got: %v", err)
	}

	if !strings.Contains(output, `"file": "shared_clean.txt"`) {
		t.Errorf("Expected output to contain 'shared_clean.txt', got:\n%s", output)
	}
	if !strings.Contains(output, `"file": "shared_hard.txt"`) {
		t.Errorf("Expected output to contain 'shared_hard.txt', got:\n%s", output)
	}
	if !strings.Contains(output, `"description": "Clean Merge (Git)"`) {
		t.Errorf("Expected output to contain 'Clean Merge (Git)', got:\n%s", output)
	}
	if !strings.Contains(output, `"description": "Hard Conflict (Git)"`) {
		t.Errorf("Expected output to contain 'Hard Conflict (Git)', got:\n%s", output)
	}
	if !strings.Contains(output, `"confidence_score": 78`) {
		// 100 - 20 (hard) - 2 (clean) = 78
		t.Errorf("Expected output to contain 'confidence_score: 78', got:\n%s", output)
	}
}

func TestConflicts_NonGitHardConflict(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)
	store := createTestStore(t, tmpDir)

	baseDir := filepath.Join(tmpDir, "base_dir")
	_ = os.MkdirAll(baseDir, 0755)

	// Create Overlay 1
	ovl1Name := "ovl1"
	upper1 := filepath.Join(tmpDir, "upper1")
	_ = os.MkdirAll(upper1, 0755)

	// Mod 1
	err := os.WriteFile(filepath.Join(upper1, "shared.txt"), []byte("line1_mod1\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create Overlay 2
	ovl2Name := "ovl2"
	upper2 := filepath.Join(tmpDir, "upper2")
	_ = os.MkdirAll(upper2, 0755)

	// Mod 2
	err = os.WriteFile(filepath.Join(upper2, "shared.txt"), []byte("line1_mod2\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	ovl1 := &api.Overlay{Name: ovl1Name, BaseDir: baseDir, UpperDir: upper1, CreatedAt: time.Now()}
	ovl2 := &api.Overlay{Name: ovl2Name, BaseDir: baseDir, UpperDir: upper2, CreatedAt: time.Now()}
	_ = store.Save(ovl1)
	_ = store.Save(ovl2)

	// Run conflicts command
	app := &cli.Command{
		Name: "phantom",
		Commands: []*cli.Command{
			NewConflictsCommand(),
		},
	}
	os.Args = []string{"phantom", "conflicts", "--format", "json", "ovl1 ovl2"}

	ctx := context.Background()

	// Redirect stdout to capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = app.Execute(ctx)

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Fatalf("expected no err, got: %v", err)
	}

	if !strings.Contains(output, `"file": "shared.txt"`) {
		t.Errorf("Expected output to contain 'shared.txt', got:\n%s", output)
	}
	if !strings.Contains(output, `"description": "Hard Conflict (Overwrite)"`) {
		t.Errorf("Expected output to contain 'Hard Conflict (Overwrite)', got:\n%s", output)
	}
	if !strings.Contains(output, `"confidence_score": 80`) {
		// 100 - 20 (hard) = 80
		t.Errorf("Expected output to contain 'confidence_score: 80', got:\n%s", output)
	}
}
