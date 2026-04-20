package node

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/martinsuchenak/phantom/pkg/api"
)

type Meta struct {
	ID       string   `json:"id"`
	GRPCAddr string   `json:"grpc_addr"`
	Repos    []string `json:"repos"`
	Version  int      `json:"version"`
}

type Config struct {
	ID       string
	BindAddr string
	GRPCAddr string
	Seeds    []string
	Repos    []string
	PIDFile  string
}

func upsertFromMeta(registry *Registry, nodeMeta gossip.MetadataReader) {
	raw, _ := json.Marshal(nodeMeta.GetAll())
	var m Meta
	if err := json.Unmarshal(raw, &m); err == nil && m.ID != "" {
		registry.Upsert(api.Peer{ID: m.ID, GRPCAddr: m.GRPCAddr, Repos: m.Repos})
	}
}

func Start(ctx context.Context, cfg Config, registry *Registry) error {
	if cfg.PIDFile != "" {
		if err := os.WriteFile(cfg.PIDFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
			return fmt.Errorf("write pid file: %w", err)
		}
		defer os.Remove(cfg.PIDFile)
	}

	gossipCfg := gossip.DefaultConfig()
	gossipCfg.NodeID = cfg.ID
	gossipCfg.BindAddr = cfg.BindAddr
	gossipCfg.AdvertiseAddr = cfg.BindAddr
	gossipCfg.Transport = gossip.NewSocketTransport(gossipCfg)
	gossipCfg.MsgCodec = codec.NewJsonCodec()

	cluster, err := gossip.NewCluster(gossipCfg)
	if err != nil {
		return fmt.Errorf("create gossip node: %w", err)
	}

	metaBytes, _ := json.Marshal(Meta{
		ID:       cfg.ID,
		GRPCAddr: cfg.GRPCAddr,
		Repos:    cfg.Repos,
		Version:  1,
	})
	cluster.LocalMetadata().SetString("node_meta", string(metaBytes))

	cluster.HandleNodeStateChangeFunc(func(node *gossip.Node, prev gossip.NodeState) {
		if node.GetObservedState() == gossip.NodeDead || node.GetObservedState() == gossip.NodeLeaving {
			registry.Remove(node.ID.String())
		}
	})

	cluster.HandleNodeMetadataChangeFunc(func(node *gossip.Node) {
		upsertFromMeta(registry, node.Metadata)
	})

	for _, n := range cluster.Nodes() {
		if n.ID != cluster.LocalNode().ID {
			upsertFromMeta(registry, n.Metadata)
		}
	}

	if len(cfg.Seeds) > 0 {
		if err := cluster.Join(cfg.Seeds); err != nil {
			return fmt.Errorf("join gossip ring: %w", err)
		}
	}

	<-ctx.Done()
	cluster.Leave()
	return nil
}
