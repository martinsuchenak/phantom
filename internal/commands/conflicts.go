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

func NewConflictsCommand() *cli.Command {
	return &cli.Command{
		Name:  "conflicts",
		Usage: "Detect file conflicts between overlays",
		Description: "Scans two or more overlays for overlapping file changes " +
			"that would conflict when merging.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Usage: "Output format (table, json)", DefaultValue: "table"},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "overlays", Usage: "Overlay names (space-separated, minimum 2)", Required: true},
		},
		Run: doConflicts,
	}
}

type conflictEntry struct {
	File     string   `json:"file"`
	Overlays []string `json:"overlays"`
}

func doConflicts(ctx context.Context, cmd *cli.Command) error {
	namesArg := cmd.GetStringArg("overlays")
	format := cmd.GetString("format")

	names := strings.Fields(namesArg)
	// Also check if remaining args were passed (cli may join them)
	if len(names) < 2 {
		// Try splitting by comma too
		names = strings.FieldsFunc(namesArg, func(r rune) bool {
			return r == ',' || r == ' '
		})
	}
	if len(names) < 2 {
		return fmt.Errorf("at least 2 overlay names required")
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	// Build map of file -> which overlays changed it
	fileMap := make(map[string][]string)

	for _, name := range names {
		ovl, err := store.Load(name)
		if err != nil {
			return fmt.Errorf("overlay %q: %w", name, err)
		}
		if ovl.UpperDir == "" {
			continue
		}

		filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			relPath, err := filepath.Rel(ovl.UpperDir, path)
			if err != nil || relPath == "." {
				return nil
			}
			if strings.HasPrefix(relPath, "work/") || relPath == "work" {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			// Normalize whiteout files to the actual filename
			baseName := filepath.Base(path)
			if strings.HasPrefix(baseName, ".wh.") {
				deletedName := strings.TrimPrefix(baseName, ".wh.")
				relPath = filepath.Join(filepath.Dir(relPath), deletedName)
			}
			fileMap[relPath] = append(fileMap[relPath], name)
			return nil
		})
	}

	// Find conflicts (files changed by more than one overlay)
	var conflicts []conflictEntry
	for file, overlays := range fileMap {
		if len(overlays) > 1 {
			conflicts = append(conflicts, conflictEntry{File: file, Overlays: overlays})
		}
	}

	if len(conflicts) == 0 {
		log.Info("No conflicts detected between %d overlays", len(names))
		return nil
	}

	if format == "json" {
		data, _ := json.MarshalIndent(conflicts, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FILE\tOVERLAYS")
	for _, c := range conflicts {
		fmt.Fprintf(w, "%s\t%s\n", c.File, strings.Join(c.Overlays, ", "))
	}
	w.Flush()
	fmt.Println()
	log.Info("%d conflicting file(s) detected", len(conflicts))
	return nil
}
