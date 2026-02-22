package commands

import (
	"context"
	"testing"
)

func TestDoCommands(t *testing.T) {
	tmpDir := t.TempDir()
	setupTestEnv(t, tmpDir)

	oldLog := log
	log = &MockLogger{}
	defer func() {
		log = oldLog
	}()

	t.Run("clone", func(t *testing.T) {
		runCommandWithArgs(t, []string{"clone"}, func() {
			_ = doClone(context.Background(), NewCloneCommand())
		})
	})

	t.Run("compare", func(t *testing.T) {
		runCommandWithArgs(t, []string{"compare"}, func() {
			_ = doCompare(context.Background(), NewCompareCommand())
		})
	})

	t.Run("config", func(t *testing.T) {
		runCommandWithArgs(t, []string{"config", "validate"}, func() {
			_ = doConfigValidate(context.Background(), NewConfigCommand())
		})
	})

	t.Run("conflicts", func(t *testing.T) {
		runCommandWithArgs(t, []string{"conflicts"}, func() {
			_ = doConflicts(context.Background(), NewConflictsCommand())
		})
	})

	t.Run("diff", func(t *testing.T) {
		runCommandWithArgs(t, []string{"diff"}, func() {
			_ = doDiff(context.Background(), NewDiffCommand())
		})
	})

	t.Run("export", func(t *testing.T) {
		runCommandWithArgs(t, []string{"export"}, func() {
			_ = doExport(context.Background(), NewExportCommand())
		})
	})

	t.Run("gc", func(t *testing.T) {
		runCommandWithArgs(t, []string{"gc"}, func() {
			_ = doGC(context.Background(), NewGCCommand())
		})
	})

	t.Run("hook", func(t *testing.T) {
		runCommandWithArgs(t, []string{"hook", "list"}, func() { _ = doHookList(context.Background(), NewHookCommand()) })
		runCommandWithArgs(t, []string{"hook", "add"}, func() { _ = doHookAdd(context.Background(), NewHookCommand()) })
		runCommandWithArgs(t, []string{"hook", "remove"}, func() { _ = doHookRemove(context.Background(), NewHookCommand()) })
		runCommandWithArgs(t, []string{"hook", "init"}, func() { _ = doHookInit(context.Background(), NewHookCommand()) })
	})

	t.Run("init", func(t *testing.T) {
		runCommandWithArgs(t, []string{"init"}, func() {
			_ = doInit(context.Background(), NewInitCommand())
		})
	})

	t.Run("inspect", func(t *testing.T) {
		runCommandWithArgs(t, []string{"inspect"}, func() {
			_ = doInspect(context.Background(), NewInspectCommand())
		})
	})

	t.Run("logs", func(t *testing.T) {
		runCommandWithArgs(t, []string{"logs"}, func() {
			_ = doLogs(context.Background(), NewLogsCommand())
		})
	})

	t.Run("merge", func(t *testing.T) {
		runCommandWithArgs(t, []string{"merge"}, func() {
			_ = doMerge(context.Background(), NewMergeCommand())
		})
	})

	t.Run("rename", func(t *testing.T) {
		runCommandWithArgs(t, []string{"rename"}, func() {
			_ = doRename(context.Background(), NewRenameCommand())
		})
	})

	t.Run("replay", func(t *testing.T) {
		runCommandWithArgs(t, []string{"replay"}, func() {
			_ = doReplay(context.Background(), NewReplayCommand())
		})
	})

	t.Run("restart", func(t *testing.T) {
		runCommandWithArgs(t, []string{"restart"}, func() {
			_ = doRestart(context.Background(), NewRestartCommand())
		})
	})

	t.Run("run", func(t *testing.T) {
		runCommandWithArgs(t, []string{"run"}, func() {
			// run needs an agent empty name to fail validation or we just run it and let it fail early
			_ = doRun(context.Background(), NewRunCommand())
		})
	})

	t.Run("runall", func(t *testing.T) {
		runCommandWithArgs(t, []string{"runall"}, func() {
			_ = doRunAll(context.Background(), NewRunAllCommand())
		})
	})

	t.Run("runchain", func(t *testing.T) {
		runCommandWithArgs(t, []string{"runchain"}, func() {
			_ = doRunChain(context.Background(), NewRunChainCommand())
		})
	})

	t.Run("start", func(t *testing.T) {
		runCommandWithArgs(t, []string{"start"}, func() {
			_ = doStart(context.Background(), NewStartCommand())
		})
	})

	t.Run("stop", func(t *testing.T) {
		runCommandWithArgs(t, []string{"stop"}, func() {
			_ = doStop(context.Background(), NewStopCommand())
		})
	})

	t.Run("sync", func(t *testing.T) {
		runCommandWithArgs(t, []string{"sync"}, func() {
			_ = doSync(context.Background(), NewSyncCommand())
		})
	})

	t.Run("watch", func(t *testing.T) {
		runCommandWithArgs(t, []string{"watch"}, func() {
			_ = doWatch(context.Background(), NewWatchCommand())
		})
	})

	t.Run("unpin", func(t *testing.T) {
		runCommandWithArgs(t, []string{"unpin"}, func() {
			_ = doUnpin(context.Background(), NewUnpinCommand())
		})
	})
}
