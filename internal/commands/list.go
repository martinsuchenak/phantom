package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewListCommand creates the list command
func NewListCommand() *cli.Command {
	return &cli.Command{
		Name:        "list",
		Usage:       "List all active overlays",
		Description: "Lists all overlays, showing their status, mount points, and associated branches.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   "Show all overlays including unmounted",
			},
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Output format (table, json, simple)",
				DefaultValue: "table",
			},
		},
		Run: doList,
	}
}

// listOverlayInfo contains info for displaying in list
type listOverlayInfo struct {
	Name    string
	Mounted bool
	Path    string
	Branch  string
	Uptime  time.Duration
	Size    int64
}

func doList(ctx context.Context, cmd *cli.Command) error {
	showAll := cmd.GetBool("all")
	format := cmd.GetString("format")

	log.Debug("Listing overlays (all=%v, format=%s)", showAll, format)

	// Initialize state store
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Load all overlays
	overlays, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load overlays: %w", err)
	}

	// Create overlay manager
	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}

	// Filter and collect status
	var infos []listOverlayInfo

	for _, ovl := range overlays {
		mounted, err := mgr.IsMounted(ovl)
		if err != nil {
			log.Debug("Failed to check mount status for %s: %v", ovl.Name, err)
		}

		// Skip unmounted if not showing all
		if !showAll && !mounted {
			continue
		}

		infos = append(infos, listOverlayInfo{
			Name:    ovl.Name,
			Mounted: mounted,
			Path:    ovl.MountPoint,
			Branch:  ovl.Branch,
			Uptime:  time.Since(ovl.CreatedAt),
		})
	}

	// Output based on format
	switch format {
	case "json":
		return printListJSON(infos)
	case "simple":
		return printListSimple(infos)
	default:
		return printListTable(infos)
	}
}

func printListTable(infos []listOverlayInfo) error {
	if len(infos) == 0 {
		log.Info("No active overlays")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tMOUNT POINT\tBRANCH\tUPTIME")

	for _, info := range infos {
		status := "mounted"
		if !info.Mounted {
			status = "unmounted"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			info.Name,
			status,
			info.Path,
			info.Branch,
			formatDuration(info.Uptime),
		)
	}

	return w.Flush()
}

func printListSimple(infos []listOverlayInfo) error {
	for _, info := range infos {
		fmt.Println(info.Name)
	}
	return nil
}

func printListJSON(infos []listOverlayInfo) error {
	fmt.Println("[")
	for i, info := range infos {
		if i > 0 {
			fmt.Println(",")
		}
		fmt.Printf(`  {"name": "%s", "mounted": %v, "path": "%s", "branch": "%s"}`,
			info.Name, info.Mounted, info.Path, info.Branch)
	}
	fmt.Println("\n]")
	return nil
}
