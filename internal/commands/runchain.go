package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/martinsuchenak/phantom/internal/agent"
	"github.com/martinsuchenak/phantom/internal/config"
	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
	"github.com/paularlott/cli/tui"
	"gopkg.in/yaml.v3"
)

// chainConfig is the YAML config for sequential agent chaining
type chainConfig struct {
	Name   string      `yaml:"name"`
	Branch string      `yaml:"branch"`
	Steps  []chainStep `yaml:"steps"`
}

// chainStep defines a single step in the chain
type chainStep struct {
	Name    string `yaml:"name" json:"name"`
	Agent   string `yaml:"agent" json:"agent"`
	Task    string `yaml:"task" json:"task"`
	Model   string `yaml:"model" json:"model"`     // optional: substituted as {model} in agent command
	Timeout int    `yaml:"timeout" json:"timeout"` // minutes, 0 = use global
}

// NewRunChainCommand creates the run-chain command
func NewRunChainCommand() *cli.Command {
	return &cli.Command{
		Name:        "run-chain",
		Usage:       "Run agents sequentially on a single overlay",
		Description: "Creates one overlay and runs agents in sequence. Each agent builds on the previous agent's work. Stops on first failure unless --continue-on-error is set.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to chain YAML config file",
			},
			&cli.StringFlag{
				Name:  "steps",
				Usage: "Comma-separated agent commands (simple mode)",
			},
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Overlay name (auto-generated if not specified)",
			},
			&cli.StringFlag{
				Name:    "branch",
				Aliases: []string{"b"},
				Usage:   "Git branch name",
			},
			&cli.IntFlag{
				Name:         "timeout",
				Usage:        "Global timeout per step in minutes (max 1440)",
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
			&cli.BoolFlag{
				Name:  "continue-on-error",
				Usage: "Continue running remaining steps even if one fails",
			},
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Summary output format (table, json)",
				DefaultValue: "table",
			},
			&cli.StringFlag{
				Name:    "model",
				Aliases: []string{"m"},
				Usage:   "Model to use (substituted as {model} in step agent commands; overrides per-step model)",
				EnvVars: []string{"OVERLAY_MODEL"},
			},
			&cli.StringFlag{
				Name:  "only",
				Usage: "Run only the step with this name",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Start running from the step with this name",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "base-dir",
				Usage:    "Base directory to overlay",
				Required: true,
			},
		},
		Run: doRunChain,
	}
}

func doRunChain(ctx context.Context, cmd *cli.Command) error {
	baseDir := resolveBaseDir(cmd.GetStringArg("base-dir"))
	configPath := cmd.GetString("config")
	stepsInline := cmd.GetString("steps")
	name := cmd.GetString("name")
	branch := cmd.GetString("branch")
	timeoutMinutes := cmd.GetInt("timeout")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	continueOnError := cmd.GetBool("continue-on-error")
	format := cmd.GetString("format")
	modelOverride := cmd.GetString("model")

	if configPath == "" && stepsInline == "" {
		return fmt.Errorf("either --config or --steps is required")
	}

	var steps []chainStep
	var chainName, chainBranch string

	if configPath != "" {
		cc, err := loadChainConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load chain config: %w", err)
		}
		steps = cc.Steps
		chainName = cc.Name
		chainBranch = cc.Branch
	} else {
		steps = parseInlineSteps(stepsInline)
	}

	// CLI flags override config values
	if name != "" {
		chainName = name
	}
	if branch != "" {
		chainBranch = branch
	}

	if chainName == "" {
		chainName = fmt.Sprintf("chain-%d", time.Now().Unix())
	}

	if len(steps) == 0 {
		return fmt.Errorf("no steps defined")
	}

	onlyStep := cmd.GetString("only")
	fromStep := cmd.GetString("from")

	steps, err := filterSteps(steps, onlyStep, fromStep)
	if err != nil {
		return err
	}

	// Apply CLI model override to all steps
	if modelOverride != "" {
		for i := range steps {
			steps[i].Model = modelOverride
		}
	}

	return processRunChain(ctx, baseDir, chainName, chainBranch, steps, timeoutMinutes, doCleanup, doPush, continueOnError, format)
}

func loadChainConfig(path string) (*chainConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cc chainConfig
	if err := yaml.Unmarshal(data, &cc); err != nil {
		return nil, err
	}

	for i, s := range cc.Steps {
		if s.Agent == "" {
			return nil, fmt.Errorf("step[%d]: 'agent' command is required", i)
		}
		if s.Name == "" {
			cc.Steps[i].Name = fmt.Sprintf("step-%d", i+1)
		}
	}

	return &cc, nil
}

func parseInlineSteps(inline string) []chainStep {
	parts := parseInlineAgents(inline) // reuse from runall.go
	steps := make([]chainStep, len(parts))
	for i, def := range parts {
		steps[i] = chainStep{
			Name:  def.Name,
			Agent: def.Agent,
		}
	}
	return steps
}

// filterSteps filters steps based on --only and --from flags
func filterSteps(steps []chainStep, onlyStep, fromStep string) ([]chainStep, error) {
	if onlyStep != "" && fromStep != "" {
		return nil, fmt.Errorf("cannot use both --only and --from")
	}

	if onlyStep != "" {
		var filtered []chainStep
		for _, s := range steps {
			if s.Name == onlyStep {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("step %q not found in config", onlyStep)
		}
		return filtered, nil
	} else if fromStep != "" {
		found := false
		for i, s := range steps {
			if s.Name == fromStep {
				steps = steps[i:]
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("step %q not found in config", fromStep)
		}
		return steps, nil
	}

	return steps, nil
}

func processRunChain(ctx context.Context, baseDir, name, branch string, steps []chainStep, globalTimeout int, doCleanup, doPush, continueOnError bool, format string) error {
	var results []agentResult
	stopped := false

	logic := func(childCtx context.Context, t *tui.TUI) error {
		absBaseDir, err := filepath.Abs(baseDir)
		if err != nil {
			return fmt.Errorf("failed to resolve base directory: %w", err)
		}

		runAutoCleanup()

		log.Info("Starting chain %q with %d step(s) on %s", name, len(steps), absBaseDir)

		store, err := state.NewStore(cfg.GetStatePath())
		if err != nil {
			return fmt.Errorf("failed to initialize state store: %w", err)
		}

		mgr, err := createOverlayManager()
		if err != nil {
			return err
		}

		gitOps := git.NewOperations()
		isGit, _ := gitOps.IsGitRepo(ctx, absBaseDir)

		if branch == "" && isGit && cfg.Git.AutoBranch {
			branch = cfg.Git.BranchPrefix + name
		}

		// Create or reuse the single overlay
		var ovl *api.Overlay
		if store.Exists(name) {
			ovl, err = store.Load(name)
			if err != nil {
				return fmt.Errorf("failed to load existing overlay: %w", err)
			}
			mounted, merr := mgr.IsMounted(ovl)
			if merr != nil {
				return merr
			}
			if !mounted {
				if err := mgr.Mount(ovl); err != nil {
					return fmt.Errorf("failed to mount existing overlay: %w", err)
				}
			}
			log.Info("Reusing existing overlay %q", name)
		} else {
			opts := &api.CreateOptions{
				Name:    name,
				BaseDir: absBaseDir,
				Branch:  branch,
			}
			ovl, err = mgr.Create(opts)
			if err != nil {
				return fmt.Errorf("failed to create overlay: %w", err)
			}

			if isGit && branch != "" {
				branchExists, _ := gitOps.BranchExists(childCtx, absBaseDir, branch)
				if !branchExists {
					log.Info("Creating branch %q", branch)
					if err := gitOps.CreateBranch(childCtx, ovl.MountPoint, branch, ""); err != nil {
						log.Warn("Failed to create branch %q: %v", branch, err)
					}
				} else {
					log.Info("Switching to branch %q", branch)
					if err := gitOps.SwitchBranch(childCtx, ovl.MountPoint, branch); err != nil {
						log.Warn("Failed to switch to branch %q: %v", branch, err)
					}
				}
			}

			if err := store.Save(ovl); err != nil {
				mgr.Cleanup(ovl)
				return fmt.Errorf("failed to save overlay state: %w", err)
			}
			log.Debug("Created overlay %q at %s", name, ovl.MountPoint)
		}

		results = make([]agentResult, 0, len(steps))
		for i, step := range steps {
			if childCtx.Err() != nil {
				log.Warn("Chain interrupted by user")
				stopped = true
				break
			}

			if t != nil {
				t.SetProgress("Running chain...", float64(i)/float64(len(steps)))
			}

			progressStr := fmt.Sprintf("[%d/%d]", i+1, len(steps))

			log.Info("%s Running step %q: %s", progressStr, step.Name, step.Agent)

			results = append(results, runChainStep(childCtx, step, ovl, absBaseDir, globalTimeout, t, progressStr))
			result := results[len(results)-1]

			if result.ExitCode != 0 || result.Error != "" {
				log.Warn("%s Step %q failed (exit %d)", progressStr, step.Name, result.ExitCode)
				if !continueOnError {
					stopped = true
					break
				}
			}
		}

		// Push if requested and at least one step succeeded
		if doPush && ovl.Branch != "" {
			hasChanges, _ := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
			if hasChanges {
				commitMsg := fmt.Sprintf("phantom chain: %s", name)
				if err := gitOps.CommitAll(ctx, ovl.MountPoint, commitMsg); err != nil {
					log.Warn("Failed to commit before push: %v", err)
				}
			}
			if err := gitOps.PushBranch(ctx, ovl.MountPoint, ovl.Branch, false); err != nil {
				log.Warn("Failed to push branch: %v", err)
			}
		}

		// If not cleaning up, clear the stale FUSE PID from state so that
		// `phantom health` does not flag the overlay as unhealthy after the
		// run completes. The unionfs-fuse process exits once the mount is no
		// longer actively used; keeping the old PID around causes a false
		// dead_pid alarm.
		if !doCleanup && ovl.PID > 0 {
			ovl.PID = 0
			if err := store.Save(ovl); err != nil {
				log.Warn("Failed to update overlay state after chain completion: %v", err)
			}
		}

		// Cleanup if requested
		if doCleanup {
			log.Debug("Cleaning up overlay %q", name)
			if err := mgr.Cleanup(ovl); err != nil {
				log.Warn("Failed to cleanup: %v", err)
			}
			store.Delete(name)
		}

		return nil
	}

	if err := logic(ctx, nil); err != nil {
		return err
	}

	// Print summary
	if err := printChainSummary(results, format, stopped); err != nil {
		return err
	}

	// Return an error so the CLI framework exits with a non-zero code
	for _, r := range results {
		if r.ExitCode != 0 || r.Error != "" {
			return fmt.Errorf("one or more chain steps failed")
		}
	}

	return nil
}

func runChainStep(ctx context.Context, step chainStep, ovl *api.Overlay, absBaseDir string, globalTimeout int, t *tui.TUI, progress string) agentResult {
	result := agentResult{
		Name:  step.Name,
		Agent: step.Agent,
	}

	timeoutMinutes := step.Timeout
	if timeoutMinutes <= 0 {
		timeoutMinutes = globalTimeout
	}
	var timeout time.Duration
	if timeoutMinutes > 0 {
		if timeoutMinutes > config.MaxTimeoutMinutes {
			timeoutMinutes = config.MaxTimeoutMinutes
		}
		timeout = time.Duration(timeoutMinutes) * time.Minute
	} else if cfg != nil && cfg.Agent.DefaultTimeoutMinutes > 0 {
		timeout = time.Duration(cfg.Agent.DefaultTimeoutMinutes) * time.Minute
	} else {
		timeout = 60 * time.Minute
	}

	runner := agent.NewRunner(cfg, log)

	runOpts := &api.RunOptions{
		Agent:    step.Agent,
		Task:     step.Task,
		Model:    step.Model,
		BaseDir:  absBaseDir,
		Name:     ovl.Name,
		Timeout:  timeout,
		Headless: true,
		Progress: progress,
	}

	if t != nil {
		w := NewTUIWriter(t, step.Name, true)
		defer w.Close()
		runOpts.Stdout = w
		runOpts.Stderr = w
	}

	startTime := time.Now()
	exitCode, err := runner.Run(ctx, ovl, runOpts)
	result.Duration = time.Since(startTime)
	result.ExitCode = exitCode

	if err != nil {
		result.Error = err.Error()
	}

	// Execute post-run hooks
	ExecuteHooks(step.Name, ovl.MountPoint, absBaseDir, ovl.Branch, step.Agent, step.Task, exitCode)

	return result
}

func printChainSummary(results []agentResult, format string, stoppedEarly bool) error {
	if format == "json" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println()
	log.Info("=== Chain Summary ===")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "STEP\tCOMMAND\tEXIT\tDURATION\tSTATUS")

	failed := 0
	for _, r := range results {
		status := "\033[32mok\033[0m"
		if r.ExitCode != 0 {
			status = "\033[31mFAILED\033[0m"
			failed++
		}
		if r.Error != "" && r.ExitCode == 0 {
			status = "\033[33mERROR\033[0m"
			failed++
		}
		if r.ExitCode == -1 {
			status = "\033[33mSKIPPED\033[0m"
			failed++
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\n",
			r.Name,
			r.Agent,
			r.ExitCode,
			formatDuration(r.Duration),
			status,
		)
	}
	w.Flush()

	fmt.Println()
	if stoppedEarly {
		log.Info("Chain stopped early after %d/%d step(s) — %d failed", len(results), len(results), failed)
	} else if failed > 0 {
		log.Info("%d/%d step(s) failed", failed, len(results))
	} else {
		log.Info("All %d step(s) completed successfully", len(results))
	}

	return nil
}
