package api

import (
	"io"
	"time"
)

// Overlay represents a single overlay filesystem instance
type Overlay struct {
	Name         string    `json:"name" yaml:"name"`
	BaseDir      string    `json:"base_dir" yaml:"base_dir"`
	MountPoint   string    `json:"mount_point" yaml:"mount_point"`
	UpperDir     string    `json:"upper_dir" yaml:"upper_dir"`
	WorkDir      string    `json:"work_dir" yaml:"work_dir"` // Linux only
	Branch       string    `json:"branch" yaml:"branch"`
	Persistent   bool      `json:"persistent" yaml:"persistent"`
	Locked       bool      `json:"locked,omitempty" yaml:"locked,omitempty"`
	PinnedCommit string    `json:"pinned_commit,omitempty" yaml:"pinned_commit,omitempty"`
	CreatedAt    time.Time `json:"created_at" yaml:"created_at"`
	PID          int       `json:"pid,omitempty" yaml:"pid,omitempty"`           // macOS unionfs / Linux fuse-overlayfs process
	UseFuse      bool      `json:"use_fuse,omitempty" yaml:"use_fuse,omitempty"` // Linux: whether fuse-overlayfs was used
}

// OverlayStatus represents the current status of an overlay
type OverlayStatus struct {
	Name      string        `json:"name"`
	Mounted   bool          `json:"mounted"`
	SizeBytes int64         `json:"size_bytes"`
	Uptime    time.Duration `json:"uptime"`
	AgentPID  int           `json:"agent_pid,omitempty"`
}

// CreateOptions contains options for creating a new overlay
type CreateOptions struct {
	Name       string
	BaseDir    string
	Branch     string
	Persistent bool
}

// RunOptions contains options for running an agent in an overlay context
type RunOptions struct {
	Agent     string
	Task      string
	Model     string // optional: substituted as {model} in agent command
	BaseDir   string
	Name      string
	Timeout   time.Duration
	Cleanup   bool
	PushOnEnd bool
	Headless  bool      // No stdin, log-only output (for parallel runs)
	Stdout    io.Writer // Optional: custom stdout writer
	Stderr    io.Writer // Optional: custom stderr writer
}

// Error codes
const (
	ErrMountFailed       = "MOUNT_FAILED"
	ErrUnmountFailed     = "UNMOUNT_FAILED"
	ErrNotFound          = "NOT_FOUND"
	ErrAlreadyExists     = "ALREADY_EXISTS"
	ErrGitFailed         = "GIT_FAILED"
	ErrFUSENotFound      = "FUSE_NOT_FOUND"
	ErrPermissionDenied  = "PERMISSION_DENIED"
	ErrInvalidConfig     = "INVALID_CONFIG"
	ErrOverlayNotMounted = "OVERLAY_NOT_MOUNTED"
	ErrOverlayLocked     = "OVERLAY_LOCKED"
)

// OverlayError represents a structured error with code
type OverlayError struct {
	Code    string
	Message string
	Cause   error
}

func (e *OverlayError) Error() string {
	if e.Cause != nil {
		return e.Code + ": " + e.Message + ": " + e.Cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *OverlayError) Unwrap() error {
	return e.Cause
}

// NewError creates a new OverlayError
func NewError(code, message string, cause error) *OverlayError {
	return &OverlayError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}
