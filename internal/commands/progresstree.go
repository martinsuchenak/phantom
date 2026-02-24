package commands

import (
	"fmt"
	"strings"
	"sync"
)

type AgentState string

const (
	StatePending  AgentState = "Pending"
	StateSkipped  AgentState = "Skipped"
	StateStarting AgentState = "Starting"
	StateFetching AgentState = "Fetching"
	StateMerging  AgentState = "Merging"
	StateQueued   AgentState = "Queued"
	StateRunning  AgentState = "Running"
	StateDone     AgentState = "Done"
	StateFailed   AgentState = "Failed"
)

// pipelineNotifier is the interface processRunPipeline uses to report progress.
// ProgressTree implements it for terminal output; tuiNotifier implements it for
// the interactive TUI.
type pipelineNotifier interface {
	// Update sets the displayed state for an agent.
	Update(agent string, state AgentState)
	// Logf posts an informational message attributed to an agent.
	Logf(format string, args ...any)
	// Errorf posts an error message attributed to an agent.
	Errorf(format string, args ...any)
	// Summary is called once after all agents finish with the final results.
	Summary(results []agentResult)
	// Clear removes any live-rendered UI (no-op for TUI).
	Clear()
}

// ProgressTree manages the terminal UI for the pipeline
type ProgressTree struct {
	mu     sync.Mutex
	agents []pipelineAgent
	states map[string]AgentState
	lines  int
}

func NewProgressTree(agents []pipelineAgent) *ProgressTree {
	pt := &ProgressTree{
		agents: agents,
		states: make(map[string]AgentState),
	}
	for _, a := range agents {
		if a.skip {
			pt.states[a.Name] = StateSkipped
		} else {
			pt.states[a.Name] = StatePending
		}
	}
	return pt
}

func (pt *ProgressTree) Update(agent string, state AgentState) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.states[agent] = state
	pt.render()
}

// Logf prints a formatted message above the progress tree without breaking the UI
func (pt *ProgressTree) Logf(format string, args ...any) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.lines > 0 {
		fmt.Printf("\033[%dA\033[J", pt.lines) // Clear UI
	}

	fmt.Printf(format+"\n", args...) // Print log message

	// Redraw UI
	pt.render()
}

// Errorf prints a formatted error message above the progress tree
func (pt *ProgressTree) Errorf(format string, args ...any) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if pt.lines > 0 {
		fmt.Printf("\033[%dA\033[J", pt.lines) // Clear UI
	}

	errMsg := fmt.Sprintf(format, args...)
	fmt.Printf("\033[31m[ERROR]\033[0m %s\n", errMsg) // Print error message

	// Redraw UI
	pt.render()
}

func stateColor(s AgentState) string {
	switch s {
	case StatePending:
		return "\033[90m" // dark grey
	case StateSkipped:
		return "\033[33m" // yellow
	case StateStarting, StateFetching, StateMerging:
		return "\033[36m" // cyan
	case StateQueued:
		return "\033[35m" // magenta
	case StateRunning:
		return "\033[34m" // blue
	case StateDone:
		return "\033[32m" // green
	case StateFailed:
		return "\033[31m" // red
	default:
		return ""
	}
}

func stateIcon(s AgentState) string {
	switch s {
	case StatePending:
		return "⏳"
	case StateSkipped:
		return "⏭️ "
	case StateStarting, StateFetching, StateMerging:
		return "⚙️ "
	case StateQueued:
		return "⏸️ "
	case StateRunning:
		return "🏃"
	case StateDone:
		return "✅"
	case StateFailed:
		return "❌"
	default:
		return "❓"
	}
}

func (pt *ProgressTree) render() {
	if pt.lines > 0 {
		fmt.Printf("\033[%dA\033[J", pt.lines)
	}

	// Compute column widths from agent names (visible chars only)
	nameWidth := len("AGENT")
	for _, a := range pt.agents {
		if len(a.Name) > nameWidth {
			nameWidth = len(a.Name)
		}
	}

	// Fixed status column: widest visible status string is "⚙️  Fetching" ~ "  Fetching" = 10 chars + icon width
	// We pad the visible portion: icon(2) + space + state name. Longest state name = "Starting" (8).
	// Use a fixed visible width of 14 for the status cell.
	statusWidth := 14

	var output []string
	output = append(output, "")

	// Header
	header := fmt.Sprintf("  \033[1m%-*s  %-*s  %s\033[0m",
		nameWidth, "AGENT",
		statusWidth, "STATUS",
		"WAITING ON")
	sep := fmt.Sprintf("  %s  %s  %s",
		strings.Repeat("─", nameWidth),
		strings.Repeat("─", statusWidth),
		strings.Repeat("─", 20))
	output = append(output, header)
	output = append(output, sep)

	for _, a := range pt.agents {
		state := pt.states[a.Name]
		icon := stateIcon(state)
		color := stateColor(state)

		// Build visible status string for padding calc, then colorize
		visibleStatus := fmt.Sprintf("%s %s", icon, state)
		// Pad to statusWidth using visible length (no ANSI codes in visibleStatus yet)
		padded := visibleStatus + strings.Repeat(" ", max(0, statusWidth-len(visibleStatus)))
		statusCell := color + padded + "\033[0m"

		// Build waiting-on column
		waitOn := ""
		if len(a.DependsOn) > 0 {
			parts := make([]string, 0, len(a.DependsOn))
			for _, dep := range a.DependsOn {
				parts = append(parts, fmt.Sprintf("%s %s", dep, stateIcon(pt.states[dep])))
			}
			waitOn = "\033[90m" + strings.Join(parts, ", ") + "\033[0m"
		}

		nameCell := fmt.Sprintf("%-*s", nameWidth, a.Name)
		line := fmt.Sprintf("  %s  %s  %s", nameCell, statusCell, waitOn)
		output = append(output, line)
	}

	output = append(output, "") // trailing blank line
	outStr := strings.Join(output, "\n")
	fmt.Print(outStr)

	pt.lines = strings.Count(outStr, "\n")
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Clear removes the progress tree from screen
func (pt *ProgressTree) Clear() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if pt.lines > 0 {
		fmt.Printf("\033[%dA\033[J", pt.lines)
		pt.lines = 0
	}
}

// Summary is a no-op for ProgressTree — the terminal summary is printed by
// printRunAllSummary after the tree is cleared.
func (pt *ProgressTree) Summary(_ []agentResult) {}
