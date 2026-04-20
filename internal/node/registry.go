package node

import (
	"sync"

	"github.com/martinsuchenak/phantom/pkg/api"
)

type Registry struct {
	mu    sync.RWMutex
	peers map[string]api.Peer
}

func NewRegistry() *Registry {
	return &Registry{peers: make(map[string]api.Peer)}
}

func (r *Registry) Upsert(p api.Peer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[p.ID] = p
}

func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.peers, id)
}

func (r *Registry) FindByRepo(repo string) []api.Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []api.Peer
	for _, p := range r.peers {
		for _, name := range p.Repos {
			if name == repo {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func (r *Registry) All() []api.Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]api.Peer, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, p)
	}
	return out
}
