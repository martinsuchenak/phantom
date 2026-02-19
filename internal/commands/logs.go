package commands

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/paularlott/cli"
)

// NewLogsCommand creates the logs command
func NewLogsCommand() *cli.Command {
	return &cli.Command{
		Name:        "logs",
		Usage:       "Show agent logs for an overlay",
		Description: "Displays the agent execution log for the specified overlay.",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:         "tail",
				Aliases:      []string{"n"},
				Usage:        "Show last N bytes (0 = entire file)",
				DefaultValue: 0,
			},
			&cli.BoolFlag{
				Name:  "path",
				Usage: "Print the log file path instead of contents",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "name",
				Usage:    "Name of the overlay",
				Required: true,
			},
		},
		Run: doLogs,
	}
}

func doLogs(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}
	tail := cmd.GetInt("tail")
	showPath := cmd.GetBool("path")

	return processLogs(name, tail, showPath)
}

func processLogs(name string, tail int, showPath bool) error {
	logPath := filepath.Join(cfg.GetLogsPath(), name+".log")

	if showPath {
		fmt.Println(logPath)
		return nil
	}

	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no logs found for overlay %q", name)
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	if tail > 0 {
		// Seek to tail bytes from end
		info, err := f.Stat()
		if err != nil {
			return err
		}
		offset := info.Size() - int64(tail)
		if offset > 0 {
			f.Seek(offset, io.SeekStart)
			// Skip partial first line
			buf := make([]byte, 1)
			for {
				_, err := f.Read(buf)
				if err != nil || buf[0] == '\n' {
					break
				}
			}
		}
	}

	_, err = io.Copy(os.Stdout, f)
	return err
}
