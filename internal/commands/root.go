package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/paularlott/cli"
	"github.com/paularlott/cli/env"
	"github.com/paularlott/logger"
)

var (
	cfg        *config.Config
	cfgPath    string
	verbose    bool
	log        logger.Logger
	version    = "0.1.0"
	commit     = ""
	date       = ""
)

// SetVersion sets version info from main
func SetVersion(v, c, d string) {
	version = v
	commit = c
	date = d
}

// Execute runs the CLI application
func Execute() {
	// Load .env file if present
	_ = env.Load() // Ignore error if .env doesn't exist

	versionStr := version
	if commit != "" && commit != "none" {
		versionStr = fmt.Sprintf("%s (%s)", version, commit[:7])
	}

	app := &cli.Command{
		Name:        "phantom",
		Version:     versionStr,
		Usage:       "Manage overlay filesystems for parallel AI agent development",
		Description: "A CLI tool for managing overlay filesystems to enable multiple AI agents to work on the same codebase in parallel without conflicts.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "config",
				Aliases:      []string{"c"},
				Usage:        "Config file path",
				EnvVars:      []string{"OVERLAY_CONFIG"},
				AssignTo:     &cfgPath,
				DefaultValue: "",
			},
			&cli.BoolFlag{
				Name:         "verbose",
				Aliases:      []string{"V"},
				Usage:        "Verbose output",
				AssignTo:     &verbose,
				Global:       true,
			},
		},
		Commands: []*cli.Command{
			NewStartCommand(),
			NewStopCommand(),
			NewListCommand(),
			NewStatusCommand(),
			NewRunCommand(),
			NewDiffCommand(),
			NewPruneCommand(),
			NewCommitCommand(),
			NewApplyCommand(),
		},
		PreRun: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			var err error
			cfg, err = config.Load(cfgPath)
			if err != nil {
				return ctx, fmt.Errorf("failed to load config: %w", err)
			}

			// Initialize logger
			log = NewCLILogger(verbose)

			return ctx, nil
		},
	}

	if err := app.Execute(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// CLILogger implements logger.Logger using stdout
type CLILogger struct {
	verbose bool
}

// NewCLILogger creates a new CLI logger
func NewCLILogger(verbose bool) *CLILogger {
	return &CLILogger{verbose: verbose}
}

func (l *CLILogger) Trace(msg string, keysAndValues ...any) {
	if l.verbose {
		fmt.Printf("[TRACE] "+msg+"\n", keysAndValues...)
	}
}

func (l *CLILogger) Debug(msg string, keysAndValues ...any) {
	if l.verbose {
		fmt.Printf("[DEBUG] "+msg+"\n", keysAndValues...)
	}
}

func (l *CLILogger) Info(msg string, keysAndValues ...any) {
	fmt.Printf(msg+"\n", keysAndValues...)
}

func (l *CLILogger) Warn(msg string, keysAndValues ...any) {
	fmt.Printf("[WARN] "+msg+"\n", keysAndValues...)
}

func (l *CLILogger) Error(msg string, keysAndValues ...any) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+msg+"\n", keysAndValues...)
}

func (l *CLILogger) Fatal(msg string, keysAndValues ...any) {
	fmt.Fprintf(os.Stderr, "[FATAL] "+msg+"\n", keysAndValues...)
	os.Exit(1)
}

func (l *CLILogger) With(key string, value any) logger.Logger {
	return l
}

func (l *CLILogger) WithError(err error) logger.Logger {
	return l
}

func (l *CLILogger) WithGroup(group string) logger.Logger {
	return l
}
