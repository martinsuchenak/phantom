package node

import (
	"encoding/json"
	"os"

	"github.com/martinsuchenak/phantom/pkg/api"
)

type PeersState struct {
	SelfID string     `json:"self_id"`
	Peers  []api.Peer `json:"peers"`
}

func WritePeersState(path, selfID string, registry *Registry) error {
	state := PeersState{
		SelfID: selfID,
		Peers:  registry.All(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func ReadPeersState(path string) (*PeersState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state PeersState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
