package commands

import (
	"testing"

	"github.com/martinsuchenak/phantom/pkg/api"
)

func TestValidatePushOverlayMissingNode(t *testing.T) {
	ovl := &api.Overlay{Name: "local-agent"}
	err := ValidatePushOverlay(ovl)
	if err == nil {
		t.Error("expected error for missing remote node")
	}
}

func TestValidatePushOverlayMissingRepo(t *testing.T) {
	ovl := &api.Overlay{Name: "agent", RemoteNode: "node-a"}
	err := ValidatePushOverlay(ovl)
	if err == nil {
		t.Error("expected error for missing remote repo")
	}
}

func TestValidatePushOverlayValid(t *testing.T) {
	ovl := &api.Overlay{
		Name:       "remote-agent",
		Remote:     true,
		RemoteNode: "node-a",
		RemoteRepo: "myapp",
	}
	err := ValidatePushOverlay(ovl)
	if err != nil {
		t.Errorf("expected no error for valid remote overlay, got %v", err)
	}
}
