package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewMergeCommand creates the merge command
func NewMergeCommand() *cli.Command {
	return &cli.Command{
		Name:        "merge",
		Usage:       "Merge changes from one overlay into another",
		Description: "Copies file changes from the source overlay's upper directory into the target overlay. Detects conflicts before merging.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be merged without making changes",
			},
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Merge even if conflicts exist (target files are overwritten)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "source", Usage: "Source overlay name", Required: true},
			&cli.StringArg{Name: "target", Usage: "Target overlay name", Required: true},
		},
		Run: doMerge,
	}
}

type mergeAction struct {
	RelPath  string
	Status   string // "copy", "delete", "conflict"
	IsConflict bool
}

func doMerge(ctx context.Context, cmd *cli.Command) error {
	srcName := cmd.GetStringArg("source")
	dstName := cmd.GetStringArg("target")
	dryRun := cmd.GetBool("dry-run")
	force := cmd.GetBool("force")

	if srcName == dstName {
		return fmt.Errorf("source and target cannot be the same overlay")
	}

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	srcOvl, err := store.Load(srcName)
	if err != nil {
		return fmt.Errorf("source overlay: %w", err)
	}
	dstOvl, err := store.Load(dstName)
	if err != nil {
		return fmt.Errorf("target overlay: %w", err)
	}

	if srcOvl.UpperDir == "" {
		return fmt.Errorf("source overlay %q has no upper directory", srcName)
	}
	if dstOvl.UpperDir == "" {
		return fmt.Errorf("target overlay %q has no upper directory", dstName)
	}

	// Scan source changes
	actions, err := planMerge(srcOvl.UpperDir, dstOvl.UpperDir)
	if err != nil {
		return err
	}

	if len(actions) == 0 {
		log.Info("No changes to merge from %q", srcName)
		return nil
	}

	// Report conflicts
	conflicts := 0
	for _, a := range actions {
		if a.IsConflict {
			conflicts++
		}
	}

	if dryRun {
		return printMergePlan(actions, srcName, dstName, conflicts)
	}

	if conflicts > 0 && !force {
		printMergePlan(actions, srcName, dstName, conflicts)
		return fmt.Errorf("%d conflict(s) detected — use --force to overwrite or resolve manually", conflicts)
	}

	// Execute merge
	copied, deleted := 0, 0
	for _, a := range actions {
		srcPath := filepath.Join(srcOvl.UpperDir, a.RelPath)
		dstPath := filepath.Join(dstOvl.UpperDir, a.RelPath)

		if a.Status == "delete" {
			// Create whiteout in target
			dir := filepath.Dir(dstPath)
			base := filepath.Base(a.RelPath)
			whiteout := filepath.Join(dir, ".wh."+base)
			if err := os.MkdirAll(filepath.Dir(whiteout), 0755); err != nil {
				log.Warn("Failed to create dir for whiteout %s: %v", a.RelPath, err)
				continue
			}
			f, err := os.Create(whiteout)
			if err != nil {
				log.Warn("Failed to create whiteout for %s: %v", a.RelPath, err)
				continue
			}
			f.Close()
			deleted++
		} else {
			// Copy file
			srcInfo, err := os.Stat(srcPath)
			if err != nil {
				log.Warn("Failed to stat %s: %v", a.RelPath, err)
				continue
			}
			if err := copyFile(srcPath, dstPath, srcInfo.Mode()); err != nil {
				log.Warn("Failed to copy %s: %v", a.RelPath, err)
				continue
			}
			copied++
		}
	}

	log.Info("Merged %d file(s) from %q into %q (%d copied, %d deleted)", copied+deleted, srcName, dstName, copied, deleted)
	if conflicts > 0 {
		log.Warn("%d conflict(s) were overwritten (--force)", conflicts)
	}
	return nil
}

func planMerge(srcUpper, dstUpper string) ([]mergeAction, error) {
	var actions []mergeAction

	err := filepath.Walk(srcUpper, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		relPath, err := filepath.Rel(srcUpper, path)
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
			actions = append(actions, mergeAction{
				RelPath: actualPath,
				Status:  "delete",
			})
			return nil
		}

		// Check if target also modified this file
		dstPath := filepath.Join(dstUpper, relPath)
		isConflict := false
		if _, err := os.Stat(dstPath); err == nil {
			isConflict = true
		}

		actions = append(actions, mergeAction{
			RelPath:    relPath,
			Status:     "copy",
			IsConflict: isConflict,
		})
		return nil
	})

	return actions, err
}

func printMergePlan(actions []mergeAction, src, dst string, conflicts int) error {
	log.Info("Merge plan: %s → %s", src, dst)
	for _, a := range actions {
		prefix := "  +"
		if a.Status == "delete" {
			prefix = "  -"
		}
		suffix := ""
		if a.IsConflict {
			suffix = "  ⚠ CONFLICT"
		}
		fmt.Printf("%s %s%s\n", prefix, a.RelPath, suffix)
	}
	fmt.Println()
	log.Info("%d file(s) to merge, %d conflict(s)", len(actions), conflicts)
	return nil
}

