package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/martinsuchenak/phantom/internal/git"
	"github.com/martinsuchenak/phantom/internal/state"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
	"gopkg.in/yaml.v3"
)

// pipelineConfig is the YAML config for DAG agent execution
type pipelineConfig struct {
	Name   string          `yaml:"name"`
	Branch string          `yaml:"branch"`
	Agents []pipelineAgent `yaml:"agents"`
}

type pipelineAgent struct {
	Name      string   `yaml:"name" json:"name"`
	Agent     string   `yaml:"agent" json:"agent"`
	Task      string   `yaml:"task" json:"task"`
	Model     string   `yaml:"model" json:"model"`
	Branch    string   `yaml:"branch" json:"branch"`
	Timeout   int      `yaml:"timeout" json:"timeout"`
	DependsOn []string `yaml:"depends_on" json:"depends_on"`
	skip      bool
}

// NewRunPipelineCommand creates the run-pipeline command
func NewRunPipelineCommand() *cli.Command {
	return &cli.Command{
		Name:        "run-pipeline",
		Usage:       "Run agents in a DAG pipeline",
		Description: "Run agents in parallel or sequentially depending on 'depends_on' configuration. Automatically resolves or delegates merge conflicts.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to pipeline YAML config file",
			},
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Pipeline name (overrides config). Reuses existing overlays if found.",
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
				Usage:   "Model to use for agents with no model set",
				EnvVars: []string{"OVERLAY_MODEL"},
			},
			&cli.BoolFlag{
				Name:  "model-override",
				Usage: "Force --model onto all agents, replacing any per-agent model",
			},
			&cli.StringFlag{
				Name:  "only",
				Usage: "Run only the specified agent",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Resume from the specified agent (skipped agents must have existing state)",
			},
		},
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "base-dir",
				Usage:    "Base directory to overlay",
				Required: true,
			},
		},
		Run: doRunPipeline,
	}
}

func doRunPipeline(ctx context.Context, cmd *cli.Command) error {
	baseDir := resolveBaseDir(cmd.GetStringArg("base-dir"))
	configPath := cmd.GetString("config")
	pipelineName := cmd.GetString("name")
	timeoutMinutes := cmd.GetInt("timeout")
	concurrency := cmd.GetInt("concurrency")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	format := cmd.GetString("format")
	modelOverride := cmd.GetString("model")
	forceModelOverride := cmd.GetBool("model-override")
	onlyAgent := cmd.GetString("only")
	fromAgent := cmd.GetString("from")

	if configPath == "" {
		return fmt.Errorf("--config is required for run-pipeline")
	}

	pc, err := loadPipelineConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load pipeline config: %w", err)
	}

	if len(pc.Agents) == 0 {
		return fmt.Errorf("no agents defined in pipeline config")
	}

	pc.Agents, err = filterPipelineAgents(pc.Agents, onlyAgent, fromAgent)
	if err != nil {
		return err
	}

	if pipelineName != "" {
		pc.Name = pipelineName
	}
	if pc.Name == "" {
		pc.Name = fmt.Sprintf("pipeline-%d", time.Now().Unix())
	}

	if modelOverride != "" {
		for i := range pc.Agents {
			if forceModelOverride || pc.Agents[i].Model == "" {
				pc.Agents[i].Model = modelOverride
			}
		}
	}

	notifier := pipelineNotifier(NewProgressTree(pc.Agents))
	return processRunPipeline(ctx, baseDir, pc, timeoutMinutes, concurrency, doCleanup, doPush, format, notifier)
}

// filterPipelineAgents filters agents based on --only and --from flags.
func filterPipelineAgents(agents []pipelineAgent, onlyAgent, fromAgent string) ([]pipelineAgent, error) {
	if onlyAgent != "" && fromAgent != "" {
		return nil, fmt.Errorf("cannot use both --only and --from")
	}

	if onlyAgent != "" {
		found := false
		for i, a := range agents {
			if a.Name == onlyAgent {
				found = true
				agents[i].DependsOn = nil // run in isolation
			} else {
				agents[i].skip = true
			}
		}
		if !found {
			return nil, fmt.Errorf("agent %q not found in pipeline config", onlyAgent)
		}
		return agents, nil
	}

	if fromAgent != "" {
		found := false
		skipSet := make(map[string]bool)
		for i, a := range agents {
			if a.Name == fromAgent {
				found = true
			}
			if !found {
				agents[i].skip = true
				skipSet[a.Name] = true
			}
		}
		if !found {
			return nil, fmt.Errorf("agent %q not found in pipeline config", fromAgent)
		}
		// Prune live agents' DependsOn of skipped names so the DAG wait loop
		// doesn't block on channels for agents recovered from state.
		for i := range agents {
			if agents[i].skip {
				continue
			}
			var pruned []string
			for _, dep := range agents[i].DependsOn {
				if !skipSet[dep] {
					pruned = append(pruned, dep)
				}
			}
			agents[i].DependsOn = pruned
		}
		return agents, nil
	}

	return agents, nil
}

// detectCycle returns an error if the dependency graph contains a cycle.
func detectCycle(agents []pipelineAgent) error {
	deps := make(map[string][]string, len(agents))
	for _, a := range agents {
		deps[a.Name] = a.DependsOn
	}
	const (
		unvisited = 0
		visiting  = 1
		visited   = 2
	)
	state := make(map[string]int, len(agents))
	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case visited:
			return nil
		case visiting:
			return fmt.Errorf("circular dependency detected: %s", strings.Join(append(path, name), " → "))
		}
		state[name] = visiting
		for _, dep := range deps[name] {
			if err := visit(dep, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = visited
		return nil
	}
	for _, a := range agents {
		if err := visit(a.Name, nil); err != nil {
			return err
		}
	}
	return nil
}

func loadPipelineConfig(path string) (*pipelineConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pc pipelineConfig
	if err := yaml.Unmarshal(data, &pc); err != nil {
		return nil, err
	}
	for i, a := range pc.Agents {
		if a.Agent == "" {
			return nil, fmt.Errorf("agent[%d]: 'agent' command is required", i)
		}
		if a.Name == "" {
			pc.Agents[i].Name = fmt.Sprintf("agent-%d", i+1)
		}
	}
	return &pc, nil
}

type pipelineDepResult struct {
	Err        error
	Branch     string
	MountPoint string
}

// conflictInjectionTmpl builds the conflict-resolution task prefix.
// Using text/template prevents the original task text from corrupting the output
// if it happens to contain the placeholder strings.
var conflictInjectionTmpl = template.Must(template.New("conflict").Parse(`[SYSTEM INJECTION]
You have been assigned to complete the following task: "{{.OriginalTask}}"
HOWEVER, the previous tasks resulted in git merge conflicts in the following files:
{{.ConflictList}}
Your IMMEDIATE PRIORITY is to resolve these merge conflicts.
1. Open the conflicted files. You will see standard <<<<<<< HEAD git conflict markers.
2. Edit the files to correctly synthesize the changes.
3. Remove all conflict markers.
4. Run ` + "`git add`" + ` and ` + "`git commit`" + ` to finalize the merge.

If you are unable to safely resolve the conflict, or you do not understand the intention of the conflicting code, you MUST return a non-zero exit code or use a designated phantom command to abort the pipeline. Do NOT commit broken code.

Once the conflict is resolved, proceed to your actual task.
[END INJECTION]
`))

func buildConflictTask(originalTask string, conflictFiles []string) (string, error) {
	list := ""
	for _, f := range conflictFiles {
		list += "- " + f + "\n"
	}
	var buf strings.Builder
	err := conflictInjectionTmpl.Execute(&buf, struct {
		OriginalTask string
		ConflictList string
	}{originalTask, list})
	if err != nil {
		return "", fmt.Errorf("failed to render conflict injection template: %w", err)
	}
	return buf.String(), nil
}

func processRunPipeline(ctx context.Context, baseDir string, pc *pipelineConfig, globalTimeout, concurrency int, doCleanup, doPush bool, format string, notifier pipelineNotifier) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
	}

	if err := detectCycle(pc.Agents); err != nil {
		return err
	}

	runAutoCleanup()
	log.Info("Starting pipeline %q with %d agent(s) on %s", pc.Name, len(pc.Agents), absBaseDir)

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
	if !isGit {
		return fmt.Errorf("run-pipeline requires a valid git repository at base-dir")
	}

	hasChanges, err := gitOps.HasUncommittedChanges(ctx, absBaseDir)
	if err != nil {
		return fmt.Errorf("failed to check repository status: %w", err)
	}
	if hasChanges {
		return fmt.Errorf("run-pipeline requires a clean working tree. Please commit or stash your changes before running")
	}

	baseBranch := pc.Branch
	if baseBranch == "" && cfg.Git.AutoBranch {
		baseBranch = cfg.Git.BranchPrefix + pc.Name
	}
	if baseBranch == "" {
		currentBranch, err := gitOps.GetCurrentBranch(ctx, absBaseDir)
		if err != nil {
			return fmt.Errorf("failed to determine current branch: %w", err)
		}
		baseBranch = currentBranch
	}

	exists, _ := gitOps.BranchExists(ctx, absBaseDir, baseBranch)
	if !exists {
		if err := gitOps.CreateBranch(ctx, absBaseDir, baseBranch, ""); err != nil {
			return fmt.Errorf("failed to create base branch %q: %w", baseBranch, err)
		}
	}

	agentMap := make(map[string]pipelineAgent)
	doneChans := make(map[string]chan struct{})
	var depResults sync.Map // string -> pipelineDepResult

	for _, a := range pc.Agents {
		agentMap[a.Name] = a
		doneChans[a.Name] = make(chan struct{})
	}

	for _, a := range pc.Agents {
		for _, depName := range a.DependsOn {
			if _, exists := agentMap[depName]; !exists {
				return fmt.Errorf("agent %q depends on unknown agent %q", a.Name, depName)
			}
		}
	}

	var wg sync.WaitGroup
	agentResults := make([]agentResult, len(pc.Agents))
	var resultsMu sync.Mutex

	var runningMu sync.Mutex
	runningCount := 0

	limiter := NewAgentLimiter(concurrency)

	// Trigger initial render for terminal notifier
	if pt, ok := notifier.(*ProgressTree); ok {
		pt.render()
	}

	for idx, a := range pc.Agents {
		wg.Add(1)
		go func(idx int, ag pipelineAgent) {
			defer wg.Done()
			defer close(doneChans[ag.Name])

			// 1. Wait for dependencies
			for _, depName := range ag.DependsOn {
				select {
				case <-ctx.Done():
					depResults.Store(ag.Name, pipelineDepResult{Err: ctx.Err()})
					return
				case <-doneChans[depName]:
					val, ok := depResults.Load(depName)
					if ok {
						res := val.(pipelineDepResult)
						if res.Err != nil {
							depErr := fmt.Errorf("dependency %q failed: %w", depName, res.Err)
							depResults.Store(ag.Name, pipelineDepResult{Err: depErr})
							resultsMu.Lock()
							agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: depErr.Error()}
							resultsMu.Unlock()
							return
						}
					}
				}
			}

			// 2. Setup overlay
			ovlName := fmt.Sprintf("%s-%s", pc.Name, ag.Name)
			branchName := ag.Branch
			if branchName == "" {
				branchName = cfg.Git.BranchPrefix + ovlName
			}

			if ag.skip {
				notifier.Logf("[%s] ⏭️  SKIPPED: Recovering state...", ag.Name)
				if store.Exists(ovlName) {
					ovl, loadErr := store.Load(ovlName)
					if loadErr == nil {
						depResults.Store(ag.Name, pipelineDepResult{Branch: branchName, MountPoint: ovl.MountPoint})
					} else {
						skipErr := fmt.Errorf("failed to load existing overlay %q: %w", ovlName, loadErr)
						depResults.Store(ag.Name, pipelineDepResult{Err: skipErr})
						notifier.Errorf("[%s] %v", ag.Name, skipErr)
					}
				} else {
					skipErr := fmt.Errorf("no saved state for skipped agent %q — run without --from to execute it first", ag.Name)
					depResults.Store(ag.Name, pipelineDepResult{Err: skipErr})
					notifier.Errorf("[%s] %v", ag.Name, skipErr)
				}
				resultsMu.Lock()
				agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: -1}
				resultsMu.Unlock()
				return
			}

			opts := &api.CreateOptions{Name: ovlName, BaseDir: absBaseDir, Branch: branchName}

			var ovl *api.Overlay
			notifier.Update(ag.Name, StateStarting)

			if store.Exists(opts.Name) {
				notifier.Logf("[%s] Reusing existing overlay %q", ag.Name, opts.Name)
				ovl, err = store.Load(opts.Name)
				if err != nil {
					agErr := fmt.Errorf("failed to load existing overlay %q: %w", opts.Name, err)
					depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
					resultsMu.Unlock()
					notifier.Update(ag.Name, StateFailed)
					return
				}
				mounted, _ := mgr.IsMounted(ovl)
				if !mounted {
					if mountErr := mgr.Mount(ovl); mountErr != nil {
						agErr := fmt.Errorf("failed to mount existing overlay %q: %w", opts.Name, mountErr)
						depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
						resultsMu.Lock()
						agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
						resultsMu.Unlock()
						notifier.Update(ag.Name, StateFailed)
						return
					}
				}
			} else {
				ovl, err = mgr.Create(opts)
				if err != nil {
					agErr := fmt.Errorf("failed to create overlay: %w", err)
					depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
					resultsMu.Unlock()
					notifier.Update(ag.Name, StateFailed)
					return
				}
				if saveErr := store.Save(ovl); saveErr != nil {
					mgr.Cleanup(ovl)
					agErr := fmt.Errorf("failed to save overlay: %w", saveErr)
					depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
					resultsMu.Unlock()
					notifier.Update(ag.Name, StateFailed)
					return
				}
			}

			defer func() {
				if doCleanup {
					mgr.Cleanup(ovl)
					store.Delete(ovlName)
				} else if ovl.PID > 0 {
					ovl.PID = 0
					store.Save(ovl)
				}
			}()

			// Wait for FUSE mount to become accessible
			for i := 0; i < 50; i++ {
				if _, statErr := os.Stat(ovl.MountPoint); statErr == nil {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if _, statErr := os.Stat(ovl.MountPoint); statErr != nil {
				agErr := fmt.Errorf("overlay mount point %q did not become accessible: %w", ovl.MountPoint, statErr)
				depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
				resultsMu.Lock()
				agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
				resultsMu.Unlock()
				notifier.Update(ag.Name, StateFailed)
				return
			}

			// Git branch setup
			branchExists, _ := gitOps.BranchExists(ctx, ovl.MountPoint, branchName)
			if !branchExists {
				notifier.Logf("[%s] Creating branch %q from %q", ag.Name, branchName, baseBranch)
				if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branchName, baseBranch); err != nil {
					notifier.Errorf("[%s] Failed to create branch %q: %v", ag.Name, branchName, err)
				}
			} else {
				notifier.Logf("[%s] Switching to branch %q", ag.Name, branchName)
				if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branchName); err != nil {
					notifier.Errorf("[%s] Failed to switch to branch %q: %v", ag.Name, branchName, err)
				}
			}

			// 3. Merge dependency branches
			var mergeConflictFiles []string
			for _, depName := range ag.DependsOn {
				val, _ := depResults.Load(depName)
				depRes := val.(pipelineDepResult)
				if depRes.Branch == "" || depRes.MountPoint == "" {
					continue
				}

				notifier.Update(ag.Name, StateFetching)
				notifier.Logf("[%s] Fetching dependency %q", ag.Name, depName)
				if fetchErr := gitOps.FetchFrom(ctx, ovl.MountPoint, depRes.MountPoint, depRes.Branch); fetchErr != nil {
					agErr := fmt.Errorf("failed to fetch from dependency %s: %w", depName, fetchErr)
					notifier.Errorf("[%s] git fetch error: %v", ag.Name, agErr)
					depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: agErr.Error()}
					resultsMu.Unlock()
					notifier.Update(ag.Name, StateFailed)
					return
				}

				notifier.Update(ag.Name, StateMerging)
				notifier.Logf("[%s] Merging dependency %q", ag.Name, depName)
				mergeErr := gitOps.MergeBranchNoEdit(ctx, ovl.MountPoint, "FETCH_HEAD")
				if mergeErr != nil {
					unmerged, unmergedErr := gitOps.GetUnmergedFiles(ctx, ovl.MountPoint)
					if unmergedErr != nil {
						depResults.Store(ag.Name, pipelineDepResult{Err: mergeErr})
						resultsMu.Lock()
						agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: mergeErr.Error()}
						resultsMu.Unlock()
						notifier.Update(ag.Name, StateFailed)
						return
					} else if len(unmerged) > 0 {
						mergeConflictFiles = append(mergeConflictFiles, unmerged...)
					} else {
						depResults.Store(ag.Name, pipelineDepResult{Err: mergeErr})
						resultsMu.Lock()
						agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: mergeErr.Error()}
						resultsMu.Unlock()
						notifier.Update(ag.Name, StateFailed)
						return
					}
				}
			}

			mergeConflictFiles = uniqueStrings(mergeConflictFiles)

			// 4. Build task, injecting conflict resolution instructions if needed
			finalTask := ag.Task
			if len(mergeConflictFiles) > 0 {
				notifier.Logf("[%s] ⚠️  Found merge conflicts! Injecting resolution prompt.", ag.Name)
				injected, tmplErr := buildConflictTask(ag.Task, mergeConflictFiles)
				if tmplErr != nil {
					notifier.Errorf("[%s] %v", ag.Name, tmplErr)
				} else {
					finalTask = injected
				}
			}

			agToRun := agentDef{
				Name:    ag.Name,
				Agent:   ag.Agent,
				Task:    finalTask,
				Model:   ag.Model,
				Branch:  branchName,
				Timeout: ag.Timeout,
			}

			// Acquire concurrency token before incrementing the counter so the
			// progress string reflects agents that are actually executing.
			notifier.Update(ag.Name, StateQueued)
			limiter.Acquire(agToRun.Agent)

			runningMu.Lock()
			runningCount++
			progressStr := fmt.Sprintf("[%d/%d]", runningCount, len(pc.Agents))
			runningMu.Unlock()

			notifier.Update(ag.Name, StateRunning)
			notifier.Logf("[%s] Running: %s", ag.Name, ag.Agent)
			result := runSingleAgent(ctx, agToRun, ovl, absBaseDir, globalTimeout, doPush, nil, progressStr)

			limiter.Release(agToRun.Agent)

			// Auto-commit so dependent agents can fetch these changes
			if result.ExitCode == 0 {
				uncommitted, checkErr := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
				if checkErr != nil {
					notifier.Errorf("[%s] Failed to check for uncommitted changes: %v", ag.Name, checkErr)
				} else if uncommitted {
					commitMsg := fmt.Sprintf("phantom pipeline: %s completed", ag.Name)
					if commitErr := gitOps.CommitAll(ctx, ovl.MountPoint, commitMsg); commitErr != nil {
						notifier.Errorf("[%s] Auto-commit failed; dependent agents may fetch stale state: %v", ag.Name, commitErr)
					}
				}
			}

			// Failsafe: reject success if conflict markers remain
			if result.ExitCode == 0 {
				unmerged, _ := gitOps.GetUnmergedFiles(ctx, ovl.MountPoint)
				if len(unmerged) > 0 {
					result.ExitCode = 1
					result.Error = fmt.Sprintf("failsafe: %d file(s) still contain conflict markers (e.g., %s)", len(unmerged), unmerged[0])
					notifier.Errorf("[%s] Pipeline failsafe triggered: unresolved conflicts remained.", ag.Name)
				}
			}

			resultsMu.Lock()
			agentResults[idx] = result
			resultsMu.Unlock()

			if result.ExitCode == 0 && result.Error == "" {
				depResults.Store(ag.Name, pipelineDepResult{Branch: branchName, MountPoint: ovl.MountPoint})
				notifier.Update(ag.Name, StateDone)
			} else {
				agErr := fmt.Errorf("agent %q failed with exit code %d: %s", ag.Name, result.ExitCode, result.Error)
				depResults.Store(ag.Name, pipelineDepResult{Err: agErr})
				notifier.Update(ag.Name, StateFailed)
			}
		}(idx, a)
	}

	wg.Wait()
	notifier.Clear()
	notifier.Summary(agentResults)

	return printPipelineSummary(agentResults, format)
}

// printPipelineSummary prints terminal output and returns an error if any agent
// failed, allowing the CLI framework to exit with a non-zero code.
func printPipelineSummary(results []agentResult, format string) error {
	if err := printRunAllSummary(results, format); err != nil {
		return err
	}
	for _, r := range results {
		if r.ExitCode != 0 && r.ExitCode != -1 {
			return fmt.Errorf("one or more pipeline agents failed")
		}
	}
	return nil
}

// uniqueStrings deduplicates a string slice preserving order.
// Paths are not trimmed to avoid mangling whitespace in valid filenames.
func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range s {
		if _, ok := seen[v]; !ok && v != "" {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
