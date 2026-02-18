package commands

import (
	"context"
	"encoding/json"
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

	return processStatus(ctx, name, format)
}

func processStatus(ctx context.Context, name, format string) error {
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
		type overlayStatusJSON struct {
			Name    string `json:"name"`
			Mounted bool   `json:"mounted"`
		}
		var statuses []overlayStatusJSON
		for _, ovl := range overlays {
			status, _ := mgr.GetStatus(ovl)
			statuses = append(statuses, overlayStatusJSON{
				Name:    ovl.Name,
				Mounted: status != nil && status.Mounted,
			})
		}
		data, err := json.MarshalIndent(statuses, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
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

// statusJSONOutput represents the JSON output structure for status
type statusJSONOutput struct {
	Name       string `json:"name"`
	Mounted    bool   `json:"mounted"`
	MountPoint string `json:"mount_point"`
	BaseDir    string `json:"base_dir"`
	UpperDir   string `json:"upper_dir"`
	Branch     string `json:"branch"`
	Persistent bool   `json:"persistent"`
	Uptime     string `json:"uptime"`
	SizeBytes  int64  `json:"size_bytes"`
}

func printStatusJSON(ovl *api.Overlay, status *api.OverlayStatus) error {
	output := statusJSONOutput{
		Name:       ovl.Name,
		Mounted:    status.Mounted,
		MountPoint: ovl.MountPoint,
		BaseDir:    ovl.BaseDir,
		UpperDir:   ovl.UpperDir,
		Branch:     ovl.Branch,
		Persistent: ovl.Persistent,
		Uptime:     formatDuration(status.Uptime),
		SizeBytes:  status.SizeBytes,
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}
