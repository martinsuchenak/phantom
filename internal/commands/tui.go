package commands

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/paularlott/cli/tui"
	"github.com/paularlott/logger"
)

// tuiWriter acts as an io.Writer mapping lines to TUI messages.
type tuiWriter struct {
	t      *tui.TUI
	agent  string
	buf    []byte
	mu     sync.Mutex
	stream bool
}

// Write buffers chunks and writes full lines to the TUI.
func (w *tuiWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stream {
		// Output raw string to stream
		w.t.StreamChunk(string(p))
	} else {
		// Buffer everything
		w.buf = append(w.buf, p...)
	}
	return len(p), nil
}

// Close flushes the buffered contents or closes the active stream.
func (w *tuiWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.stream {
		w.t.StreamChunk("\n```\n")
		w.t.StreamComplete()
	} else if len(w.buf) > 0 {
		w.t.AddMessageAs(tui.RoleAssistant, w.agent, "```text\n"+string(w.buf)+"\n```")
		w.buf = nil // Clear buffer
	}
	return nil
}

// NewTUIWriter creates an io.Writer that posts to the TUI
func NewTUIWriter(t *tui.TUI, agent string, stream bool) *tuiWriter {
	w := &tuiWriter{t: t, agent: agent, stream: stream}
	if stream {
		t.StartStreamingAs(agent)
		t.StreamChunk("```text\n")
	}
	return w
}

// tuiLogger implements logger.Logger sending logs directly to the TUI.
type tuiLogger struct {
	t       *tui.TUI
	verbose bool
}

func (l *tuiLogger) Trace(msg string, keysAndValues ...any) {
	if l.verbose {
		l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf("[TRACE] "+msg, keysAndValues...))
	}
}

func (l *tuiLogger) Debug(msg string, keysAndValues ...any) {
	if l.verbose {
		l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf("[DEBUG] "+msg, keysAndValues...))
	}
}

func (l *tuiLogger) Info(msg string, keysAndValues ...any) {
	l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf(msg, keysAndValues...))
}

func (l *tuiLogger) Warn(msg string, keysAndValues ...any) {
	l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf("⚠️ [WARN] "+msg, keysAndValues...))
}

func (l *tuiLogger) Error(msg string, keysAndValues ...any) {
	l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf("❌ [ERROR] "+msg, keysAndValues...))
}

func (l *tuiLogger) Fatal(msg string, keysAndValues ...any) {
	l.t.AddMessageAs(tui.RoleSystem, "system", fmt.Sprintf("☠️ [FATAL] "+msg, keysAndValues...))
	l.t.Exit()
	os.Exit(1)
}

func (l *tuiLogger) With(key string, value any) logger.Logger {
	return l
}

func (l *tuiLogger) WithError(err error) logger.Logger {
	return l
}

func (l *tuiLogger) WithGroup(group string) logger.Logger {
	return l
}

// RunWithTUI enables Output-Only TUI mode and executes the provided logic
// concurrently, tearing down the TUI once the logic completes.
func RunWithTUI(ctx context.Context, status string, logic func(ctx context.Context, t *tui.TUI) error) error {
	enabled := false
	t := tui.New(tui.Config{
		InputEnabled: &enabled,
		StatusLeft:   status,
	})

	// Wrap global logger to prevent rendering corruption
	oldLog := log
	log = &tuiLogger{t: t, verbose: verbose}

	var logicErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer t.Exit()
		logicErr = logic(ctx, t)
	}()

	t.Run(ctx)
	wg.Wait()

	// Restore normal logger
	log = oldLog

	return logicErr
}
