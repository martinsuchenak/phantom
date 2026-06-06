package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	phantommdns "github.com/martinsuchenak/phantom/internal/mdns"
	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/cli"
)

func NewNodeListCommand() *cli.Command {
	return &cli.Command{
		Name:        "list",
		Usage:       "List known cluster peers",
		Description: "Reads the peers state file and displays known cluster peers. Use --mdns to discover live peers on the LAN instead.",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "mdns",
				Usage: "Discover peers live via mDNS instead of reading the cached peers state",
			},
		},
		Run: doNodeList,
	}
}

func doNodeList(_ context.Context, cmd *cli.Command) error {
	var peers []api.Peer

	if cmd.GetBool("mdns") {
		log.Info("Probing LAN via mDNS (timeout: %s)...", phantommdns.DefaultTimeout)
		discovered, err := phantommdns.Discover(0)
		if err != nil {
			return fmt.Errorf("mDNS discovery failed: %w", err)
		}
		peers = discovered
	} else {
		state, err := node.ReadPeersState(cfg.GetPeersStatePath())
		if err != nil {
			return fmt.Errorf("failed to read peers state: %w", err)
		}
		peers = state.Peers
	}

	if len(peers) == 0 {
		log.Info("No peers found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tGRPC ADDR\tREPOS")
	for _, p := range peers {
		repos := "-"
		if len(p.Repos) > 0 {
			repos = fmt.Sprintf("%v", p.Repos)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.GRPCAddr, repos)
	}
	return w.Flush()
}
