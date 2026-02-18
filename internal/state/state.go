package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/martinsuchenak/phantom/pkg/api"
)

// Store manages overlay state persistence
type Store struct {
	stateDir string
}

// NewStore creates a new state store
func NewStore(stateDir string) (*Store, error) {
	statePath := filepath.Join(stateDir, "state")
	if err := os.MkdirAll(statePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}
	return &Store{stateDir: statePath}, nil
}

// Save persists an overlay's state to disk
func (s *Store) Save(overlay *api.Overlay) error {
	if overlay.Name == "" {
		return fmt.Errorf("overlay name is required")
	}

	path := s.overlayPath(overlay.Name)
	data, err := json.MarshalIndent(overlay, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal overlay state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write overlay state: %w", err)
	}

	return nil
}

// Load retrieves an overlay's state from disk
func (s *Store) Load(name string) (*api.Overlay, error) {
	if name == "" {
		return nil, fmt.Errorf("overlay name is required")
	}

	path := s.overlayPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, api.NewError(api.ErrNotFound, fmt.Sprintf("overlay %q not found", name), err)
		}
		return nil, fmt.Errorf("failed to read overlay state: %w", err)
	}

	var overlay api.Overlay
	if err := json.Unmarshal(data, &overlay); err != nil {
		return nil, fmt.Errorf("failed to unmarshal overlay state: %w", err)
	}

	return &overlay, nil
}

// LoadAll retrieves all overlay states from disk
func (s *Store) LoadAll() ([]*api.Overlay, error) {
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*api.Overlay{}, nil
		}
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var overlays []*api.Overlay
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-5] // remove .json extension
		overlay, err := s.Load(name)
		if err != nil {
			// Log warning but continue loading others
			fmt.Fprintf(os.Stderr, "warning: failed to load overlay %q: %v\n", name, err)
			continue
		}
		overlays = append(overlays, overlay)
	}

	return overlays, nil
}

// Delete removes an overlay's state from disk
func (s *Store) Delete(name string) error {
	if name == "" {
		return fmt.Errorf("overlay name is required")
	}

	path := s.overlayPath(name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil // Already gone
		}
		return fmt.Errorf("failed to delete overlay state: %w", err)
	}

	return nil
}

// Exists checks if an overlay state exists
func (s *Store) Exists(name string) bool {
	path := s.overlayPath(name)
	_, err := os.Stat(path)
	return err == nil
}

func (s *Store) overlayPath(name string) string {
	return filepath.Join(s.stateDir, name+".json")
}
