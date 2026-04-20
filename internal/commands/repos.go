package commands

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/martinsuchenak/phantom/internal/node"
	"github.com/paularlott/cli"
)

func NewReposCommand() *cli.Command {
	return &cli.Command{
		Name:        "repos",
		Usage:       "Show repos and the nodes that serve them",
		Description: "Reads the peers state file and displays a mapping of repos to the nodes that host each repo.",
		Run:         doRepos,
	}
}

func doRepos(_ context.Context, _ *cli.Command) error {
	state, err := node.ReadPeersState(cfg.GetPeersStatePath())
	if err != nil {
		return fmt.Errorf("failed to read peers state: %w", err)
	}

	if len(state.Peers) == 0 {
		log.Info("No peers found")
		return nil
	}

	repoMap := make(map[string][]struct {
		NodeID string
		Addr   string
	})
	for _, p := range state.Peers {
		for _, r := range p.Repos {
			repoMap[r] = append(repoMap[r], struct {
				NodeID string
				Addr   string
			}{NodeID: p.ID, Addr: p.GRPCAddr})
		}
	}

	if len(repoMap) == 0 {
		log.Info("No repos found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tNODE\tADDRESS")

	for repo, nodes := range repoMap {
		for i, n := range nodes {
			if i == 0 {
				fmt.Fprintf(w, "%s\t%s\t%s\n", repo, n.NodeID, n.Addr)
			} else {
				fmt.Fprintf(w, "\t%s\t%s\n", n.NodeID, n.Addr)
			}
		}
	}

	return w.Flush()
}
