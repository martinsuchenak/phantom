package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/pkg/api"
)

func runGit(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed in %s: %v\nOutput: %s", args, dir, err, string(out))
	}
}

func initGitRepo(t *testing.T, dir string) {
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Create initial commit
	file := filepath.Join(dir, "README.md")
	os.WriteFile(file, []byte("init"), 0644)
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
}

func TestSyncGit(t *testing.T) {
	tmpDir := t.TempDir()

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()
	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	log = &MockLogger{}

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)
	initGitRepo(t, baseDir)

	mountPoint := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(mountPoint, 0755)

	// Copy .git and contents from base to mount point to mock it as an overlay Repo
	// We'll just init another git repo there with same state and an extra commit
	initGitRepo(t, mountPoint)
	runGit(t, mountPoint, "checkout", "-b", "phantom/overlay1")
	file := filepath.Join(mountPoint, "overlay.txt")
	os.WriteFile(file, []byte("overlay change"), 0644)
	runGit(t, mountPoint, "add", "overlay.txt")
	runGit(t, mountPoint, "commit", "-m", "overlay")

	// We have uncommitted changes to test stash
	os.WriteFile(filepath.Join(mountPoint, "uncommitted.txt"), []byte("uncommitted"), 0644)
	runGit(t, mountPoint, "add", "uncommitted.txt")

	os.WriteFile(filepath.Join(baseDir, "base.txt"), []byte("base change"), 0644)
	runGit(t, baseDir, "add", "base.txt")
	runGit(t, baseDir, "commit", "-m", "base update")

	// Since we don't have a real remote linked between base and overlay, we can't test remote fetching easily without more setup.
	// But syncGit doesn't strictly fail if fetch fails, it just logs a warning. So it should be fine.

	// Link overlay mountPoint to fetch from baseDir
	runGit(t, mountPoint, "remote", "add", "origin", baseDir)

	gitOps := git.NewOperations()
	ovl := &api.Overlay{
		Name:       "overlay1",
		BaseDir:    baseDir,
		MountPoint: mountPoint,
	}

	// Wait, syncGit expects to run git commands...
	// Let's test with doStash=false
	err := syncGit(context.Background(), ovl, nil, gitOps, false, false)
	if err != nil {
		t.Fatalf("syncGit failed: %v", err)
	}

	// Test dry run inside same function since it's fast
	err = syncGit(context.Background(), ovl, nil, gitOps, true, true)
	if err != nil {
		t.Fatalf("syncGit dryRun failed: %v", err)
	}
}

func TestSyncGitStash(t *testing.T) {
	tmpDir := t.TempDir()

	oldCfg := cfg
	oldLog := log
	defer func() {
		cfg = oldCfg
		log = oldLog
	}()
	cfg = &config.Config{
		StateDir: filepath.Join(tmpDir, "state"),
	}
	log = &MockLogger{}

	baseDir := filepath.Join(tmpDir, "base")
	os.MkdirAll(baseDir, 0755)
	initGitRepo(t, baseDir)

	mountPoint := filepath.Join(tmpDir, "mnt")
	os.MkdirAll(mountPoint, 0755)
	initGitRepo(t, mountPoint)
	runGit(t, mountPoint, "checkout", "-b", "phantom/overlay1")
	runGit(t, mountPoint, "remote", "add", "origin", baseDir)

	// Uncommitted change to Stash
	os.WriteFile(filepath.Join(mountPoint, "uncommitted.txt"), []byte("uncommitted"), 0644)
	runGit(t, mountPoint, "add", "uncommitted.txt")

	gitOps := git.NewOperations()
	ovl := &api.Overlay{
		Name:       "overlay1",
		BaseDir:    baseDir,
		MountPoint: mountPoint,
	}

	// doStash=true
	err := syncGit(context.Background(), ovl, nil, gitOps, false, true)
	if err != nil {
		t.Fatalf("syncGit with stash failed: %v", err)
	}
}
