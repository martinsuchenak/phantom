package commands

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewExportCommand creates the export command
func NewExportCommand() *cli.Command {
	return &cli.Command{
		Name:  "export",
		Usage: "Export overlay changes as a patch or tarball",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Usage:   "Output file path (default: stdout for diff, required for tar)",
			},
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Export format: diff, tar",
				DefaultValue: "diff",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doExport,
	}
}

func doExport(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	output := cmd.GetString("output")
	format := cmd.GetString("format")

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

	switch format {
	case "tar":
		if output == "" {
			return fmt.Errorf("--output is required for tar format")
		}
		return exportTar(ovl.UpperDir, ovl.BaseDir, output)
	case "diff":
		return exportDiff(ovl.UpperDir, ovl.BaseDir, output)
	default:
		return fmt.Errorf("unknown format %q (use: diff, tar)", format)
	}
}

func exportDiff(upperDir, baseDir, output string) error {
	var w io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	count := 0
	err := filepath.Walk(upperDir, func(path string, info os.FileInfo, err error) error {
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
			deletedPath := filepath.Join(filepath.Dir(relPath), deletedName)
			basePath := filepath.Join(baseDir, deletedPath)
			if baseContent, err := os.ReadFile(basePath); err == nil {
				writeUnifiedDiff(w, deletedPath, string(baseContent), "")
				count++
			}
			return nil
		}

		newContent, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		basePath := filepath.Join(baseDir, relPath)
		oldContent := ""
		if data, err := os.ReadFile(basePath); err == nil {
			oldContent = string(data)
		}

		writeUnifiedDiff(w, relPath, oldContent, string(newContent))
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk upper directory: %w", err)
	}

	if output != "" {
		log.Info("Exported %d file(s) to %s", count, output)
	}
	return nil
}

func writeUnifiedDiff(w io.Writer, path, oldContent, newContent string) {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	if oldContent == "" {
		fmt.Fprintf(w, "--- /dev/null\n")
		fmt.Fprintf(w, "+++ b/%s\n", path)
		fmt.Fprintf(w, "@@ -0,0 +1,%d @@\n", len(newLines))
		for _, line := range newLines {
			fmt.Fprintf(w, "+%s\n", line)
		}
		return
	}

	if newContent == "" {
		fmt.Fprintf(w, "--- a/%s\n", path)
		fmt.Fprintf(w, "+++ /dev/null\n")
		fmt.Fprintf(w, "@@ -1,%d +0,0 @@\n", len(oldLines))
		for _, line := range oldLines {
			fmt.Fprintf(w, "-%s\n", line)
		}
		return
	}

	// Simple full-file diff for modified files
	fmt.Fprintf(w, "--- a/%s\n", path)
	fmt.Fprintf(w, "+++ b/%s\n", path)
	fmt.Fprintf(w, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for _, line := range oldLines {
		fmt.Fprintf(w, "-%s\n", line)
	}
	for _, line := range newLines {
		fmt.Fprintf(w, "+%s\n", line)
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func exportTar(upperDir, baseDir, output string) error {
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	var tw *tar.Writer
	if strings.HasSuffix(output, ".gz") || strings.HasSuffix(output, ".tgz") {
		gw := gzip.NewWriter(f)
		defer gw.Close()
		tw = tar.NewWriter(gw)
	} else {
		tw = tar.NewWriter(f)
	}
	defer tw.Close()

	count := 0
	err = filepath.Walk(upperDir, func(path string, info os.FileInfo, err error) error {
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
			return nil // skip whiteouts in tar
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()
		io.Copy(tw, file)
		count++
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk upper directory: %w", err)
	}

	log.Info("Exported %d file(s) to %s", count, output)
	return nil
}
