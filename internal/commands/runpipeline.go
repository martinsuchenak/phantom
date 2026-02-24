package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
				Usage:   "Model to use (substituted as {model} in agent commands; overrides per-agent model)",
				EnvVars: []string{"OVERLAY_MODEL"},
			},
			&cli.StringFlag{
				Name:  "only",
				Usage: "Run only the specified agent",
			},
			&cli.StringFlag{
				Name:  "from",
				Usage: "Resume from the specified agent",
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
	baseDir := cmd.GetStringArg("base-dir")
	configPath := cmd.GetString("config")
	pipelineName := cmd.GetString("name")
	timeoutMinutes := cmd.GetInt("timeout")
	concurrency := cmd.GetInt("concurrency")
	doCleanup := cmd.GetBool("cleanup")
	doPush := cmd.GetBool("push")
	format := cmd.GetString("format")
	modelOverride := cmd.GetString("model")
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

	// Apply CLI model override
	if modelOverride != "" {
		for i := range pc.Agents {
			pc.Agents[i].Model = modelOverride
		}
	}

	return processRunPipeline(ctx, baseDir, pc, timeoutMinutes, concurrency, doCleanup, doPush, format)
}

// filterPipelineAgents filters agents based on --only and --from flags, and prunes missing dependencies
func filterPipelineAgents(agents []pipelineAgent, onlyAgent, fromAgent string) ([]pipelineAgent, error) {
	if onlyAgent != "" && fromAgent != "" {
		return nil, fmt.Errorf("cannot use both --only and --from")
	}

	if onlyAgent != "" {
		found := false
		for i, a := range agents {
			if a.Name == onlyAgent {
				found = true
			} else {
				agents[i].skip = true
			}
		}
		if !found {
			return nil, fmt.Errorf("agent %q not found in pipeline config", onlyAgent)
		}
	} else if fromAgent != "" {
		found := false
		for i, a := range agents {
			if a.Name == fromAgent {
				found = true
			}
			if !found {
				agents[i].skip = true
			}
		}
		if !found {
			return nil, fmt.Errorf("agent %q not found in pipeline config", fromAgent)
		}
	}

	return agents, nil
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

func processRunPipeline(ctx context.Context, baseDir string, pc *pipelineConfig, globalTimeout, concurrency int, doCleanup, doPush bool, format string) error {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to resolve base directory: %w", err)
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
		return fmt.Errorf("run-pipeline requires a clean working tree. Please commit or stash your changes before running.")
	}

	baseBranch := pc.Branch
	if baseBranch == "" && cfg.Git.AutoBranch {
		baseBranch = cfg.Git.BranchPrefix + pc.Name
	}
	if baseBranch == "" {
		currentBranch, err := gitOps.GetCurrentBranch(ctx, absBaseDir)
		if err == nil {
			baseBranch = currentBranch
		} else {
			baseBranch = "main" // fallback
		}
	}

	if baseBranch != "" {
		exists, _ := gitOps.BranchExists(ctx, absBaseDir, baseBranch)
		if !exists {
			if err := gitOps.CreateBranch(ctx, absBaseDir, baseBranch, ""); err != nil {
				return fmt.Errorf("failed to create base branch %q: %w", baseBranch, err)
			}
		}
	}

	agentMap := make(map[string]pipelineAgent)
	doneChans := make(map[string]chan struct{})
	var results sync.Map // string -> pipelineDepResult

	for _, a := range pc.Agents {
		agentMap[a.Name] = a
		doneChans[a.Name] = make(chan struct{})
	}

	// Ensure no unknown dependencies
	for _, a := range pc.Agents {
		for _, depName := range a.DependsOn {
			if _, exists := agentMap[depName]; !exists {
				return fmt.Errorf("agent %q depends on unknown agent %q", a.Name, depName)
			}
		}
	}

	var wg sync.WaitGroup
	var startedAgents atomic.Int32
	agentResults := make([]agentResult, len(pc.Agents))
	var resultsMu sync.Mutex

	limiter := NewAgentLimiter(concurrency)

	for idx, a := range pc.Agents {
		wg.Add(1)
		go func(idx int, ag pipelineAgent) {
			defer wg.Done()
			defer close(doneChans[ag.Name])

			// 1. Wait for all dependencies
			for _, depName := range ag.DependsOn {
				select {
				case <-ctx.Done():
					results.Store(ag.Name, pipelineDepResult{Err: ctx.Err()})
					return
				case <-doneChans[depName]:
					val, ok := results.Load(depName)
					if ok {
						res := val.(pipelineDepResult)
						if res.Err != nil {
							// Dependency failed
							err := fmt.Errorf("dependency %q failed: %w", depName, res.Err)
							results.Store(ag.Name, pipelineDepResult{Err: err})

							resultsMu.Lock()
							agentResults[idx] = agentResult{
								Name:     ag.Name,
								Agent:    ag.Agent,
								ExitCode: 1,
								Error:    err.Error(),
							}
							resultsMu.Unlock()
							return
						}
					}
				}
			}

			// 2. Setup Overlay
			ovlName := fmt.Sprintf("%s-%s", pc.Name, ag.Name)
			branchName := ag.Branch
			if branchName == "" {
				branchName = cfg.Git.BranchPrefix + ovlName
			}

			if ag.skip {
				log.Info("[%s] \033[33mSKIPPED\033[0m: Recovering state...", ag.Name)
				if store.Exists(ovlName) {
					if ovl, err := store.Load(ovlName); err == nil {
						results.Store(ag.Name, pipelineDepResult{
							Branch:     branchName,
							MountPoint: ovl.MountPoint,
						})
					} else {
						err = fmt.Errorf("failed to load existing overlay %q: %v", ovlName, err)
						results.Store(ag.Name, pipelineDepResult{Err: err})
					}
				} else {
					err := fmt.Errorf("state not found for skipped agent %q (dependency branch is missing)", ag.Name)
					results.Store(ag.Name, pipelineDepResult{Err: err})
				}
				resultsMu.Lock()
				agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 0, Error: ""}
				resultsMu.Unlock()
				return
			}

			opts := &api.CreateOptions{
				Name:    ovlName,
				BaseDir: absBaseDir,
				Branch:  branchName,
			}

			// If we have dependencies, create the new branch from baseBranch, then merge dependency branches into it
			var ovl *api.Overlay
			if store.Exists(opts.Name) {
				log.Info("[%s] Reusing existing overlay %q", ag.Name, opts.Name)
				var loadErr error
				ovl, loadErr = store.Load(opts.Name)
				if loadErr != nil {
					err = fmt.Errorf("failed to load existing overlay %q: %w", opts.Name, loadErr)
					results.Store(ag.Name, pipelineDepResult{Err: err})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
					resultsMu.Unlock()
					return
				}
				mounted, _ := mgr.IsMounted(ovl)
				if !mounted {
					if mountErr := mgr.Mount(ovl); mountErr != nil {
						err = fmt.Errorf("failed to mount existing overlay %q: %w", opts.Name, mountErr)
						results.Store(ag.Name, pipelineDepResult{Err: err})
						resultsMu.Lock()
						agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
						resultsMu.Unlock()
						return
					}
				}
			} else {
				var createErr error
				ovl, createErr = mgr.Create(opts)
				if createErr != nil {
					err = fmt.Errorf("failed to create overlay: %w", createErr)
					results.Store(ag.Name, pipelineDepResult{Err: err})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
					resultsMu.Unlock()
					return
				}

				if saveErr := store.Save(ovl); saveErr != nil {
					mgr.Cleanup(ovl)
					err = fmt.Errorf("failed to save overlay: %w", saveErr)
					results.Store(ag.Name, pipelineDepResult{Err: err})
					resultsMu.Lock()
					agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
					resultsMu.Unlock()
					return
				}
			}

			defer func() {
				if doCleanup {
					mgr.Cleanup(ovl)
					store.Delete(ovlName)
				} else {
					if ovl.PID > 0 {
						ovl.PID = 0
						store.Save(ovl)
					}
				}
			}()

			// Wait for FUSE to fully mount and expose the base directory (esp. .git)
			for i := 0; i < 50; i++ {
				if _, err := os.Stat(filepath.Join(ovl.MountPoint, ".git")); err == nil {
					break // .git is visible!
				}
				time.Sleep(100 * time.Millisecond)
			}

			// Git checkout to new branch from base branch
			branchExists, _ := gitOps.BranchExists(ctx, ovl.MountPoint, branchName)
			if !branchExists {
				log.Info("[%s] Creating branch %q from %q", ag.Name, branchName, baseBranch)
				if err := gitOps.CreateBranch(ctx, ovl.MountPoint, branchName, baseBranch); err != nil {
					log.Error("[%s] Failed to create branch %q: %v", ag.Name, branchName, err)
				}
			} else {
				log.Info("[%s] Switching to branch %q", ag.Name, branchName)
				if err := gitOps.SwitchBranch(ctx, ovl.MountPoint, branchName); err != nil {
					log.Error("[%s] Failed to switch to branch %q: %v", ag.Name, branchName, err)
				}
			}

			// 3. Merge dependency branches
			var mergeConflictFiles []string
			if len(ag.DependsOn) > 0 {
				for _, depName := range ag.DependsOn {
					val, _ := results.Load(depName)
					depRes := val.(pipelineDepResult)
					if depRes.Branch != "" && depRes.MountPoint != "" {
						log.Info("[%s] Fetching dependency %q", ag.Name, depName)
						fetchErr := gitOps.FetchFrom(ctx, ovl.MountPoint, depRes.MountPoint, depRes.Branch)
						if fetchErr != nil {
							err := fmt.Errorf("failed to fetch from dependency %s: %w", depName, fetchErr)
							log.Error("[%s] git fetch error: %v", ag.Name, err)
							results.Store(ag.Name, pipelineDepResult{Err: err})
							resultsMu.Lock()
							agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
							resultsMu.Unlock()
							return
						}

						log.Info("[%s] Merging dependency %q", ag.Name, depName)
						err := gitOps.MergeBranchNoEdit(ctx, ovl.MountPoint, "FETCH_HEAD")
						if err != nil {
							// If there's a conflict, err might be non-nil. Check unmerged files.
							unmerged, unmergedErr := gitOps.GetUnmergedFiles(ctx, ovl.MountPoint)
							if unmergedErr != nil {
								// Unexpected git error while checking status
								results.Store(ag.Name, pipelineDepResult{Err: err})
								resultsMu.Lock()
								agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
								resultsMu.Unlock()
								return
							} else if len(unmerged) > 0 {
								// Conflict recorded
								mergeConflictFiles = append(mergeConflictFiles, unmerged...)
							} else {
								// Real error merging, abort.
								results.Store(ag.Name, pipelineDepResult{Err: err})
								resultsMu.Lock()
								agentResults[idx] = agentResult{Name: ag.Name, Agent: ag.Agent, ExitCode: 1, Error: err.Error()}
								resultsMu.Unlock()
								return
							}
						}
					}
				}
			}

			// In case of duplicate conflicts
			mergeConflictFiles = uniqueStrings(mergeConflictFiles)

			// 4. Run Agent
			var finalTask string = ag.Task

			if len(mergeConflictFiles) > 0 {
				// Inject the specialized prompt
				injection := `
[SYSTEM INJECTION]
You have been assigned to complete the following task: "{original_task}"
HOWEVER, the previous tasks resulted in git merge conflicts in the following files:
{conflicted_files}

Your IMMEDIATE PRIORITY is to resolve these merge conflicts.
1. Open the conflicted files. You will see standard ` + "`<<<<<<< HEAD`" + ` git conflict markers.
2. Edit the files to correctly synthesize the changes.
3. Remove all conflict markers.
4. Run ` + "`git add`" + ` and ` + "`git commit`" + ` to finalize the merge.

If you are unable to safely resolve the conflict, or you do not understand the intention of the conflicting code, you MUST return a non-zero exit code or use a designated phantom command to abort the pipeline. Do NOT commit broken code.

Once the conflict is resolved, proceed to your actual task.
[END INJECTION]
`
				finalTask = strings.Replace(injection, "{original_task}", ag.Task, 1)
				conflictList := ""
				for _, f := range mergeConflictFiles {
					conflictList += "- " + f + "\n"
				}
				finalTask = strings.Replace(finalTask, "{conflicted_files}", conflictList, 1)

				log.Warn("[%s] Found merge conflicts! Injecting resolution prompt.", ag.Name)
			}

			agToRun := agentDef{
				Name:    ag.Name,
				Agent:   ag.Agent,
				Task:    finalTask,
				Model:   ag.Model,
				Branch:  branchName,
				Timeout: ag.Timeout,
			}

			// Wait for concurrency token
			limiter.Acquire(agToRun.Agent)

			started := startedAgents.Add(1)
			progressStr := fmt.Sprintf("[%d/%d]", started, len(pc.Agents))

			result := runSingleAgent(ctx, agToRun, ovl, absBaseDir, globalTimeout, doPush, nil, progressStr)

			limiter.Release(agToRun.Agent)

			// Auto-commit so that dependent agents can fetch these changes
			if result.ExitCode == 0 {
				hasChanges, err := gitOps.HasUncommittedChanges(ctx, ovl.MountPoint)
				if err == nil && hasChanges {
					gitOps.CommitAll(ctx, ovl.MountPoint, fmt.Sprintf("phantom pipeline: %s completed", ag.Name))
				}
			}

			// Failsafe: check unresolved conflicts after completion
			if result.ExitCode == 0 {
				unmerged, _ := gitOps.GetUnmergedFiles(ctx, ovl.MountPoint)
				if len(unmerged) > 0 {
					result.ExitCode = 1
					result.Error = fmt.Sprintf("failed failsafe: %d files still contain conflict markers (e.g., %s)", len(unmerged), unmerged[0])
					log.Error("[%s] Pipeline failsafe triggered: unresolved conflicts remained.", ag.Name)
				}
			}

			resultsMu.Lock()
			agentResults[idx] = result
			resultsMu.Unlock()

			if result.ExitCode == 0 && result.Error == "" {
				results.Store(ag.Name, pipelineDepResult{Branch: branchName, MountPoint: ovl.MountPoint})
			} else {
				err := fmt.Errorf("agent %q failed with exit code %d: %s", ag.Name, result.ExitCode, result.Error)
				results.Store(ag.Name, pipelineDepResult{Err: err})
			}
		}(idx, a)
	}

	// Wait for all agents to complete
	wg.Wait()

	return printRunAllSummary(agentResults, format)
}

func uniqueStrings(s []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, v := range s {
		v = strings.TrimSpace(v)
		if _, ok := seen[v]; !ok && v != "" {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
