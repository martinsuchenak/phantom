package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/paularlott/cli"
)

func NewProjectCommand() *cli.Command {
	return &cli.Command{
		Name:        "project",
		Usage:       "Manage registered projects",
		Description: "Register projects to use their names as base directories. Mark projects with --serve to expose them via gRPC for remote overlays.",
		Commands: []*cli.Command{
			NewProjectAddCommand(),
			NewProjectRemoveCommand(),
			NewProjectListCommand(),
			NewProjectServeCommand(),
			NewProjectUnserveCommand(),
		},
	}
}

func NewProjectAddCommand() *cli.Command {
	return &cli.Command{
		Name:        "add",
		Usage:       "Register a new project",
		Description: "Adds a project name-to-path mapping. Use --serve to also expose it via gRPC for remote overlays.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "serve",
				Usage: "Expose this project via gRPC for remote overlays (phantom node start)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name for the registered project",
				Required: true,
			},
			&cli.StringArg{
				Name:     "path",
				Usage:    "Base directory path of the project",
				Required: true,
			},
		},
		Run: doProjectAdd,
	}
}

func NewProjectRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:        "remove",
		Usage:       "Remove a registered project",
		Description: "Removes a project from the configuration.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the registered project to remove",
				Required: true,
			},
		},
		Run: doProjectRemove,
	}
}

func NewProjectListCommand() *cli.Command {
	return &cli.Command{
		Name:        "list",
		Usage:       "List registered projects",
		Description: "Lists all registered projects, their paths, and whether they are served remotely.",
		Run:         doProjectList,
	}
}

func NewProjectServeCommand() *cli.Command {
	return &cli.Command{
		Name:        "serve",
		Usage:       "Mark a project as served via gRPC",
		Description: "Marks an existing project so that it is exposed by the phantom node daemon for remote overlays.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the project to serve",
				Required: true,
			},
		},
		Run: doProjectServe,
	}
}

func NewProjectUnserveCommand() *cli.Command {
	return &cli.Command{
		Name:        "unserve",
		Usage:       "Stop serving a project via gRPC",
		Description: "Removes the serve flag from a project so it is no longer exposed by the phantom node daemon.",
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the project to stop serving",
				Required: true,
			},
		},
		Run: doProjectUnserve,
	}
}

func doProjectAdd(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	projectPath := cmd.GetStringArg("path")
	serve := cmd.GetBool("serve")

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Warn("Directory %q does not exist. It has been added anyway, but may cause errors if used.", absPath)
		} else {
			return fmt.Errorf("failed to access path: %w", err)
		}
	} else if !info.IsDir() {
		return fmt.Errorf("path %q is not a directory", absPath)
	}

	if cfg.Projects == nil {
		cfg.Projects = make(map[string]config.Project)
	}

	cfg.Projects[name] = config.Project{Path: absPath, Serve: serve}
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	if serve {
		log.Info("Registered project %q -> %s (served via gRPC)", name, absPath)
	} else {
		log.Info("Registered project %q -> %s", name, absPath)
	}
	return nil
}

func doProjectRemove(_ context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")

	if cfg.Projects == nil {
		return fmt.Errorf("no projects registered")
	}

	if _, ok := cfg.Projects[name]; !ok {
		return fmt.Errorf("project %q not found", name)
	}

	delete(cfg.Projects, name)
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.Info("Removed project %q", name)
	return nil
}

func doProjectList(_ context.Context, _ *cli.Command) error {
	if len(cfg.Projects) == 0 {
		log.Info("No projects registered.")
		return nil
	}

	names := make([]string, 0, len(cfg.Projects))
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tPATH\tSERVED")
	for _, name := range names {
		proj := cfg.Projects[name]
		served := "-"
		if proj.Serve {
			served = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", name, proj.Path, served)
	}
	return w.Flush()
}

func doProjectServe(_ context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	return setProjectServe(name, true)
}

func doProjectUnserve(_ context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	return setProjectServe(name, false)
}

func setProjectServe(name string, serve bool) error {
	if cfg.Projects == nil {
		return fmt.Errorf("no projects registered")
	}

	proj, ok := cfg.Projects[name]
	if !ok {
		return fmt.Errorf("project %q not found", name)
	}

	proj.Serve = serve
	cfg.Projects[name] = proj

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	action := "now served"
	if !serve {
		action = "no longer served"
	}
	log.Info("Project %q is %s via gRPC", name, action)
	return nil
}
