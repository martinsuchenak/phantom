package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/git"
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

		_ = filepath.Walk(ovl.UpperDir, func(path string, info os.FileInfo, err error) error {
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
	type detailedConflict struct {
		File        string   `json:"file"`
		Overlays    []string `json:"overlays"`
		IsHard      bool     `json:"is_hard"` // True if it's a non-git repo or a git hunk conflict
		Description string   `json:"description"`
	}
	var conflicts []detailedConflict

	gitOps := git.NewOperations()

	for file, overlays := range fileMap {
		if len(overlays) > 1 {
			isHard := true
			desc := "Hard Conflict (Overwrite)"

			// Check if base is a git repo. If so, try to see if git can auto-merge it.
			// We need to pick two overlays to check at a time. For simplicity, we just check the first vs the second.
			// In reality, if 3+ overlays touch a file, it's increasingly likely to be a hard conflict.
			if len(overlays) == 2 {
				// We need the base dir from one of the overlays
				ovl, _ := store.Load(overlays[0])
				if ovl != nil {
					isGit, _ := gitOps.IsGitRepo(ctx, ovl.BaseDir)
					if isGit {
						ovl1, _ := store.Load(overlays[0])
						ovl2, _ := store.Load(overlays[1])
						baseFile := filepath.Join(ovl.BaseDir, file)
						ourFile := filepath.Join(ovl1.UpperDir, file)
						theirFile := filepath.Join(ovl2.UpperDir, file)

						// If base file doesn't exist, we use an empty file as the common ancestor
						// or just consider it a hard conflict if both created it with different contents.
						// git merge-file handles non-existent base poorly if we don't pass an empty file.
						// For now, let's just create an empty temp file if base doesn't exist.
						if _, err := os.Stat(baseFile); os.IsNotExist(err) {
							tmpBase, err := os.CreateTemp("", "phantom-empty-base-*")
							if err == nil {
								// defer removal
								defer func() { _ = os.Remove(tmpBase.Name()) }()
								_ = tmpBase.Close()
								baseFile = tmpBase.Name()
							}
						}

						hasConflict, err := gitOps.CheckFileConflict(ctx, ovl.BaseDir, baseFile, ourFile, theirFile)
						if err == nil {
							if hasConflict {
								isHard = true
								desc = "Hard Conflict (Git)"
							} else {
								isHard = false
								desc = "Clean Merge (Git)"
							}
						}
					}
				}
			} else {
				desc = "Hard Conflict (3+ Overlays)"
			}

			conflicts = append(conflicts, detailedConflict{
				File:        file,
				Overlays:    overlays,
				IsHard:      isHard,
				Description: desc,
			})
		}
	}

	if len(conflicts) == 0 {
		log.Info("No conflicts detected between %d overlays (Confidence: 100%%)", len(names))
		return nil
	}

	// Calculate confidence score
	score := 100
	for _, c := range conflicts {
		if c.IsHard {
			score -= 20
		} else {
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}

	if format == "json" {
		out := struct {
			Conflicts []detailedConflict `json:"conflicts"`
			Score     int                `json:"confidence_score"`
		}{
			Conflicts: conflicts,
			Score:     score,
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "STATUS\tFILE\tOVERLAYS")
	for _, c := range conflicts {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", c.Description, c.File, strings.Join(c.Overlays, ", "))
	}
	_ = w.Flush()
	fmt.Println()
	log.Info("%d overlapping file(s) detected", len(conflicts))
	log.Info("Merge Confidence Score: %d%%", score)

	if score < 50 {
		log.Info("Warning: Low confidence. Manual review highly recommended before running 'phantom apply'.")
	}
	return nil
}
