package overlay

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Mock exec command helper
// based on https://github.com/golang/go/blob/master/src/os/exec/exec_test.go#L31
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	return cmd
}

// TestHelperProcess isn't a real test. It's used as a helper process
// for TestManager logic that calls "exec.Command".
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	// args[0] is program name, args[1] is -test.run, args[2] is --, args[3] is command
	args := os.Args
	for len(args) > 0 {
		if args[0] == "--" {
			args = args[1:]
			break
		}
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "No command\n")
		os.Exit(2)
	}

	cmd, cmdArgs := args[0], args[1:]

	// Check for unionfs-fuse (might be full path)
	if cmd == "unionfs-fuse" || filepath.Base(cmd) == "unionfs-fuse" {
		cmd = "unionfs-fuse" // normalize for switch
	}

	switch cmd {
	case "unionfs-fuse":
		// Check arguments
		// Expected: -o [options] -o [options] dirs mountpoint
		if len(cmdArgs) < 4 {
			fmt.Fprintf(os.Stderr, "Not enough arguments for unionfs-fuse\n")
			os.Exit(1)
		}
		// Validate mount point exists
		mountPoint := cmdArgs[len(cmdArgs)-1]
		if _, err := os.Stat(mountPoint); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Mount point does not exist: %s\n", mountPoint)
			os.Exit(1)
		}
		os.Exit(0)

	case "umount":
		if len(cmdArgs) == 0 {
			os.Exit(1)
		}
		os.Exit(0)

	case "mount":
		mountedPaths := os.Getenv("GO_TEST_MOUNTED_PATHS")
		if mountedPaths != "" {
			for _, path := range filepath.SplitList(mountedPaths) {
				fmt.Printf("map %s on %s (fuse)\n", path, path)
			}
		}
		os.Exit(0)

	case "ps":
		// Mock ps command — return empty output (no unionfs process found)
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", cmd)
		os.Exit(2)
	}
}

func TestDarwinManager_Integration(t *testing.T) {
	// Only run on Darwin
	// (Actually we can run this anywhere since we mocked exec,
	//  BUT DarwinManager struct fields might be OS specific if guarded by build tags.
	//  darwin.go uses //go:build darwin, so this test file also needs to be darwin only
	//  OR we need access to DarwinManager struct.)
	// darwin.go has //go:build darwin
}
