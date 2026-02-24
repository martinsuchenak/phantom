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
		fmt.Printf("\033[%dA\033[J", pt.lines) // Clear previously rendered lines
	}

	var output []string
	output = append(output, "\n\033[1m🚀 Pipeline Progress\033[0m\n")

	children := make(map[string][]string)
	agentDeps := make(map[string][]string)
	roots := []string{}

	for _, a := range pt.agents {
		agentDeps[a.Name] = a.DependsOn
		if len(a.DependsOn) == 0 {
			roots = append(roots, a.Name)
		}
		for _, dep := range a.DependsOn {
			children[dep] = append(children[dep], a.Name)
		}
	}

	visited := make(map[string]bool)

	var doPrint func(node, prefix string, isLast bool)
	doPrint = func(node, prefix string, isLast bool) {
		if visited[node] {
			return
		}
		visited[node] = true

		state := pt.states[node]

		var icon string
		var color string
		switch state {
		case StatePending:
			icon = "⏳"
			color = "\033[90m" // dark grey
		case StateSkipped:
			icon = "⏭️ "
			color = "\033[33m" // yellow
		case StateStarting, StateFetching, StateMerging:
			icon = "⚙️ "
			color = "\033[36m" // cyan
		case StateQueued:
			icon = "⏸️ "
			color = "\033[35m" // magenta
		case StateRunning:
			icon = "🏃"
			color = "\033[34m" // blue
		case StateDone:
			icon = "✅"
			color = "\033[32m" // green
		case StateFailed:
			icon = "❌"
			color = "\033[31m" // red
		}

		connector := "├──"
		if isLast {
			connector = "└──"
		}

		line := fmt.Sprintf("%s%s %s %s [%s]\033[0m", prefix, connector, icon, color+node, state)

		// Append live "needs" suffix for nodes with dependencies
		deps := agentDeps[node]
		if len(deps) > 0 {
			parts := make([]string, 0, len(deps))
			for _, dep := range deps {
				parts = append(parts, fmt.Sprintf("%s %s", dep, stateIcon(pt.states[dep])))
			}
			line += fmt.Sprintf(" \033[90m(needs: %s)\033[0m", strings.Join(parts, ", "))
		}

		output = append(output, line)

		childs := children[node]
		for i, child := range childs {
			childPrefix := prefix
			if isLast {
				childPrefix += "    "
			} else {
				childPrefix += "│   "
			}
			doPrint(child, childPrefix, i == len(childs)-1)
		}
	}

	for i, root := range roots {
		doPrint(root, "", i == len(roots)-1)
	}

	output = append(output, "") // Extra blank line at the end
	outStr := strings.Join(output, "\n")
	fmt.Print(outStr)

	pt.lines = strings.Count(outStr, "\n")
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
