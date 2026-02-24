package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// NewRunAllCommand creates the run-all command
func NewRunAllCommand() *cli.Command {
	return &cli.Command{
		Name:        "run-all",
		Usage:       "Run multiple agents in parallel",
		Description: "Creates separate overlays and runs multiple agents concurrently on the same codebase. Define agents via a YAML config file or inline.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to agents YAML config file",
			},
			&cli.StringFlag{
				Name:  "agents",
				Usage: "Comma-separated agent commands (simple mode)",
			},
			&cli.IntFlag{
				Name:         "timeout",
				Usage:        "Global timeout per agent in minutes (max 1440)",
				DefaultValue: 0,
			},
			&cli.IntFlag{
				Name:         "concurrency",
				Aliases:      []string{"j"},
				Usage:        "Max concurrency per agent executable type (0 = unlimited)",
				DefaultValue: 0,
			},
			&cli.BoolFlag{
				Name:  "cleanup",
				Usage: "Cleanup all overlays after completion",
			},
			&cli.BoolFlag{
				Name:  "push",
				Usage: "Push branches to remote on completion",
			},
			&cli.StringFlag{
				Name:         "format",
				Usage:        "Summary output format (table, json)",
				DefaultValue: "table",
			},
			&cli.StringFlag{
				Name:    "model",
				Aliases: []string{"m"},
				Usage:   "Model to use (substituted as {model} in agent commands; overrides per-agent model)",
				EnvVars: []string{"OVERLAY_MODEL"},
			},
			&cli.StringFlag{
				Name:  "only",
				Usage: "Run only the agent with this name",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Start running from the agent with this name",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "base-dir",
				Usage:    "Base directory to overlay",
				Required: true,
			},
		},
		Run: doRunAll,
	}
}

// agentDef defines a single agent in the config file
type agentDef struct {
	Name    string `yaml:"name" json:"name"`
	Agent   string `yaml:"agent" json:"agent"`
	Task    string `yaml:"task" json:"task"`
	Model   string `yaml:"model" json:"model"` // optional: substituted as {model} in agent command
	Branch  string `yaml:"branch" json:"branch"`
	Timeout int    `yaml:"timeout" json:"timeout"` // minutes, 0 = use global
}

// agentsConfig is the YAML config file structure
type agentsConfig struct {
	Mode   string     `yaml:"mode"`   // "parallel" (default) or "sequential"
	Name   string     `yaml:"name"`   // overlay name for sequential mode
	Branch string     `yaml:"branch"` // branch for sequential mode
	Agents []agentDef `yaml:"agents"`
}

// agentResult holds the outcome of a single agent run
type agentResult struct {
	Name     string        `json:"name"`
	Agent    string        `json:"agent"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"-"`
	Error    string        `json:"error,omitempty"`
}

func doRunAll(ctx context.Context, cmd *cli.Command) error {
	baseDir := cmd.GetStringArg("base-dir")
	configPath := cmd.GetString("config")
	agentsInline := cmd.GetString("agents")
	timeoutMinutes := cmd.GetInt("timeout")
	concurrency := cmd.GetInt("concurrency")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	format := cmd.GetString("format")
	modelOverride := cmd.GetString("model")

	if configPath == "" && agentsInline == "" {
		return fmt.Errorf("either --config or --agents is required")
	}

	// Parse agent definitions
	var agents []agentDef
	var err error

	if configPath != "" {
		agents, err = loadAgentsConfig(configPath)
		if err != nil {
			return fmt.Errorf("failed to load agents config: %w", err)
		}
	} else {
		agents = parseInlineAgents(agentsInline)
	}

	if len(agents) == 0 {
		return fmt.Errorf("no agents defined")
	}

	onlyAgent := cmd.GetString("only")
	fromAgent := cmd.GetString("from")

	agents, err = filterAgents(agents, onlyAgent, fromAgent)
	if err != nil {
		return err
	}

	// Apply CLI model override to all agents
	if modelOverride != "" {
		for i := range agents {
			agents[i].Model = modelOverride
		}
	}

	// If config specifies sequential mode, delegate to chain execution
	if configPath != "" {
		mode, chainName, chainBranch := getConfigMode(configPath)
		if mode == "sequential" {
			steps := agentsToChainSteps(agents)
			return processRunChain(ctx, baseDir, chainName, chainBranch, steps, timeoutMinutes, doCleanup, doPush, false, format)
		}
	}

	return processRunAll(ctx, baseDir, agents, timeoutMinutes, concurrency, doCleanup, doPush, format)
}

func loadAgentsConfig(path string) ([]agentDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg agentsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Validate
	for i, a := range cfg.Agents {
		if a.Agent == "" {
			return nil, fmt.Errorf("agent[%d]: 'agent' command is required", i)
		}
		if a.Name == "" {
			cfg.Agents[i].Name = fmt.Sprintf("agent-%d", i+1)
		}
	}

	return cfg.Agents, nil
}

func parseInlineAgents(inline string) []agentDef {
	parts := strings.Split(inline, ",")
	var agents []agentDef
	for i, part := range parts {
		cmd := strings.TrimSpace(part)
		if cmd == "" {
			continue
		}
		agents = append(agents, agentDef{
			Name:  fmt.Sprintf("agent-%d", i+1),
			Agent: cmd,
		})
	}
	return agents
}

// filterAgents filters agents based on --only and --from flags
func filterAgents(agents []agentDef, onlyAgent, fromAgent string) ([]agentDef, error) {
	if onlyAgent != "" && fromAgent != "" {
		return nil, fmt.Errorf("cannot use both --only and --from")
	}

	if onlyAgent != "" {
		var filtered []agentDef
		for _, a := range agents {
			if a.Name == onlyAgent {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("agent %q not found in config", onlyAgent)
		}
		return filtered, nil
	} else if fromAgent != "" {
		found := false
		for i, a := range agents {
			if a.Name == fromAgent {
				agents = agents[i:]
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("agent %q not found in config", fromAgent)
		}
		return agents, nil
	}

	return agents, nil
}

func processRunAll(ctx context.Context, baseDir string, agents []agentDef, globalTimeout, concurrency int, doCleanup, doPush bool, format string) error {
	var results []agentResult

	logic := func(childCtx context.Context, t *tui.TUI) error {
		absBaseDir, err := filepath.Abs(baseDir)
		if err != nil {
			return fmt.Errorf("failed to resolve base directory: %w", err)
		}

		// Run auto-cleanup
		runAutoCleanup()

		log.Info("Starting %d agent(s) in parallel on %s", len(agents), absBaseDir)

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

		// Phase 1: Create all overlays sequentially (mount operations shouldn't race)
		type agentContext struct {
			def agentDef
			ovl *api.Overlay
		}
		var agentContexts []agentContext

		for _, def := range agents {
			// Check for name collision
			if store.Exists(def.Name) {
				log.Warn("Overlay %q already exists, skipping", def.Name)
				continue
			}

			branch := def.Branch
			if branch == "" && isGit && cfg.Git.AutoBranch {
				branch = cfg.Git.BranchPrefix + def.Name
			}

			opts := &api.CreateOptions{
				Name:    def.Name,
				BaseDir: absBaseDir,
				Branch:  branch,
			}

			ovl, err := mgr.Create(opts)
			if err != nil {
				log.Error("Failed to create overlay %q: %v", def.Name, err)
				continue
			}

			// Handle git branch
			if isGit && branch != "" {
				branchExists, _ := gitOps.BranchExists(childCtx, absBaseDir, branch)
				if !branchExists {
					log.Info("[%s] Creating branch %q", def.Name, branch)
					if err := gitOps.CreateBranch(childCtx, ovl.MountPoint, branch, ""); err != nil {
						log.Warn("[%s] Failed to create branch %q: %v", def.Name, branch, err)
					}
				} else {
					log.Info("[%s] Switching to branch %q", def.Name, branch)
					if err := gitOps.SwitchBranch(childCtx, ovl.MountPoint, branch); err != nil {
						log.Warn("[%s] Failed to switch to branch %q: %v", def.Name, branch, err)
					}
				}
			}

			if err := store.Save(ovl); err != nil {
				mgr.Cleanup(ovl)
				log.Error("Failed to save state for %q: %v", def.Name, err)
				continue
			}

			log.Debug("[%s] Overlay created at %s", def.Name, ovl.MountPoint)
			agentContexts = append(agentContexts, agentContext{def: def, ovl: ovl})
		}

		if len(agentContexts) == 0 {
			return fmt.Errorf("no overlays were created successfully")
		}

		// Phase 2: Run all agents in parallel
		var wg sync.WaitGroup
		results = make([]agentResult, len(agentContexts))

		limiter := NewAgentLimiter(concurrency)

		for i, ac := range agentContexts {
			wg.Add(1)
			go func(idx int, ac agentContext) {
				defer wg.Done()
				limiter.Acquire(ac.def.Agent)
				progressStr := fmt.Sprintf("[%d/%d]", idx+1, len(agentContexts))
				results[idx] = runSingleAgent(childCtx, ac.def, ac.ovl, absBaseDir, globalTimeout, doPush, t, progressStr)
				limiter.Release(ac.def.Agent)
			}(i, ac)
		}

		wg.Wait()

		// Phase 3: Cleanup if requested, otherwise clear stale FUSE PIDs so that
		// `phantom health` does not raise false dead_pid alarms after runs complete.
		if doCleanup {
			for _, ac := range agentContexts {
				log.Debug("Cleaning up overlay %q", ac.def.Name)
				if err := mgr.Cleanup(ac.ovl); err != nil {
					log.Warn("Failed to cleanup %q: %v", ac.def.Name, err)
				}
				store.Delete(ac.def.Name)
			}
		} else {
			for _, ac := range agentContexts {
				if ac.ovl.PID > 0 {
					ac.ovl.PID = 0
					if err := store.Save(ac.ovl); err != nil {
						log.Warn("Failed to update state for %q: %v", ac.def.Name, err)
					}
				}
			}
		}

		return nil
	}

	if err := logic(ctx, nil); err != nil {
		return err
	}

	// Phase 4: Print summary
	return printRunAllSummary(results, format)
}

func runSingleAgent(ctx context.Context, def agentDef, ovl *api.Overlay, absBaseDir string, globalTimeout int, doPush bool, t *tui.TUI, progress string) agentResult {
	result := agentResult{
		Name:  def.Name,
		Agent: def.Agent,
	}

	// Determine timeout
	timeoutMinutes := def.Timeout
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
		Agent:     def.Agent,
		Task:      def.Task,
		Model:     def.Model,
		BaseDir:   absBaseDir,
		Name:      def.Name,
		Timeout:   timeout,
		PushOnEnd: doPush,
		Headless:  true,
		Progress:  progress,
	}

	if t != nil {
		w := NewTUIWriter(t, def.Name, false)
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
	ExecuteHooks(def.Name, ovl.MountPoint, absBaseDir, ovl.Branch, def.Agent, def.Task, exitCode)

	return result
}

func printRunAllSummary(results []agentResult, format string) error {
	if format == "json" {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Println()
	log.Info("=== Run Summary ===")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tCOMMAND\tEXIT\tDURATION\tSTATUS")

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
	if failed > 0 {
		log.Info("%d/%d agent(s) failed", failed, len(results))
		os.Exit(1)
	} else {
		log.Info("All %d agent(s) completed successfully", len(results))
	}

	return nil
}

// getConfigMode reads the mode, name, and branch from a YAML config file
func getConfigMode(path string) (string, string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "parallel", "", ""
	}
	var cfg agentsConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "parallel", "", ""
	}
	mode := cfg.Mode
	if mode == "" {
		mode = "parallel"
	}
	return mode, cfg.Name, cfg.Branch
}

// agentsToChainSteps converts agentDef slice to chainStep slice for sequential mode
func agentsToChainSteps(agents []agentDef) []chainStep {
	steps := make([]chainStep, len(agents))
	for i, a := range agents {
		steps[i] = chainStep{
			Name:    a.Name,
			Agent:   a.Agent,
			Task:    a.Task,
			Timeout: a.Timeout,
		}
	}
	return steps
}
