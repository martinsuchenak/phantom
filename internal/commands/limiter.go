package commands

import (
	"path/filepath"
	"strings"
	"sync"
)

// AgentLimiter manages concurrency limits per agent type (executable)
type AgentLimiter struct {
	mu     sync.Mutex
	limits map[string]chan struct{}
	limit  int
}

// NewAgentLimiter creates a new limiter with the specified max concurrency per type.
// A limit <= 0 means unlimited.
func NewAgentLimiter(limit int) *AgentLimiter {
	return &AgentLimiter{
		limits: make(map[string]chan struct{}),
		limit:  limit,
	}
}

// Acquire requests a concurrency token for the specific agent type.
func (l *AgentLimiter) Acquire(agentCmd string) {
	if l.limit <= 0 {
		return
	}
	baseAgent := getBaseAgent(agentCmd)
	if baseAgent == "" {
		return
	}

	l.mu.Lock()
	ch, ok := l.limits[baseAgent]
	if !ok {
		ch = make(chan struct{}, l.limit)
		l.limits[baseAgent] = ch
	}
	l.mu.Unlock()

	ch <- struct{}{}
}

// Release releases the token for the specific agent type.
func (l *AgentLimiter) Release(agentCmd string) {
	if l.limit <= 0 {
		return
	}
	baseAgent := getBaseAgent(agentCmd)
	if baseAgent == "" {
		return
	}

	l.mu.Lock()
	ch, ok := l.limits[baseAgent]
	l.mu.Unlock()

	if ok {
		<-ch
	}
}

func getBaseAgent(agentCmd string) string {
	parts := strings.Fields(agentCmd)
	if len(parts) == 0 {
		return ""
	}
	return filepath.Base(parts[0])
}
