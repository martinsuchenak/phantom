package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewDiffCommand creates the diff command
func NewDiffCommand() *cli.Command {
	return &cli.Command{
		Name:        "diff",
		Usage:       "Show files changed in an overlay",
		Description: "Lists all files that have been modified, added, or deleted in the overlay's upper directory compared to the base.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Output format (table, json, simple)",
				DefaultValue: "table",
			},
			&cli.BoolFlag{
				Name:  "stat",
				Usage: "Show summary statistics only",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay",
				Required: true,
			},
		},
		Run: doDiff,
	}
}

// diffEntry represents a single changed file
type diffEntry struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added, modified, deleted
	Size   int64  `json:"size"`
}

// diffResult holds the full diff output
type diffResult struct {
	Name     string      `json:"name"`
	Files    []diffEntry `json:"files"`
	Added    int         `json:"added"`
	Modified int         `json:"modified"`
	Deleted  int         `json:"deleted"`
}

func doDiff(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}
	format := cmd.GetString("format")
	statOnly := cmd.GetBool("stat")

	return processDiff(name, format, statOnly)
}

func processDiff(name, format string, statOnly bool) error {
	log.Debug("Showing diff for overlay %q", name)

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	if ovl.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", name)
	}

	// Walk the upper directory to find all changes
	result := diffResult{Name: name}

	err = filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Get relative path from upper dir
		relPath, err := filepath.Rel(ovl.UpperDir, path)
		if err != nil || relPath == "." {
			return nil
		}

		// Skip the work directory marker files (Linux overlayfs internals)
		if strings.HasPrefix(relPath, "work/") || relPath == "work" {
			return nil
		}

		// Only track files, not directories (unless it's a whiteout)
		if info.IsDir() {
			return nil
		}

		// Check if this is a whiteout file (overlayfs marks deletions this way)
		baseName := filepath.Base(path)
		if strings.HasPrefix(baseName, ".wh.") {
			// Whiteout file = deletion
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			deletedPath := filepath.Join(filepath.Dir(relPath), deletedName)
			result.Files = append(result.Files, diffEntry{
				Path:   deletedPath,
				Status: "deleted",
				Size:   0,
			})
			result.Deleted++
			return nil
		}

		// Check if file exists in base directory
		basePath := filepath.Join(ovl.BaseDir, relPath)
		status := "added"
		if _, err := os.Stat(basePath); err == nil {
			status = "modified"
		}

		entry := diffEntry{
			Path:   relPath,
			Status: status,
			Size:   info.Size(),
		}
		result.Files = append(result.Files, entry)

		if status == "added" {
			result.Added++
		} else {
			result.Modified++
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk upper directory: %w", err)
	}

	if statOnly {
		return printDiffStat(result)
	}

	switch format {
	case "json":
		return printDiffJSON(result)
	case "simple":
		return printDiffSimple(result)
	default:
		return printDiffTable(result)
	}
}

func printDiffTable(result diffResult) error {
	if len(result.Files) == 0 {
		log.Info("No changes in overlay %q", result.Name)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STATUS\tFILE\tSIZE")

	for _, f := range result.Files {
		sizeStr := formatSize(f.Size)
		if f.Status == "deleted" {
			sizeStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", f.Status, f.Path, sizeStr)
	}

	fmt.Fprintln(w)
	w.Flush()

	return printDiffStat(result)
}

func printDiffSimple(result diffResult) error {
	for _, f := range result.Files {
		prefix := "M"
		switch f.Status {
		case "added":
			prefix = "A"
		case "deleted":
			prefix = "D"
		}
		fmt.Printf("%s\t%s\n", prefix, f.Path)
	}
	return nil
}

func printDiffJSON(result diffResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printDiffStat(result diffResult) error {
	total := result.Added + result.Modified + result.Deleted
	log.Info("%d file(s) changed: %d added, %d modified, %d deleted",
		total, result.Added, result.Modified, result.Deleted)
	return nil
}
// countFileChanges returns quick file change counts for an overlay's upper directory.
// Used by status command to show a summary without full diff output.
func countFileChanges(upperDir, baseDir string) (added, modified, deleted int) {
	if upperDir == "" {
		return
	}

	filepath.Walk(upperDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(upperDir, path)
		if relPath == "." {
			return nil
		}

		if strings.HasPrefix(relPath, "work/") || relPath == "work" {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		baseName := filepath.Base(path)
		if strings.HasPrefix(baseName, ".wh.") {
			deleted++
			return nil
		}

		basePath := filepath.Join(baseDir, relPath)
		if _, err := os.Stat(basePath); err == nil {
			modified++
		} else {
			added++
		}
		return nil
	})
	return
}
