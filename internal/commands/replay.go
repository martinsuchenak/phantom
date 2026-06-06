package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/paularlott/cli"
)

// NewReplayCommand creates the replay command
func NewReplayCommand() *cli.Command {
	return &cli.Command{
		Name:        "replay",
		Usage:       "Re-run the last agent command in an overlay",
		Description: "Reads the agent command and task from the overlay's log file and re-runs it. The overlay must be mounted.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show what would be run without executing",
			},
			&cli.IntFlag{
				Name:         "timeout",
				Usage:        "Timeout in minutes (default: from config)",
				DefaultValue: 0,
			},
			&cli.BoolFlag{
				Name:  "cleanup",
				Usage: "Cleanup overlay after completion",
			},
			&cli.BoolFlag{
				Name:  "push",
				Usage: "Push branch to remote on completion",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay",
				Required: true,
			},
		},
		Run: doReplay,
	}
}

// lastRunInfo holds parsed info from the log header
type lastRunInfo struct {
	Agent string
	Task  string
}

func doReplay(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	dryRun := cmd.GetBool("dry-run")
	timeoutMinutes := cmd.GetInt("timeout")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")

	store, err := state.NewStore(cfg.GetStatePath())
	if err != nil {
		return fmt.Errorf("failed to initialize state store: %w", err)
	}

	ovl, err := store.Load(name)
	if err != nil {
		return err
	}

	// Parse last run from log
	info, err := parseLastRun(name)
	if err != nil {
		return err
	}

	if info.Agent == "" {
		return fmt.Errorf("no previous agent command found in logs for %q", name)
	}

	if dryRun {
		fmt.Printf("Would replay in overlay %q:\n", name)
		fmt.Printf("  Agent: %s\n", info.Agent)
		fmt.Printf("  Task:  %s\n", info.Task)
		return nil
	}

	// Re-run using processRun (reuses existing overlay)
	exitCode, err := processRun(ctx, info.Agent, info.Task, "", ovl.BaseDir, name, "", timeoutMinutes, doCleanup, doPush, false)
	if err != nil {
		return err
	}
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}

// parseLastRun reads the log file and extracts the most recent agent/task from headers
func parseLastRun(name string) (*lastRunInfo, error) {
	logPath := filepath.Join(cfg.GetLogsPath(), name+".log")

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no logs found for overlay %q — run an agent first", name)
		}
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Scan for the last occurrence of Agent/Task headers
	// Log format:
	//   === Phantom Agent Log ===
	//   Overlay:  name
	//   Agent:    command
	//   Task:     description
	//   Started:  timestamp
	//   =========================
	var info lastRunInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Agent:") {
			info.Agent = strings.TrimSpace(strings.TrimPrefix(line, "Agent:"))
		} else if strings.HasPrefix(line, "Task:") {
			info.Task = strings.TrimSpace(strings.TrimPrefix(line, "Task:"))
		}
	}

	return &info, scanner.Err()
}
