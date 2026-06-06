package commands

import "testing"

func TestCommandConstructors(t *testing.T) {
	tests := []struct {
		name string
		fn   func() interface{ GetName() string }
	}{
		// We test the Name field directly since cli.Command may not have GetName
	}
	_ = tests

	// Test all command constructors return correct names
	cmds := map[string]func(){
		"diff":      func() { c := NewDiffCommand(); assertName(t, c.Name, "diff") },
		"export":    func() { c := NewExportCommand(); assertName(t, c.Name, "export") },
		"template":  func() { c := NewTemplateCommand(); assertName(t, c.Name, "template") },
		"run-all":   func() { c := NewRunAllCommand(); assertName(t, c.Name, "run-all") },
		"gc":        func() { c := NewGCCommand(); assertName(t, c.Name, "gc") },
		"snapshot":  func() { c := NewSnapshotCommand(); assertName(t, c.Name, "snapshot") },
		"init":      func() { c := NewInitCommand(); assertName(t, c.Name, "init") },
		"conflicts": func() { c := NewConflictsCommand(); assertName(t, c.Name, "conflicts") },
	}

	for name, fn := range cmds {
		t.Run(name, func(t *testing.T) {
			fn()
		})
	}
}

func assertName(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("command name = %q, want %q", got, want)
	}
}
