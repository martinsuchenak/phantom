package commands

import (
	"github.com/paularlott/cli"
)

// NewSnapshotCommand creates the snapshot command with subcommands
func NewSnapshotCommand() *cli.Command {
	return &cli.Command{
		Name:  "snapshot",
		Usage: "Manage overlay snapshots",
		Commands: []*cli.Command{
			{
				Name:  "save",
				Usage: "Save a snapshot of an overlay",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "snapshot-name", Aliases: []string{"s"}, Usage: "Snapshot name"},
				},
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
				},
				Run: doSnapshotSave,
			},
			{
				Name:  "restore",
				Usage: "Restore an overlay from a snapshot",
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "name", Usage: "Overlay name", Required: true},
					&cli.StringArg{Name: "snapshot", Usage: "Snapshot name", Required: true},
				},
				Run: doSnapshotRestore,
			},
			{
				Name:  "list",
				Usage: "List snapshots",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "format", Usage: "Output format (table, json)", DefaultValue: "table"},
				},
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "name", Usage: "Overlay name (optional)", Required: false},
				},
				Run: doSnapshotList,
			},
			{
				Name:  "delete",
				Usage: "Delete a snapshot",
				Arguments: []cli.Argument{
					&cli.StringArg{Name: "snapshot", Usage: "Snapshot name", Required: true},
				},
				Run: doSnapshotDelete,
			},
		},
	}
}
