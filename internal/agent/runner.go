package agent

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/logger"
)

// Runner handles running agent commands in overlay contexts
type Runner struct {
	cfg    *config.Config
	gitOps *git.Operations
	log    logger.Logger
}

// NewRunner creates a new agent runner
func NewRunner(cfg *config.Config, log logger.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		gitOps: git.NewOperations(),
		log:    log,
	}
}

// Run executes an agent command in the overlay context
func (r *Runner) Run(ctx context.Context, ovl *api.Overlay, opts *api.RunOptions) (int, error) {
	// Create context with timeout
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Log start
	prefix := fmt.Sprintf("\033[1m[%s]\033[0m", ovl.Name)
	if opts.Progress != "" {
		prefix = fmt.Sprintf("\033[36m%s\033[0m %s", opts.Progress, prefix)
	}

	r.log.Info("%s Starting agent: %s", prefix, opts.Agent)
	r.log.Debug("%s Task: %s", prefix, opts.Task)
	r.log.Debug("%s Overlay: %s", prefix, ovl.MountPoint)

	// Build the command - parse agent string into command and args
	// This avoids shell injection by not using sh -c
	// Substitute {task} placeholder in agent command with the actual task
	agentCmd := opts.Agent
	if opts.Task != "" {
		agentCmd = strings.ReplaceAll(agentCmd, "{task}", opts.Task)
	}
	// Substitute {model} placeholder if provided; if not provided, remove any
	// flag+placeholder pair (e.g. "--model {model}") so templates work without --model.
	if opts.Model != "" {
		agentCmd = strings.ReplaceAll(agentCmd, "{model}", opts.Model)
	} else {
		// Remove common flag patterns that wrap {model} so the command stays valid
		agentCmd = strings.ReplaceAll(agentCmd, "--model {model}", "")
		agentCmd = strings.ReplaceAll(agentCmd, "-m {model}", "")
		// Remove bare placeholder in case it was used directly
		agentCmd = strings.ReplaceAll(agentCmd, "{model}", "")
		// Collapse any double spaces left behind
		for strings.Contains(agentCmd, "  ") {
			agentCmd = strings.ReplaceAll(agentCmd, "  ", " ")
		}
		agentCmd = strings.TrimSpace(agentCmd)
	}
	cmd := r.buildCommand(ctx, agentCmd)
	cmd.Dir = ovl.MountPoint

	// In headless mode (parallel runs), don't attach stdin.
	// If there's a task and no {task} placeholder was used, pipe it via stdin
	// so agents like "claude --print" can read it.
	if !opts.Headless {
		cmd.Stdin = os.Stdin
	} else if opts.Task != "" && !strings.Contains(opts.Agent, "{task}") {
		cmd.Stdin = strings.NewReader(opts.Task)
	}

	// Set up log file for agent output
	logFile, err := r.openLogFile(ovl.Name)
	if err != nil {
		r.log.Debug("Failed to create log file: %v", err)
		if opts.Headless {
			// In headless mode with no log file, discard output unless custom writers provided
			cmd.Stdout = io.Discard
			if opts.Stdout != nil {
				cmd.Stdout = opts.Stdout
			}
			cmd.Stderr = io.Discard
			if opts.Stderr != nil {
				cmd.Stderr = opts.Stderr
			}
		} else {
			cmd.Stdout = os.Stdout
			if opts.Stdout != nil {
				cmd.Stdout = opts.Stdout
			}
			cmd.Stderr = os.Stderr
			if opts.Stderr != nil {
				cmd.Stderr = opts.Stderr
			}
		}
	} else {
		defer logFile.Close()
		// Write header to log
		fmt.Fprintf(logFile, "=== Phantom Agent Log ===\n")
		fmt.Fprintf(logFile, "Overlay:  %s\n", ovl.Name)
		fmt.Fprintf(logFile, "Agent:    %s\n", opts.Agent)
		fmt.Fprintf(logFile, "Task:     %s\n", opts.Task)
		fmt.Fprintf(logFile, "Started:  %s\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(logFile, "=========================\n\n")

		if opts.Headless {
			// Headless: output goes to log file, plus custom writers if any
			if opts.Stdout != nil {
				cmd.Stdout = io.MultiWriter(opts.Stdout, logFile)
			} else {
				cmd.Stdout = logFile
			}
			if opts.Stderr != nil {
				cmd.Stderr = io.MultiWriter(opts.Stderr, logFile)
			} else {
				cmd.Stderr = logFile
			}
		} else {
			// Interactive: tee output to both terminal and log file
			stdoutWriters := []io.Writer{os.Stdout, logFile}
			if opts.Stdout != nil {
				stdoutWriters = []io.Writer{opts.Stdout, logFile}
			}
			cmd.Stdout = io.MultiWriter(stdoutWriters...)

			stderrWriters := []io.Writer{os.Stderr, logFile}
			if opts.Stderr != nil {
				stderrWriters = []io.Writer{opts.Stderr, logFile}
			}
			cmd.Stderr = io.MultiWriter(stderrWriters...)
		}
	}

	// Set environment variables
	cmd.Env = r.buildEnv(ovl, opts)

	// Run the command
	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	// Log completion
	var statusColor string
	if exitCode == 0 {
		statusColor = "\033[32m" // green
	} else {
		statusColor = "\033[31m" // red
	}
	r.log.Info("%s Agent completed in %s (%sexit code %d\033[0m)", prefix, duration.Round(time.Second), statusColor, exitCode)

	// Write footer to log file
	if logFile != nil {
		fmt.Fprintf(logFile, "\n=========================\n")
		fmt.Fprintf(logFile, "Finished: %s\n", time.Now().Format(time.RFC3339))
		fmt.Fprintf(logFile, "Duration: %s\n", duration.Round(time.Second))
		fmt.Fprintf(logFile, "Exit:     %d\n", exitCode)
	}

	// Handle git operations on completion
	if ovl.Branch != "" && opts.PushOnEnd {
		if exitCode == 0 || r.cfg.Agent.CleanupOnFailure {
			r.handleGitOperations(ctx, ovl, exitCode == 0)
		}
	}

	return exitCode, err
}

// buildCommand parses the agent string and creates an exec.Cmd
// It handles quoted arguments and avoids shell injection
func (r *Runner) buildCommand(ctx context.Context, agent string) *exec.Cmd {
	args := parseCommandLine(agent)
	if len(args) == 0 {
		// Fallback to empty command (will fail gracefully)
		return exec.CommandContext(ctx, "")
	}
	return exec.CommandContext(ctx, args[0], args[1:]...)
}

// parseCommandLine splits a command line string into arguments
// respecting quoted strings
func parseCommandLine(cmdLine string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range cmdLine {
		switch {
		case r == '"' || r == '\'':
			if inQuote && r == quoteChar {
				inQuote = false
				quoteChar = 0
			} else if !inQuote {
				inQuote = true
				quoteChar = r
			} else {
				current.WriteRune(r)
			}
		case r == ' ' || r == '\t':
			if inQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

// buildEnv builds the environment variables for the agent
func (r *Runner) buildEnv(ovl *api.Overlay, opts *api.RunOptions) []string {
	env := os.Environ()

	// Core overlay variables
	env = append(env,
		fmt.Sprintf("OVERLAY_NAME=%s", ovl.Name),
		fmt.Sprintf("OVERLAY_PATH=%s", ovl.MountPoint),
		fmt.Sprintf("OVERLAY_BASE=%s", ovl.BaseDir),
		fmt.Sprintf("OVERLAY_BRANCH=%s", ovl.Branch),
		fmt.Sprintf("OVERLAY_TASK=%s", opts.Task),
		fmt.Sprintf("OVERLAY_MODEL=%s", opts.Model),
	)

	// Add configured agent env vars
	for _, e := range r.cfg.AgentEnv {
		env = append(env, e)
	}

	return env
}

// handleGitOperations handles git operations after agent completion
func (r *Runner) handleGitOperations(ctx context.Context, ovl *api.Overlay, success bool) {
	// Check for changes
	hasChanges, err := r.gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
	if err != nil {
		r.log.Debug("Failed to check for changes: %v", err)
		return
	}

	if !hasChanges {
		r.log.Debug("No changes to commit")
		return
	}

	// Commit changes
	commitMsg := fmt.Sprintf("Agent changes: %s", ovl.Branch)
	if err := r.gitOps.CommitAll(ctx, ovl.MountPoint, commitMsg); err != nil {
		r.log.Debug("Failed to commit changes: %v", err)
		return
	}

	r.log.Debug("Committed agent changes")

	// Push if requested
	if r.cfg.Git.AutoPushOnStop {
		if err := r.gitOps.PushBranch(ctx, ovl.MountPoint, ovl.Branch, false); err != nil {
			r.log.Debug("Failed to push branch: %v", err)
		} else {
			r.log.Debug("Pushed branch to remote")
		}
	}
}

// openLogFile creates or appends to the log file for an overlay's agent run
func (r *Runner) openLogFile(name string) (*os.File, error) {
	logsDir := r.cfg.GetLogsPath()
	if err := os.MkdirAll(logsDir, 0700); err != nil {
		return nil, err
	}

	logPath := filepath.Join(logsDir, name+".log")
	return os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
}
