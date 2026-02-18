package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

// NewStatusCommand creates the status command
func NewStatusCommand() *cli.Command {
	return &cli.Command{
		Name:        "status",
		Usage:       "Show the status of an overlay",
		Description: "Shows detailed status information for one or all overlays.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Output format (table, json)",
				DefaultValue: "table",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay (optional, shows all if not specified)",
				Required: false,
			},
		},
		MinArgs: 0,
		MaxArgs: 1,
		Run:     doStatus,
	}
}

func doStatus(ctx context.Context, cmd *cli.Command) error {
	args := cmd.GetArgs()
	name := ""
	if len(args) > 0 {
		name = args[0]
	}
	format := cmd.GetString("format")

	log.Debug("Showing status for overlay %q (format=%s)", name, format)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Create overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	if name != "" {
		// Show status for a specific overlay
		return showSingleStatus(ctx, store, mgr, name, format)
	}

	// Show status for all overlays
	return showAllStatus(store, mgr, format)
}

func showSingleStatus(ctx context.Context, store *state.Store, mgr overlayManager, name, format string) error {
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	status, err := mgr.GetStatus(ovl)
	if err != nil {
		return fmt.Errorf("failed to get overlay status: %w", err)
	}

	if format == "json" {
		return printStatusJSON(ovl, status)
	}

	return printStatusTable(ctx, ovl, status)
}

func showAllStatus(store *state.Store, mgr overlayManager, format string) error {
	overlays, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load overlays: %w", err)
	}

	if len(overlays) == 0 {
		log.Info("No overlays found")
		return nil
	}

	if format == "json" {
		fmt.Println("[")
		for i, ovl := range overlays {
			if i > 0 {
				fmt.Println(",")
			}
			status, _ := mgr.GetStatus(ovl)
			fmt.Printf(`  {"name": "%s", "mounted": %v}`,
				ovl.Name, status != nil && status.Mounted)
		}
		fmt.Println("\n]")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tMOUNTED\tUPTIME\tSIZE")

	for _, ovl := range overlays {
		status, err := mgr.GetStatus(ovl)
		if err != nil {
			log.Debug("Failed to get status for %s: %v", ovl.Name, err)
			fmt.Fprintf(w, "%s\t?\t?\t?\n", ovl.Name)
			continue
		}

		mounted := "no"
		if status.Mounted {
			mounted = "yes"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			ovl.Name,
			mounted,
			formatDuration(status.Uptime),
			formatSize(status.SizeBytes),
		)
	}

	return w.Flush()
}

func printStatusTable(ctx context.Context, ovl *api.Overlay, status *api.OverlayStatus) error {
	fmt.Printf("Name:        %s\n", ovl.Name)
	fmt.Printf("Status:      ")
	if status.Mounted {
		fmt.Println("mounted")
	} else {
		fmt.Println("unmounted")
	}
	fmt.Printf("Mount Point: %s\n", ovl.MountPoint)
	fmt.Printf("Base Dir:    %s\n", ovl.BaseDir)
	fmt.Printf("Upper Dir:   %s\n", ovl.UpperDir)
	fmt.Printf("Branch:      %s\n", ovl.Branch)
	fmt.Printf("Persistent:  %v\n", ovl.Persistent)
	fmt.Printf("Created:     %s\n", ovl.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Uptime:      %s\n", formatDuration(status.Uptime))

	if status.Mounted && status.SizeBytes > 0 {
		fmt.Printf("Data Size:   %s\n", formatSize(status.SizeBytes))
	}

	// Show git status if mounted and branch exists
	if status.Mounted && ovl.Branch != "" {
		gitOps := git.NewOperations()
		changes, err := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
		if err == nil {
			fmt.Printf("Uncommitted: %v\n", changes)
		}
	}

	return nil
}

func printStatusJSON(ovl *api.Overlay, status *api.OverlayStatus) error {
	fmt.Printf(`{
  "name": "%s",
  "mounted": %v,
  "mount_point": "%s",
  "base_dir": "%s",
  "upper_dir": "%s",
  "branch": "%s",
  "persistent": %v,
  "uptime": "%s",
  "size_bytes": %d
}`,
		ovl.Name,
		status.Mounted,
		ovl.MountPoint,
		ovl.BaseDir,
		ovl.UpperDir,
		ovl.Branch,
		ovl.Persistent,
		formatDuration(status.Uptime),
		status.SizeBytes)
	fmt.Println()
	return nil
}
