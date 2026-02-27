package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/paularlott/cli"
)

func TestProjectResolver(t *testing.T) {
	// Setup global config mock
	testCfg := config.DefaultConfig()
	testCfg.Projects = map[string]string{
		"testproj": "/path/to/testproj",
	}
	cfg = testCfg
	defer func() { cfg = nil }()

	t.Run("Resolves registered project", func(t *testing.T) {
		res := resolveBaseDir("testproj")
		if res != "/path/to/testproj" {
			t.Errorf("expected /path/to/testproj, got %q", res)
		}
	})

	t.Run("Returns input for unregistered project", func(t *testing.T) {
		res := resolveBaseDir("/some/absolute/path")
		if res != "/some/absolute/path" {
			t.Errorf("expected /some/absolute/path, got %q", res)
		}
	})

	t.Run("Returns input for empty config", func(t *testing.T) {
		cfg.Projects = nil
		res := resolveBaseDir("proj")
		if res != "proj" {
			t.Errorf("expected proj, got %q", res)
		}
	})
}

func execCmd(args ...string) error {
	oldArgs := os.Args
	os.Args = append([]string{"phantom", "project"}, args...)
	defer func() { os.Args = oldArgs }()

	app := &cli.Command{
		Name: "phantom",
		Commands: []*cli.Command{
			NewProjectCommand(),
		},
	}
	return app.Execute(context.Background())
}

func TestProjectCommands(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	cfgPath = filepath.Join(tmpDir, "config.yaml")

	// Set global config
	cfg = config.DefaultConfig()
	cfg.Projects = make(map[string]string)

	// Suppress logs
	oldLog := log
	log = NewCLILogger(false)
	defer func() { log = oldLog }()

	t.Run("Add Project", func(t *testing.T) {
		projPath := filepath.Join(tmpDir, "my-app")
		err := os.MkdirAll(projPath, 0755)
		if err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		err = execCmd("add", "myapp", projPath)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.Projects["myapp"] != projPath {
			t.Errorf("expected project %q to have path %q, got %q", "myapp", projPath, cfg.Projects["myapp"])
		}
	})

	t.Run("Remove Project", func(t *testing.T) {
		// Add test project directly
		cfg.Projects["toremove"] = "/tmp/fake"

		err := execCmd("remove", "toremove")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if _, exists := cfg.Projects["toremove"]; exists {
			t.Errorf("expected project to be removed")
		}
	})

	t.Run("Remove Non-existent Project", func(t *testing.T) {
		err := execCmd("remove", "doesntexist")
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected error to contain 'not found', got %q", err.Error())
		}
	})

	t.Run("List Projects", func(t *testing.T) {
		cfg.Projects["proj1"] = "/tmp/1"
		cfg.Projects["proj2"] = "/tmp/2"

		err := execCmd("list")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
