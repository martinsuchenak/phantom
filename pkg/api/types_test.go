package api

import (
	"errors"
	"testing"
	"time"
)

func TestOverlay(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		overlay Overlay
	}{
		{
			name: "basic overlay",
			overlay: Overlay{
				Name:       "test-overlay",
				BaseDir:    "/path/to/base",
				MountPoint: "/path/to/mnt",
				UpperDir:   "/path/to/upper",
				WorkDir:    "/path/to/work",
				Branch:     "feature/test",
				Persistent: false,
				CreatedAt:  now,
			},
		},
		{
			name: "persistent overlay with PID",
			overlay: Overlay{
				Name:       "persistent-overlay",
				BaseDir:    "/path/to/base",
				MountPoint: "/path/to/mnt",
				UpperDir:   "/path/to/upper",
				Branch:     "main",
				Persistent: true,
				CreatedAt:  now,
				PID:        12345,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.overlay.Name == "" {
				t.Error("Name should not be empty")
			}
			if tt.overlay.BaseDir == "" {
				t.Error("BaseDir should not be empty")
			}
			if tt.overlay.MountPoint == "" {
				t.Error("MountPoint should not be empty")
			}
			if tt.overlay.CreatedAt.IsZero() {
				t.Error("CreatedAt should be set")
			}
		})
	}
}

func TestOverlayStatus(t *testing.T) {
	tests := []struct {
		name   string
		status OverlayStatus
	}{
		{
			name: "mounted overlay",
			status: OverlayStatus{
				Name:      "test",
				Mounted:   true,
				SizeBytes: 1024,
				Uptime:    time.Hour,
			},
		},
		{
			name: "unmounted overlay",
			status: OverlayStatus{
				Name:      "test",
				Mounted:   false,
				SizeBytes: 0,
				Uptime:    0,
			},
		},
		{
			name: "overlay with agent PID",
			status: OverlayStatus{
				Name:     "test",
				Mounted:  true,
				Uptime:   30 * time.Minute,
				AgentPID: 54321,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.status.Name == "" {
				t.Error("Name should not be empty")
			}
		})
	}
}

func TestCreateOptions(t *testing.T) {
	opts := &CreateOptions{
		Name:       "test",
		BaseDir:    "/path/to/base",
		Branch:     "feature/test",
		Persistent: true,
	}

	if opts.Name != "test" {
		t.Errorf("expected Name to be 'test', got %s", opts.Name)
	}
	if opts.BaseDir != "/path/to/base" {
		t.Errorf("expected BaseDir to be '/path/to/base', got %s", opts.BaseDir)
	}
	if !opts.Persistent {
		t.Error("expected Persistent to be true")
	}
}

func TestRunOptions(t *testing.T) {
	opts := &RunOptions{
		Agent:     "claude code",
		Task:      "implement feature",
		BaseDir:   "/path/to/base",
		Name:      "test-run",
		Timeout:   time.Hour,
		Cleanup:   true,
		PushOnEnd: false,
	}

	if opts.Agent != "claude code" {
		t.Errorf("expected Agent to be 'claude code', got %s", opts.Agent)
	}
	if opts.Task != "implement feature" {
		t.Errorf("expected Task to be 'implement feature', got %s", opts.Task)
	}
	if opts.Timeout != time.Hour {
		t.Errorf("expected Timeout to be 1 hour, got %v", opts.Timeout)
	}
}

func TestOverlayError(t *testing.T) {
	tests := []struct {
		name       string
		err        *OverlayError
		expected   string
		hasCause   bool
		errorCode  string
	}{
		{
			name:      "error without cause",
			err:       NewError(ErrNotFound, "overlay not found", nil),
			expected:  "NOT_FOUND: overlay not found",
			hasCause:  false,
			errorCode: ErrNotFound,
		},
		{
			name:       "error with cause",
			err:        NewError(ErrMountFailed, "mount failed", errors.New("permission denied")),
			expected:   "MOUNT_FAILED: mount failed: permission denied",
			hasCause:   true,
			errorCode:  ErrMountFailed,
		},
		{
			name:      "git error",
			err:       NewError(ErrGitFailed, "failed to create branch", nil),
			expected:  "GIT_FAILED: failed to create branch",
			hasCause:  false,
			errorCode: ErrGitFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Error() method
			errMsg := tt.err.Error()
			if errMsg != tt.expected {
				t.Errorf("expected error message %q, got %q", tt.expected, errMsg)
			}

			// Test code
			if tt.err.Code != tt.errorCode {
				t.Errorf("expected error code %q, got %q", tt.errorCode, tt.err.Code)
			}

			// Test Unwrap()
			unwrapped := tt.err.Unwrap()
			if tt.hasCause && unwrapped == nil {
				t.Error("expected cause to be present")
			}
			if !tt.hasCause && unwrapped != nil {
				t.Error("expected no cause")
			}
		})
	}
}

func TestErrorCodes(t *testing.T) {
	codes := []string{
		ErrMountFailed,
		ErrUnmountFailed,
		ErrNotFound,
		ErrAlreadyExists,
		ErrGitFailed,
		ErrFUSENotFound,
		ErrPermissionDenied,
		ErrInvalidConfig,
		ErrOverlayNotMounted,
		ErrRemoteUnavailable,
		ErrAuthFailed,
		ErrSyncFailed,
		ErrFileTooLarge,
	}

	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			if code == "" {
				t.Error("error code should not be empty")
			}
		})
	}
}

func TestOverlayRemoteFields(t *testing.T) {
	o := Overlay{
		Name:            "agent1",
		Remote:          true,
		RemoteNode:      "node-a",
		RemoteRepo:      "myapp",
		RemoteMountPath: "/home/user/.phantom/remote-mounts/node-a/myapp",
	}
	if !o.Remote {
		t.Error("expected Remote to be true")
	}
	if o.RemoteNode != "node-a" {
		t.Errorf("expected RemoteNode 'node-a', got %q", o.RemoteNode)
	}
	if o.RemoteRepo != "myapp" {
		t.Errorf("expected RemoteRepo 'myapp', got %q", o.RemoteRepo)
	}
}

func TestPeer(t *testing.T) {
	p := Peer{
		ID:       "node-a",
		GRPCAddr: "192.168.1.10:50051",
		Repos:    []string{"myapp", "other"},
	}
	if len(p.Repos) != 2 {
		t.Errorf("expected 2 repos, got %d", len(p.Repos))
	}
}
