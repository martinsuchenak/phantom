package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewHealthCommand creates the health command
func NewHealthCommand() *cli.Command {
	return &cli.Command{
		Name:        "health",
		Usage:       "Check health of overlays and system",
		Description: "Verifies overlay mounts are healthy, detects zombie/stale mounts, and checks system prerequisites.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Output format (table, json)",
				DefaultValue: "table",
			},
			&cli.BoolFlag{
				Name:  "fix",
				Usage: "Attempt to fix detected issues (remount stale overlays, clean zombies)",
			},
		},
		Run: doHealth,
	}
}

type healthIssue struct {
	Overlay string `json:"overlay"`
	Kind    string `json:"kind"` // "stale_mount", "missing_upper", "missing_base", "zombie"
	Message string `json:"message"`
	Fixed   bool   `json:"fixed,omitempty"`
}

type healthReport struct {
	Platform string        `json:"platform"`
	FuseOK   bool          `json:"fuse_available"`
	Overlays int           `json:"total_overlays"`
	Healthy  int           `json:"healthy"`
	Issues   []healthIssue `json:"issues,omitempty"`
}

func doHealth(ctx context.Context, cmd *cli.Command) error {
	format := cmd.GetString("format")
	fix := cmd.GetBool("fix")

	return processHealth(ctx, format, fix)
}

func processHealth(ctx context.Context, format string, fix bool) error {
	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return fmt.Errorf("failed to create overlay manager: %w", err)
	}

	overlays, err := store.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load overlays: %w", err)
	}

	report := healthReport{
		Platform: runtime.GOOS,
		FuseOK:   checkFuseAvailable(),
		Overlays: len(overlays),
	}

	for _, ovl := range overlays {
		var issues []healthIssue

		// Check if base directory still exists
		if _, err := os.Stat(ovl.BaseDir); os.IsNotExist(err) {
			issues = append(issues, healthIssue{
				Overlay: ovl.Name,
				Kind:    "missing_base",
				Message: fmt.Sprintf("base directory missing: %s", ovl.BaseDir),
			})
		}

		// Check if upper directory exists
		if _, err := os.Stat(ovl.UpperDir); os.IsNotExist(err) {
			issues = append(issues, healthIssue{
				Overlay: ovl.Name,
				Kind:    "missing_upper",
				Message: fmt.Sprintf("upper directory missing: %s", ovl.UpperDir),
			})
		}

		// Check mount status
		mounted, err := mgr.IsMounted(ovl)
		if err != nil {
			issues = append(issues, healthIssue{
				Overlay: ovl.Name,
				Kind:    "mount_error",
				Message: fmt.Sprintf("failed to check mount: %v", err),
			})
		} else if !mounted {
			// Check if mount point dir has stale data (zombie)
			entries, _ := os.ReadDir(ovl.MountPoint)
			if len(entries) > 0 {
				issue := healthIssue{
					Overlay: ovl.Name,
					Kind:    "zombie",
					Message: "mount point has data but overlay is not mounted (stale/zombie)",
				}
				if fix {
					// Try to remount
					if err := mgr.Mount(ovl); err == nil {
						issue.Fixed = true
						issue.Message += " — remounted"
					} else {
						issue.Message += fmt.Sprintf(" — remount failed: %v", err)
					}
				}
				issues = append(issues, issue)
			} else {
				issue := healthIssue{
					Overlay: ovl.Name,
					Kind:    "stale_mount",
					Message: "overlay is not mounted",
				}
				if fix {
					if err := mgr.Mount(ovl); err == nil {
						issue.Fixed = true
						issue.Message += " — remounted"
					} else {
						issue.Message += fmt.Sprintf(" — remount failed: %v", err)
					}
				}
				issues = append(issues, issue)
			}
		}

		// Check PID liveness (macOS unionfs / fuse-overlayfs)
		if mounted && ovl.PID > 0 {
			proc, err := os.FindProcess(ovl.PID)
			if err != nil || !isProcessAlive(proc) {
				issues = append(issues, healthIssue{
					Overlay: ovl.Name,
					Kind:    "dead_pid",
					Message: fmt.Sprintf("FUSE process (PID %d) is not running but overlay appears mounted", ovl.PID),
				})
			}
		}

		if len(issues) == 0 {
			report.Healthy++
		}
		report.Issues = append(report.Issues, issues...)
	}

	return printHealthReport(report, format)
}

func checkFuseAvailable() bool {
	if runtime.GOOS == "darwin" {
		// Check for macFUSE or FUSE-T
		for _, path := range []string{"/Library/Filesystems/macfuse.fs", "/Library/Filesystems/fuse-t.fs"} {
			if _, err := os.Stat(path); err == nil {
				return true
			}
		}
		return false
	}
	// Linux: check for fuse-overlayfs or /dev/fuse
	if _, err := os.Stat("/dev/fuse"); err == nil {
		return true
	}
	return false
}

func isProcessAlive(proc *os.Process) bool {
	// On Unix, sending signal 0 checks if process exists
	err := proc.Signal(os.Signal(nil))
	return err == nil
}

func printHealthReport(report healthReport, format string) error {
	if format == "json" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	log.Info("Platform:  %s", report.Platform)
	if report.FuseOK {
		log.Info("FUSE:      available")
	} else {
		log.Info("FUSE:      not found")
	}
	log.Info("Overlays:  %d total, %d healthy", report.Overlays, report.Healthy)

	if len(report.Issues) == 0 {
		log.Info("No issues detected")
		return nil
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "OVERLAY\tISSUE\tDETAILS")
	for _, issue := range report.Issues {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", issue.Overlay, issue.Kind, issue.Message)
	}
	return w.Flush()
}
