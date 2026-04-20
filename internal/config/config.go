package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// Project represents a registered local project. The YAML format accepts both
// the legacy plain-string form and the current object form:
//
//	# legacy (auto-migrated on first load)
//	projects:
//	  myapp: /path/to/myapp
//
//	# current
//	projects:
//	  myapp:
//	    path: /path/to/myapp
//	    serve: true   # expose via gRPC for remote overlays
type Project struct {
	Path  string `yaml:"path"`
	Serve bool   `yaml:"serve,omitempty"`
}

// UnmarshalYAML makes Project accept both a bare string (legacy) and a mapping.
func (p *Project) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		p.Path = value.Value
		return nil
	}
	type plain Project
	return value.Decode((*plain)(p))
}

// Config represents the application configuration
type Config struct {
	StateDir string   `yaml:"state_dir"`
	Paths    Paths    `yaml:"paths"`
	Logging  Logging  `yaml:"logging"`
	Overlay  Overlay  `yaml:"overlay"`
	Git      Git      `yaml:"git"`
	Darwin   Darwin   `yaml:"darwin"`
	Linux    Linux    `yaml:"linux"`
	Agent    Agent              `yaml:"agent"`
	AgentEnv []string           `yaml:"agent_env"`
	Projects map[string]Project `yaml:"projects"`
	Node     NodeConfig         `yaml:"node"`
}

// Paths allows overriding individual directory locations
type Paths struct {
	Overlays  string `yaml:"overlays"`  // overlay data (upper dirs)
	Mounts    string `yaml:"mounts"`    // mount points
	Logs      string `yaml:"logs"`      // agent logs
	Snapshots string `yaml:"snapshots"` // snapshots
}

// Logging configuration
type Logging struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// Overlay configuration
type Overlay struct {
	Persistent      bool `yaml:"persistent"`
	AutoCleanupDays int  `yaml:"auto_cleanup_days"`
}

// Git configuration
type Git struct {
	AutoBranch     bool   `yaml:"auto_branch"`
	BranchPrefix   string `yaml:"branch_prefix"`
	AutoPushOnStop bool   `yaml:"auto_push_on_stop"`
}

// Darwin (macOS) specific configuration
type Darwin struct {
	UnionFSPath string   `yaml:"unionfs_path"`
	FUSEOptions []string `yaml:"fuse_options"`
}

// Linux specific configuration
type Linux struct {
	UseFuse         bool     `yaml:"use_fuse"`          // Use fuse-overlayfs instead of native
	FuseOverlayPath string   `yaml:"fuse_overlay_path"` // Path to fuse-overlayfs binary
	FUSEOptions     []string `yaml:"fuse_options"`
}

// Agent configuration
type Agent struct {
	DefaultTimeoutMinutes int  `yaml:"default_timeout_minutes"`
	CleanupOnSuccess      bool `yaml:"cleanup_on_success"`
	CleanupOnFailure      bool `yaml:"cleanup_on_failure"`
}

type NodeAuth struct {
	Mode     string `yaml:"mode"`
	Secret   string `yaml:"secret"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	CAFile   string `yaml:"ca_file"`
}

type NodeRepo struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

type NodeSync struct {
	AutoGitCommit    bool  `yaml:"auto_git_commit"`
	MaxFileSizeBytes int64 `yaml:"max_file_size_bytes"`
}

type NodeConfig struct {
	ID         string     `yaml:"id"`
	GossipPort int        `yaml:"gossip_port"`
	GRPCPort   int        `yaml:"grpc_port"`
	Seeds      []string   `yaml:"seeds"`
	// Repos is the legacy field. On load it is migrated to Projects with
	// Serve: true and then cleared. Use cfg.Projects instead.
	Repos      []NodeRepo `yaml:"repos,omitempty"`
	Auth       NodeAuth   `yaml:"auth"`
	Sync       NodeSync   `yaml:"sync"`
}

// MaxTimeoutMinutes is the maximum allowed timeout value
const MaxTimeoutMinutes = 1440 // 24 hours

// ValidLogLevels contains the allowed log level values
var ValidLogLevels = map[string]bool{
	"trace": true,
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
	"fatal": true,
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	stateDir := filepath.Join(homeDir, ".phantom")

	return &Config{
		StateDir: stateDir,
		Logging: Logging{
			Level: "info",
			File:  filepath.Join(stateDir, "phantom.log"),
		},
		Overlay: Overlay{
			Persistent:      false,
			AutoCleanupDays: 7,
		},
		Git: Git{
			AutoBranch:     true,
			BranchPrefix:   "phantom/",
			AutoPushOnStop: false,
		},
		Darwin: Darwin{
			UnionFSPath: "", // auto-detect
			FUSEOptions: []string{"cow"},
		},
		Linux: Linux{
			UseFuse:         false, // use native overlayfs by default
			FuseOverlayPath: "",    // auto-detect
			FUSEOptions:     []string{},
		},
		Agent: Agent{
			DefaultTimeoutMinutes: 60,
			CleanupOnSuccess:      true,
			CleanupOnFailure:      false,
		},
		AgentEnv: []string{
			"OVERLAY_ENABLED=true",
		},
		Projects: make(map[string]Project),
		Node: NodeConfig{
			GossipPort: 7946,
			GRPCPort:   50051,
			Seeds:      []string{},
			Repos:      []NodeRepo{},
			Auth: NodeAuth{
				Mode: "none",
			},
			Sync: NodeSync{
				AutoGitCommit:    true,
				MaxFileSizeBytes: 50 * 1024 * 1024,
			},
		},
	}
}

// Load loads configuration from a file
func Load(path string) (*Config, error) {
	// Start with defaults
	cfg := DefaultConfig()

	// If no path specified, use default
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".phantom", "config.yaml")
	}

	// Check if config file exists
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return defaults if no config file
			return cfg, nil
		}
		return nil, err
	}

	// Merge file config over defaults
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Expand home directory in paths
	cfg.StateDir = expandHome(cfg.StateDir)
	cfg.Logging.File = expandHome(cfg.Logging.File)
	cfg.Paths.Overlays = expandHome(cfg.Paths.Overlays)
	cfg.Paths.Mounts = expandHome(cfg.Paths.Mounts)
	cfg.Paths.Logs = expandHome(cfg.Paths.Logs)
	cfg.Paths.Snapshots = expandHome(cfg.Paths.Snapshots)

	// Ensure Projects map is initialized if missing from yaml
	if cfg.Projects == nil {
		cfg.Projects = make(map[string]Project)
	}

	// Migrate legacy node.repos entries into Projects with Serve: true.
	if len(cfg.Node.Repos) > 0 {
		for _, r := range cfg.Node.Repos {
			if existing, ok := cfg.Projects[r.Name]; ok {
				existing.Serve = true
				cfg.Projects[r.Name] = existing
			} else {
				cfg.Projects[r.Name] = Project{Path: r.Path, Serve: true}
			}
		}
		cfg.Node.Repos = nil
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks the configuration for invalid values
func (c *Config) Validate() error {
	// Validate log level
	if c.Logging.Level != "" && !ValidLogLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level %q: must be one of trace, debug, info, warn, error, fatal", c.Logging.Level)
	}

	// Validate timeout
	if c.Agent.DefaultTimeoutMinutes < 0 {
		return fmt.Errorf("default_timeout_minutes cannot be negative")
	}
	if c.Agent.DefaultTimeoutMinutes > MaxTimeoutMinutes {
		return fmt.Errorf("default_timeout_minutes cannot exceed %d (24 hours)", MaxTimeoutMinutes)
	}

	// Validate auto cleanup days
	if c.Overlay.AutoCleanupDays < 0 {
		return fmt.Errorf("auto_cleanup_days cannot be negative")
	}

	// Validate state directory is not empty
	if c.StateDir == "" {
		return fmt.Errorf("state_dir cannot be empty")
	}

	return nil
}

// Save saves configuration to a file
func (c *Config) Save(path string) error {
	if path == "" {
		homeDir, _ := os.UserHomeDir()
		path = filepath.Join(homeDir, ".phantom", "config.yaml")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// EnsureNodeDefaults fills in any missing NodeConfig fields with sensible
// defaults. It returns a human-readable description of each field that was
// set so the caller can log or display them. It does not save the config.
func (c *Config) EnsureNodeDefaults() []string {
	nc := &c.Node
	var changed []string

	if nc.ID == "" {
		nc.ID = uuid.New().String()
		changed = append(changed, fmt.Sprintf("node.id = %q (generated UUID)", nc.ID))
	}
	if nc.GRPCPort == 0 {
		nc.GRPCPort = 50051
		changed = append(changed, "node.grpc_port = 50051")
	}
	if nc.GossipPort == 0 {
		nc.GossipPort = 7946
		changed = append(changed, "node.gossip_port = 7946")
	}
	if nc.Auth.Mode == "" {
		nc.Auth.Mode = "none"
		changed = append(changed, "node.auth.mode = \"none\"")
	}
	if nc.Sync.MaxFileSizeBytes == 0 {
		nc.Sync.MaxFileSizeBytes = 50 * 1024 * 1024
		changed = append(changed, "node.sync.max_file_size_bytes = 52428800 (50 MiB)")
	}

	return changed
}

// GetStatePath returns the full path to the state directory
func (c *Config) GetStatePath() string {
	return c.StateDir
}

// GetOverlaysPath returns the path where overlay data is stored
func (c *Config) GetOverlaysPath() string {
	if c.Paths.Overlays != "" {
		return c.Paths.Overlays
	}
	return filepath.Join(c.StateDir, "overlays")
}

// GetMountPath returns the path where overlays are mounted
func (c *Config) GetMountPath() string {
	if c.Paths.Mounts != "" {
		return c.Paths.Mounts
	}
	return filepath.Join(c.StateDir, "mnt")
}

// GetLogsPath returns the path where agent logs are stored
func (c *Config) GetLogsPath() string {
	if c.Paths.Logs != "" {
		return c.Paths.Logs
	}
	return filepath.Join(c.StateDir, "logs")
}

// GetSnapshotsPath returns the path where snapshots are stored
func (c *Config) GetSnapshotsPath() string {
	if c.Paths.Snapshots != "" {
		return c.Paths.Snapshots
	}
	return filepath.Join(c.StateDir, "snapshots")
}

func (c *Config) GetRemoteMountsPath() string {
	return filepath.Join(c.StateDir, "remote-mounts")
}

func (c *Config) GetNodePIDPath() string {
	return filepath.Join(c.StateDir, "node.pid")
}

func (c *Config) GetPeersStatePath() string {
	return filepath.Join(c.StateDir, "peers.json")
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}
