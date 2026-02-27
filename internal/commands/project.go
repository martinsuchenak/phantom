package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/paularlott/cli"
)

// NewProjectCommand creates the project command group
func NewProjectCommand() *cli.Command {
	return &cli.Command{
		Name:        "project",
		Usage:       "Manage registered projects",
		Description: "Register projects to use their names instead of absolute paths as base directories.",
		Commands: []*cli.Command{
			NewProjectAddCommand(),
			NewProjectRemoveCommand(),
			NewProjectListCommand(),
		},
	}
}

// NewProjectAddCommand creates the project add command
func NewProjectAddCommand() *cli.Command {
	return &cli.Command{
		Name:        "add",
		Usage:       "Register a new project",
		Description: "Adds a project name to path mapping so the name can be used as a base directory.",
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

// NewProjectRemoveCommand creates the project remove command
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

// NewProjectListCommand creates the project list command
func NewProjectListCommand() *cli.Command {
	return &cli.Command{
		Name:        "list",
		Usage:       "List registered projects",
		Description: "Lists all currently registered projects and their paths.",
		Run:         doProjectList,
	}
}

func doProjectAdd(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	projectPath := cmd.GetStringArg("path")

	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Verify the directory exists (optional, but helpful)
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
		cfg.Projects = make(map[string]string)
	}

	cfg.Projects[name] = absPath
	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	log.Info("Successfully registered project %q -> %s", name, absPath)
	return nil
}

func doProjectRemove(ctx context.Context, cmd *cli.Command) error {
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

	log.Info("Successfully removed project %q", name)
	return nil
}

func doProjectList(ctx context.Context, cmd *cli.Command) error {
	if len(cfg.Projects) == 0 {
		log.Info("No projects registered.")
		return nil
	}

	var names []string
	for name := range cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tPATH")

	for _, name := range names {
		fmt.Fprintf(w, "%s\t%s\n", name, cfg.Projects[name])
	}
	w.Flush()

	return nil
}
