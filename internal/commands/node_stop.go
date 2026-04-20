package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/paularlott/cli"
)

func NewNodeStopCommand() *cli.Command {
	return &cli.Command{
		Name:        "stop",
		Usage:       "Stop the running node daemon",
		Description: "Sends SIGTERM to the node daemon process identified by the PID file.",
		Run:         doNodeStop,
	}
}

func doNodeStop(_ context.Context, _ *cli.Command) error {
	pidPath := cfg.GetNodePIDPath()

	data, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("node is not running (pid file not found)")
		}
		return fmt.Errorf("read pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid pid in %s: %w", pidPath, err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = os.Remove(pidPath)
		return fmt.Errorf("process %d is not running", pid)
	}

	if err := proc.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("send SIGTERM to process %d: %w", pid, err)
	}

	log.Info("Sent SIGTERM to node process %d", pid)
	return nil
}
