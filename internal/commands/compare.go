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

// NewCompareCommand creates the compare command
func NewCompareCommand() *cli.Command {
	return &cli.Command{
		Name:        "compare",
		Usage:       "Compare changes between two overlays",
		Description: "Shows a side-by-side view of what each overlay changed relative to the base. Highlights files unique to each overlay and files both modified.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Usage: "Output format (table, json)", DefaultValue: "table"},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "overlay-a", Usage: "First overlay name", Required: true},
			&cli.StringArg{Name: "overlay-b", Usage: "Second overlay name", Required: true},
		},
		Run: doCompare,
	}
}

type compareEntry struct {
	File    string `json:"file"`
	StatusA string `json:"status_a,omitempty"` // added/modified/deleted or empty
	StatusB string `json:"status_b,omitempty"`
	SizeA   int64  `json:"size_a,omitempty"`
	SizeB   int64  `json:"size_b,omitempty"`
	Both    bool   `json:"both"` // changed in both overlays
}

type compareResult struct {
	OverlayA  string         `json:"overlay_a"`
	OverlayB  string         `json:"overlay_b"`
	Files     []compareEntry `json:"files"`
	OnlyA     int            `json:"only_a"`
	OnlyB     int            `json:"only_b"`
	Both      int            `json:"both"`
}

func doCompare(ctx context.Context, cmd *cli.Command) error {
	nameA := cmd.GetStringArg("overlay-a")
	nameB := cmd.GetStringArg("overlay-b")
	format := cmd.GetString("format")

	if nameA == nameB {
		return fmt.Errorf("cannot compare an overlay with itself")
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovlA, err := store.Load(nameA)
	if err != nil {
		return fmt.Errorf("overlay %q: %w", nameA, err)
	}
	ovlB, err := store.Load(nameB)
	if err != nil {
		return fmt.Errorf("overlay %q: %w", nameB, err)
	}

	if ovlA.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", nameA)
	}
	if ovlB.UpperDir == "" {
		return fmt.Errorf("overlay %q has no upper directory", nameB)
	}

	// Scan both overlays
	changesA := scanChanges(ovlA.UpperDir, ovlA.BaseDir)
	changesB := scanChanges(ovlB.UpperDir, ovlB.BaseDir)

	// Build comparison
	result := buildComparison(nameA, nameB, changesA, changesB)

	if format == "json" {
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	return printCompareTable(result)
}

type fileChange struct {
	Status string // added, modified, deleted
	Size   int64
}

func scanChanges(upperDir, baseDir string) map[string]fileChange {
	changes := make(map[string]fileChange)

	filepath.Walk(upperDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, err := filepath.Rel(upperDir, path)
		if err != nil || relPath == "." {
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
			deletedName := strings.TrimPrefix(baseName, ".wh.")
			actualPath := filepath.Join(filepath.Dir(relPath), deletedName)
			changes[actualPath] = fileChange{Status: "deleted", Size: 0}
			return nil
		}

		status := "added"
		if _, err := os.Stat(filepath.Join(baseDir, relPath)); err == nil {
			status = "modified"
		}
		changes[relPath] = fileChange{Status: status, Size: info.Size()}
		return nil
	})

	return changes
}

func buildComparison(nameA, nameB string, changesA, changesB map[string]fileChange) compareResult {
	result := compareResult{OverlayA: nameA, OverlayB: nameB}

	// Collect all unique file paths
	allFiles := make(map[string]bool)
	for f := range changesA {
		allFiles[f] = true
	}
	for f := range changesB {
		allFiles[f] = true
	}

	for file := range allFiles {
		entry := compareEntry{File: file}
		chA, inA := changesA[file]
		chB, inB := changesB[file]

		if inA {
			entry.StatusA = chA.Status
			entry.SizeA = chA.Size
		}
		if inB {
			entry.StatusB = chB.Status
			entry.SizeB = chB.Size
		}

		if inA && inB {
			entry.Both = true
			result.Both++
		} else if inA {
			result.OnlyA++
		} else {
			result.OnlyB++
		}

		result.Files = append(result.Files, entry)
	}

	return result
}

func printCompareTable(result compareResult) error {
	if len(result.Files) == 0 {
		log.Info("No changes in either overlay")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "FILE\t%s\t%s\tNOTE\n", result.OverlayA, result.OverlayB)

	for _, f := range result.Files {
		colA := f.StatusA
		if colA == "" {
			colA = "—"
		}
		colB := f.StatusB
		if colB == "" {
			colB = "—"
		}
		note := ""
		if f.Both {
			if f.StatusA == f.StatusB && f.SizeA == f.SizeB {
				note = "identical change"
			} else {
				note = "⚠ diverged"
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", f.File, colA, colB, note)
	}
	w.Flush()

	fmt.Println()
	log.Info("Only in %s: %d | Only in %s: %d | Both: %d",
		result.OverlayA, result.OnlyA, result.OverlayB, result.OnlyB, result.Both)
	return nil
}
