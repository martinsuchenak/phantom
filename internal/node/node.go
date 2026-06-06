package node

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/martinsuchenak/phantom/pkg/api"
	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/logger"
)

type Meta struct {
	ID       string   `json:"id"`
	GRPCAddr string   `json:"grpc_addr"`
	Repos    []string `json:"repos"`
	Version  int      `json:"version"`
}

type Config struct {
	ID            string
	BindAddr      string
	AdvertiseAddr string
	GRPCAddr      string
	Seeds         []string
	Repos         []string
	PIDFile       string
	Logger        logger.Logger
	// RepoUpdateCh receives updated repo slices when the served project list
	// changes at runtime. A nil channel disables live updates.
	RepoUpdateCh <-chan []string
}

func upsertFromMeta(registry *Registry, nodeMeta gossip.MetadataReader) {
	all := nodeMeta.GetAll()
	raw, ok := all["node_meta"]
	if !ok {
		return
	}
	metaStr, ok := raw.(string)
	if !ok {
		return
	}
	var m Meta
	if err := json.Unmarshal([]byte(metaStr), &m); err == nil && m.ID != "" {
		registry.Upsert(api.Peer{ID: m.ID, GRPCAddr: m.GRPCAddr, Repos: m.Repos})
	}
}

func Start(ctx context.Context, cfg Config, registry *Registry) error {
	nodeID, err := resolveNodeID(cfg.ID, cfg.PIDFile)
	if err != nil {
		return fmt.Errorf("resolve node ID: %w", err)
	}

	if cfg.PIDFile != "" {
		if err := os.WriteFile(cfg.PIDFile, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
			return fmt.Errorf("write pid file: %w", err)
		}
		defer func() { _ = os.Remove(cfg.PIDFile) }()
	}

	gossipCfg := gossip.DefaultConfig()
	gossipCfg.NodeID = nodeID
	gossipCfg.BindAddr = cfg.BindAddr
	if cfg.AdvertiseAddr != "" {
		gossipCfg.AdvertiseAddr = cfg.AdvertiseAddr
	} else {
		gossipCfg.AdvertiseAddr = cfg.BindAddr
	}
	gossipCfg.Transport = gossip.NewSocketTransport(gossipCfg)
	gossipCfg.MsgCodec = codec.NewJsonCodec()

	cluster, err := gossip.NewCluster(gossipCfg)
	if err != nil {
		return fmt.Errorf("create gossip node: %w", err)
	}

	cluster.Start()

	metaBytes, _ := json.Marshal(Meta{
		ID:       nodeID,
		GRPCAddr: cfg.GRPCAddr,
		Repos:    cfg.Repos,
		Version:  1,
	})
	cluster.LocalMetadata().SetString("node_meta", string(metaBytes))

	cluster.HandleNodeStateChangeFunc(func(node *gossip.Node, prev gossip.NodeState) {
		cur := node.GetObservedState()
		cfg.Logger.Debug("gossip: node %s state change %s -> %s", node.ID, prev, cur)
		if cur == gossip.NodeDead || cur == gossip.NodeLeaving {
			registry.Remove(node.ID.String())
			cfg.Logger.Info("gossip: peer %s left (%s)", node.ID, cur)
		} else if prev == gossip.NodeDead || prev == gossip.NodeUnknown {
			cfg.Logger.Info("gossip: peer %s joined (%s)", node.ID, cur)
		} else {
			cfg.Logger.Debug("gossip: peer %s state %s -> %s", node.ID, prev, cur)
		}
	})

	cluster.HandleNodeMetadataChangeFunc(func(node *gossip.Node) {
		all := node.Metadata.GetAll()
		raw, ok := all["node_meta"]
		if !ok {
			cfg.Logger.Debug("gossip: peer %s metadata changed but no node_meta found", node.ID)
			return
		}
		metaStr, ok := raw.(string)
		if !ok {
			return
		}
		var m Meta
		if err := json.Unmarshal([]byte(metaStr), &m); err == nil && m.ID != "" {
			registry.Upsert(api.Peer{ID: m.ID, GRPCAddr: m.GRPCAddr, Repos: m.Repos})
			cfg.Logger.Info("gossip: peer %s metadata updated (repos: %s)", m.ID, strings.Join(m.Repos, ", "))
		}
	})

	for _, n := range cluster.Nodes() {
		if n.ID != cluster.LocalNode().ID {
			upsertFromMeta(registry, n.Metadata)
			cfg.Logger.Info("gossip: discovered existing peer %s", n.ID)
		}
	}

	if len(cfg.Seeds) > 0 {
		cfg.Logger.Info("gossip: joining ring via seeds: %s", strings.Join(cfg.Seeds, ", "))
		if err := cluster.Join(cfg.Seeds); err != nil {
			return fmt.Errorf("join gossip ring: %w", err)
		}
		cfg.Logger.Info("gossip: joined ring successfully")
	}

	for {
		select {
		case <-ctx.Done():
			cfg.Logger.Info("gossip: shutting down, leaving ring")
			cluster.Leave()
			return nil
		case repos, ok := <-cfg.RepoUpdateCh:
			if !ok {
				continue
			}
			updated, _ := json.Marshal(Meta{
				ID:       nodeID,
				GRPCAddr: cfg.GRPCAddr,
				Repos:    repos,
				Version:  1,
			})
			cluster.LocalMetadata().SetString("node_meta", string(updated))
			cfg.Logger.Info("gossip: updated advertised repos: %v", repos)
		}
	}
}

func nodeIDPath(pidFile string) string {
	if pidFile == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(pidFile), "node_id")
}

func resolveNodeID(configuredID, pidFile string) (string, error) {
	if _, err := uuid.Parse(configuredID); err == nil {
		return configuredID, nil
	}

	idFile := nodeIDPath(pidFile)
	if idFile == "" {
		return uuid.New().String(), nil
	}

	data, err := os.ReadFile(idFile)
	if err == nil {
		stored := string(data)
		if _, perr := uuid.Parse(stored); perr == nil {
			return stored, nil
		}
	}

	generated := uuid.New().String()
	_ = os.WriteFile(idFile, []byte(generated), 0600)
	return generated, nil
}
