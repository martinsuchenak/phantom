package main

import (
	"github.com/martinsuchenak/phantom/internal/commands"
)

// Set by goreleaser
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	commands.SetVersion(version, commit, date)
	commands.Execute()
}
