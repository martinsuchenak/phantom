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
	testCfg := config.DefaultConfig()
	testCfg.Projects = map[string]config.Project{
		"testproj": {Path: "/path/to/testproj"},
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
	tmpDir := t.TempDir()
	cfgPath = filepath.Join(tmpDir, "config.yaml")

	cfg = config.DefaultConfig()
	cfg.Projects = make(map[string]config.Project)

	oldLog := log
	log = NewCLILogger(false)
	defer func() { log = oldLog }()

	t.Run("Add Project", func(t *testing.T) {
		projPath := filepath.Join(tmpDir, "my-app")
		if err := os.MkdirAll(projPath, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		if err := execCmd("add", "myapp", projPath); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		proj, ok := cfg.Projects["myapp"]
		if !ok {
			t.Fatal("expected project myapp to exist")
		}
		if proj.Path != projPath {
			t.Errorf("expected path %q, got %q", projPath, proj.Path)
		}
		if proj.Serve {
			t.Error("expected serve to be false by default")
		}
	})

	t.Run("Add Project with --serve", func(t *testing.T) {
		projPath := filepath.Join(tmpDir, "served-app")
		if err := os.MkdirAll(projPath, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}

		if err := execCmd("add", "--serve", "servedapp", projPath); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		proj, ok := cfg.Projects["servedapp"]
		if !ok {
			t.Fatal("expected project servedapp to exist")
		}
		if !proj.Serve {
			t.Error("expected serve to be true")
		}
	})

	t.Run("Serve existing project", func(t *testing.T) {
		cfg.Projects["myapp"] = config.Project{Path: cfg.Projects["myapp"].Path, Serve: false}

		if err := execCmd("serve", "myapp"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !cfg.Projects["myapp"].Serve {
			t.Error("expected serve to be true after serve command")
		}
	})

	t.Run("Unserve project", func(t *testing.T) {
		if err := execCmd("unserve", "myapp"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if cfg.Projects["myapp"].Serve {
			t.Error("expected serve to be false after unserve command")
		}
	})

	t.Run("Serve non-existent project", func(t *testing.T) {
		err := execCmd("serve", "doesntexist")
		if err == nil {
			t.Fatal("expected error for non-existent project")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got %q", err.Error())
		}
	})

	t.Run("Remove Project", func(t *testing.T) {
		cfg.Projects["toremove"] = config.Project{Path: "/tmp/fake"}

		if err := execCmd("remove", "toremove"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if _, exists := cfg.Projects["toremove"]; exists {
			t.Error("expected project to be removed")
		}
	})

	t.Run("Remove Non-existent Project", func(t *testing.T) {
		err := execCmd("remove", "doesntexist")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected error to contain 'not found', got %q", err.Error())
		}
	})

	t.Run("List Projects", func(t *testing.T) {
		cfg.Projects["proj1"] = config.Project{Path: "/tmp/1"}
		cfg.Projects["proj2"] = config.Project{Path: "/tmp/2", Serve: true}

		if err := execCmd("list"); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})
}
