package commands

import (
	"testing"
	"time"
)

func TestPrintListTable_Empty(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	err := printListTable(nil)
	if err != nil {
		t.Errorf("printListTable empty failed: %v", err)
	}
}

func TestPrintListTable(t *testing.T) {
	setupTestEnv(t, t.TempDir())

	infos := []listOverlayInfo{
		{Name: "test1", Mounted: true, Path: "/tmp/mnt/test1", Branch: "feature/test", Uptime: 5 * time.Minute},
		{Name: "test2", Mounted: false, Path: "/tmp/mnt/test2", Branch: "", Uptime: 0},
	}

	err := printListTable(infos)
	if err != nil {
		t.Errorf("printListTable failed: %v", err)
	}
}

func TestPrintListSimple(t *testing.T) {
	infos := []listOverlayInfo{
		{Name: "test1"},
		{Name: "test2"},
	}

	err := printListSimple(infos)
	if err != nil {
		t.Errorf("printListSimple failed: %v", err)
	}
}

func TestPrintListJSON(t *testing.T) {
	infos := []listOverlayInfo{
		{Name: "test1", Mounted: true, Path: "/tmp/mnt/test1"},
	}

	err := printListJSON(infos)
	if err != nil {
		t.Errorf("printListJSON failed: %v", err)
	}
}
