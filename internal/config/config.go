package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	StateDir string   `yaml:"state_dir"`
	Logging  Logging  `yaml:"logging"`
	Overlay  Overlay  `yaml:"overlay"`
	Git      Git      `yaml:"git"`
	Darwin   Darwin   `yaml:"darwin"`
	Linux    Linux    `yaml:"linux"`
	Agent    Agent    `yaml:"agent"`
	AgentEnv []string `yaml:"agent_env"`
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

// GetStatePath returns the full path to the state directory
func (c *Config) GetStatePath() string {
	return c.StateDir
}

// GetOverlaysPath returns the path where overlay data is stored
func (c *Config) GetOverlaysPath() string {
	return filepath.Join(c.StateDir, "overlays")
}

// GetMountPath returns the path where overlays are mounted
func (c *Config) GetMountPath() string {
	return filepath.Join(c.StateDir, "mnt")
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, path[1:])
	}
	return path
}
