package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

func NewInspectCommand() *cli.Command {
	return &cli.Command{
		Name:  "inspect",
		Usage: "Show detailed information about an overlay",
		Description: "Dumps everything about an overlay: state, mount status, " +
			"git info, file changes, snapshots, and log tail.",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "format", Usage: "Output format (text, json)", DefaultValue: "text"},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
		},
		Run: doInspect,
	}
}

type inspectOutput struct {
	Name         string `json:"name"`
	BaseDir      string `json:"base_dir"`
	MountPoint   string `json:"mount_point"`
	UpperDir     string `json:"upper_dir"`
	Branch       string `json:"branch"`
	Persistent   bool   `json:"persistent"`
	Locked       bool   `json:"locked"`
	PinnedCommit string `json:"pinned_commit,omitempty"`
	CreatedAt    string `json:"created_at"`
	Mounted      bool   `json:"mounted"`
	Uptime       string `json:"uptime"`
	SizeBytes    int64  `json:"size_bytes"`
	PID          int    `json:"pid,omitempty"`
	FilesAdded   int    `json:"files_added"`
	FilesMod     int    `json:"files_modified"`
	FilesDel     int    `json:"files_deleted"`
	GitBranch    string `json:"git_branch,omitempty"`
	GitDirty     bool   `json:"git_dirty,omitempty"`
	Snapshots    int    `json:"snapshots"`
	HasLog       bool   `json:"has_log"`
}

func doInspect(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	format := cmd.GetString("format")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}
	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	mgr, err := createOverlayManager()
	if err != nil {
		return err
	}
	status, _ := mgr.GetStatus(ovl)

	added, modified, deleted := countFileChanges(ovl.UpperDir, ovl.BaseDir)

	// Count snapshots for this overlay
	snapCount := 0
	snapshotsDir := cfg.GetSnapshotsPath()
	if entries, err := os.ReadDir(snapshotsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			data, err := os.ReadFile(filepath.Join(snapshotsDir, e.Name(), "meta.json"))
			if err != nil {
				continue
			}
			var meta snapshotMeta
			if err := json.Unmarshal(data, &meta); err == nil && meta.Overlay == name {
				snapCount++
			}
		}
	}

	// Check log file
	logPath := filepath.Join(cfg.GetLogsPath(), name+".log")
	hasLog := false
	if _, err := os.Stat(logPath); err == nil {
		hasLog = true
	}

	// Git info
	gitOps := git.NewOperations()
	gitBranch := ""
	gitDirty := false
	if status != nil && status.Mounted {
		if branch, err := gitOps.GetCurrentBranch(ctx, ovl.MountPoint); err == nil {
			gitBranch = branch
		}
		if dirty, err := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint); err == nil {
			gitDirty = dirty
		}
	}

	out := inspectOutput{
		Name:         ovl.Name,
		BaseDir:      ovl.BaseDir,
		MountPoint:   ovl.MountPoint,
		UpperDir:     ovl.UpperDir,
		Branch:       ovl.Branch,
		Persistent:   ovl.Persistent,
		Locked:       ovl.Locked,
		PinnedCommit: ovl.PinnedCommit,
		CreatedAt:    ovl.CreatedAt.Format("2006-01-02 15:04:05"),
		PID:          ovl.PID,
		FilesAdded:   added,
		FilesMod:     modified,
		FilesDel:     deleted,
		GitBranch:    gitBranch,
		GitDirty:     gitDirty,
		Snapshots:    snapCount,
		HasLog:       hasLog,
	}
	if status != nil {
		out.Mounted = status.Mounted
		out.Uptime = formatDuration(status.Uptime)
		out.SizeBytes = status.SizeBytes
	}

	if format == "json" {
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Name:         %s\n", out.Name)
	fmt.Printf("Base Dir:     %s\n", out.BaseDir)
	fmt.Printf("Mount Point:  %s\n", out.MountPoint)
	fmt.Printf("Upper Dir:    %s\n", out.UpperDir)
	fmt.Printf("Branch:       %s\n", out.Branch)
	fmt.Printf("Persistent:   %v\n", out.Persistent)
	fmt.Printf("Locked:       %v\n", out.Locked)
	if out.PinnedCommit != "" {
		pinShort := out.PinnedCommit
		if len(pinShort) > 10 {
			pinShort = pinShort[:10]
		}
		fmt.Printf("Pinned:       %s\n", pinShort)
	}
	fmt.Printf("Created:      %s\n", out.CreatedAt)
	fmt.Printf("Mounted:      %v\n", out.Mounted)
	fmt.Printf("Uptime:       %s\n", out.Uptime)
	fmt.Printf("Data Size:    %s\n", formatSize(out.SizeBytes))
	if out.PID > 0 {
		fmt.Printf("FUSE PID:     %d\n", out.PID)
	}
	fmt.Printf("Changes:      %d added, %d modified, %d deleted\n", out.FilesAdded, out.FilesMod, out.FilesDel)
	if out.GitBranch != "" {
		fmt.Printf("Git Branch:   %s\n", out.GitBranch)
		fmt.Printf("Git Dirty:    %v\n", out.GitDirty)
	}
	fmt.Printf("Snapshots:    %d\n", out.Snapshots)
	if out.HasLog {
		fmt.Printf("Log:          %s\n", logPath)
	} else {
		fmt.Printf("Log:          none\n")
	}
	return nil
}
