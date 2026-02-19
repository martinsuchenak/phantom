package commands

import (
	"context"
	"fmt"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/paularlott/cli"
)

func NewConfigCommand() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "Configuration management",
		Commands: []*cli.Command{
			{
				Name:  "validate",
				Usage: "Validate configuration file",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "path", Aliases: []string{"p"}, Usage: "Config file path (default: ~/.phantom/config.yaml)"},
				},
				Run: doConfigValidate,
			},
			{
				Name:  "show",
				Usage: "Show current configuration",
				Run:   doConfigShow,
			},
		},
	}
}

func doConfigValidate(ctx context.Context, cmd *cli.Command) error {
	path := cmd.GetString("path")
	_, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("configuration invalid: %w", err)
	}
	log.Info("Configuration is valid")
	return nil
}

func doConfigShow(ctx context.Context, cmd *cli.Command) error {
	fmt.Printf("state_dir:       %s\n", cfg.StateDir)
	fmt.Printf("overlays_path:   %s\n", cfg.GetOverlaysPath())
	fmt.Printf("mounts_path:     %s\n", cfg.GetMountPath())
	fmt.Printf("logs_path:       %s\n", cfg.GetLogsPath())
	fmt.Printf("snapshots_path:  %s\n", cfg.GetSnapshotsPath())
	fmt.Printf("log_level:       %s\n", cfg.Logging.Level)
	fmt.Printf("log_file:        %s\n", cfg.Logging.File)
	fmt.Printf("auto_cleanup:    %d days\n", cfg.Overlay.AutoCleanupDays)
	fmt.Printf("auto_branch:     %v\n", cfg.Git.AutoBranch)
	fmt.Printf("branch_prefix:   %s\n", cfg.Git.BranchPrefix)
	fmt.Printf("default_timeout: %d min\n", cfg.Agent.DefaultTimeoutMinutes)
	return nil
}
