package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/paularlott/cli"
	"gopkg.in/yaml.v3"
)

type agentTemplate struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Agent       string `json:"agent" yaml:"agent"`
	TaskMode    string `json:"task_mode" yaml:"task_mode"`   // "arg", "stdin", "placeholder"
	ModelHint   string `json:"model_hint" yaml:"model_hint"` // example model name for docs, e.g. "claude-opus-4-5"
}

var builtinTemplates = []agentTemplate{
	{
		Name:        "claude",
		Description: "Claude Code (Anthropic) — headless, skip all permission prompts",
		Agent:       "claude --print --dangerously-skip-permissions --model {model}",
		TaskMode:    "stdin",
		ModelHint:   "claude-opus-4-5",
	},
	{
		Name:        "claude-interactive",
		Description: "Claude Code — interactive mode with task as argument",
		Agent:       `claude --model {model} "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "claude-opus-4-5",
	},
	{
		Name:        "gemini",
		Description: "Gemini CLI (Google) — headless, auto-approve all tools via --yolo",
		Agent:       "gemini --yolo --model {model}",
		TaskMode:    "stdin",
		ModelHint:   "gemini-2.0-flash",
	},
	{
		Name:        "gemini-arg",
		Description: "Gemini CLI — headless with task as argument, auto-approve all tools",
		Agent:       `gemini --yolo --model {model} "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "gemini-2.0-flash",
	},
	{
		Name:        "aider",
		Description: "Aider — headless, auto-approve all changes via --yes-always",
		Agent:       `aider --yes-always --model {model} --message "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "gpt-4o",
	},
	{
		Name:        "vibe",
		Description: "Mistral Vibe — task passed via --prompt flag",
		Agent:       `vibe --prompt "{task}"`,
		TaskMode:    "placeholder",
	},
	{
		Name:        "copilot",
		Description: "GitHub Copilot CLI — non-interactive, all tools allowed",
		Agent:       `copilot --prompt "{task}" --allow-all-tools --model {model}`,
		TaskMode:    "placeholder",
		ModelHint:   "claude-sonnet-4",
	},
	{
		Name:        "gh-copilot",
		Description: "GitHub Copilot CLI extension (gh copilot suggest) — task as argument",
		Agent:       `gh copilot suggest "{task}"`,
		TaskMode:    "placeholder",
	},
	{
		Name:        "codex",
		Description: "OpenAI Codex CLI — headless, full autonomy via --full-auto",
		Agent:       `codex --full-auto --model {model} "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "o3",
	},
	{
		Name:        "opencode",
		Description: "OpenCode — non-interactive run mode, all tools auto-approved, quiet output",
		Agent:       `opencode run -q --model {model} "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "openai/gpt-4o",
	},
	{
		Name:        "opencode-stdin",
		Description: "OpenCode — non-interactive run mode, task piped via --prompt flag",
		Agent:       "opencode run -q --model {model} --prompt",
		TaskMode:    "stdin",
		ModelHint:   "openai/gpt-4o",
	},
	{
		Name:        "qwen-code",
		Description: "Qwen Code CLI (Alibaba) — headless, auto-approve all tools via --yolo",
		Agent:       `qwen --yolo --model {model} --prompt "{task}"`,
		TaskMode:    "placeholder",
		ModelHint:   "qwen2.5-coder-32b-instruct",
	},
	{
		Name:        "qwen-code-stdin",
		Description: "Qwen Code CLI — headless, all tools auto-approved, task piped to stdin",
		Agent:       "qwen --yolo --model {model}",
		TaskMode:    "stdin",
		ModelHint:   "qwen2.5-coder-32b-instruct",
	},
	{
		Name:        "kiro",
		Description: "Kiro CLI — non-interactive chat, all tools trusted, task as placeholder",
		Agent:       `kiro-cli chat --no-interactive --trust-all-tools "{task}"`,
		TaskMode:    "placeholder",
	},
}

// NewTemplateCommand creates the template command
func NewTemplateCommand() *cli.Command {
	return &cli.Command{
		Name:  "template",
		Usage: "Manage agent templates",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List available agent templates",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "format", Usage: "Output format (table, json)", DefaultValue: "table"},
				},
				Run: doTemplateList,
			},
			{
				Name:  "show",
				Usage: "Show details of a template",
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "name", Usage: "Template name", Required: true},
				},
				Run: doTemplateShow,
			},
			{
				Name:  "generate",
				Usage: "Generate agents.yaml (or chain.yaml with --chain) from templates",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: "Output file (default: stdout)"},
					&cli.StringFlag{Name: "agents", Usage: "Comma-separated template names (e.g. claude,aider,gemini)", Required: true},
					&cli.BoolFlag{Name: "chain", Usage: "Generate chain.yaml format (for run-chain) instead of agents.yaml"},
					&cli.StringFlag{Name: "name", Aliases: []string{"n"}, Usage: "Chain/pipeline name (chain mode only)"},
					&cli.StringFlag{Name: "branch", Aliases: []string{"b"}, Usage: "Git branch (chain mode only)"},
				},
				Run: doTemplateGenerate,
			},
		},
	}
}

func doTemplateList(ctx context.Context, cmd *cli.Command) error {
	format := cmd.GetString("format")
	if format == "json" {
		data, err := json.MarshalIndent(builtinTemplates, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tAGENT\tTASK MODE\tDESCRIPTION")
	for _, t := range builtinTemplates {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.Name, t.Agent, t.TaskMode, t.Description)
	}
	return w.Flush()
}

func doTemplateShow(ctx context.Context, cmd *cli.Command) error {
	name := cmd.GetStringArg("name")
	for _, t := range builtinTemplates {
		if t.Name == name {
			fmt.Printf("Name:        %s\n", t.Name)
			fmt.Printf("Description: %s\n", t.Description)
			fmt.Printf("Agent:       %s\n", t.Agent)
			fmt.Printf("Task Mode:   %s\n", t.TaskMode)
			if t.ModelHint != "" {
				fmt.Printf("Model hint:  %s  (pass --model or set 'model:' in agents.yaml)\n", t.ModelHint)
			}
			fmt.Println()
			fmt.Println("Example agents.yaml entry:")
			fmt.Println()
			example := agentDef{Name: t.Name + "-agent", Agent: t.Agent, Task: "your task here", Branch: "feature/" + t.Name}
			if t.ModelHint != "" {
				example.Model = t.ModelHint
			}
			data, _ := yaml.Marshal([]agentDef{example})
			fmt.Printf("agents:\n  %s", string(data))
			return nil
		}
	}
	return fmt.Errorf("template %q not found (use: phantom template list)", name)
}

func doTemplateGenerate(ctx context.Context, cmd *cli.Command) error {
	agentNames := cmd.GetString("agents")
	output := cmd.GetString("output")
	chain := cmd.GetBool("chain")
	chainName := cmd.GetString("name")
	chainBranch := cmd.GetString("branch")

	names := parseInlineAgentNames(agentNames)
	if len(names) == 0 {
		return fmt.Errorf("no agent names provided")
	}

	var content string
	var err error

	if chain {
		content, err = generateChainYAML(names, chainName, chainBranch)
	} else {
		content, err = generateAgentsYAML(names)
	}
	if err != nil {
		return err
	}

	if output != "" {
		if err := os.WriteFile(output, []byte(content), 0600); err != nil {
			return err
		}
		log.Info("Generated %s with %d agent(s)", output, len(names))
		return nil
	}

	fmt.Print(content)
	return nil
}

func generateAgentsYAML(names []string) (string, error) {
	var agents []agentDef
	for _, name := range names {
		tmpl := findTemplate(name)
		if tmpl == nil {
			return "", fmt.Errorf("unknown template %q (use: phantom template list)", name)
		}
		def := agentDef{
			Name:   tmpl.Name + "-agent",
			Agent:  tmpl.Agent,
			Task:   "TODO: describe your task",
			Branch: "feature/" + tmpl.Name,
		}
		if tmpl.ModelHint != "" {
			def.Model = tmpl.ModelHint
		}
		agents = append(agents, def)
	}

	cfg := agentsConfig{Agents: agents}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return "", err
	}

	header := "# Generated by: phantom template generate\n" +
		"# Edit the 'task' fields before running.\n" +
		"# The 'model' field is optional — remove it to use the agent's default model,\n" +
		"# or override all agents at once with: phantom run-all --model <name>\n\n"

	return header + string(data), nil
}

func generateChainYAML(names []string, name, branch string) (string, error) {
	if name == "" {
		name = "my-pipeline"
	}

	var steps []chainStep
	for _, n := range names {
		tmpl := findTemplate(n)
		if tmpl == nil {
			return "", fmt.Errorf("unknown template %q (use: phantom template list)", n)
		}
		step := chainStep{
			Name:  tmpl.Name + "-step",
			Agent: tmpl.Agent,
			Task:  "TODO: describe this step's task",
		}
		if tmpl.ModelHint != "" {
			step.Model = tmpl.ModelHint
		}
		steps = append(steps, step)
	}

	cc := chainConfig{
		Name:   name,
		Branch: branch,
		Steps:  steps,
	}
	data, err := yaml.Marshal(cc)
	if err != nil {
		return "", err
	}

	header := "# Generated by: phantom template generate --chain\n" +
		"# Edit the 'task' fields before running.\n" +
		"# The 'model' field is optional — remove it to use the agent's default model,\n" +
		"# or override all steps at once with: phantom run-chain --model <name>\n\n"

	return header + string(data), nil
}

func findTemplate(name string) *agentTemplate {
	for _, t := range builtinTemplates {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

func parseInlineAgentNames(s string) []string {
	var names []string
	for _, part := range splitAndTrim(s, ",") {
		if part != "" {
			names = append(names, part)
		}
	}
	return names
}

func splitAndTrim(s, sep string) []string {
	parts := make([]string, 0)
	for _, p := range splitString(s, sep) {
		trimmed := trimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	result := make([]string, 0)
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
	}
	return result
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}
