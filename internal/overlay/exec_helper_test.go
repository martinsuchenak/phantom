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
		// Simulate success (run in foreground indefinitely to simulate a deacon/process?)
		// But in our case, the manager runs it and expects it to start.
		// If we exit 0 immediately, it's like it ran successfully.
		// However, real unionfs-fuse might fork.
		// Our Darwin code simply calls cmd.Start().
		// If we exit immediately, cmd.Wait() will return nil.
		// We'll simulate a running process by sleeping a bit if needed, but for Start() verification
		// we just need it to start.

		// For the PID test, we might want to stay alive.
		// But the Manager.Mount code calls cmd.Start(), then releases the process.
		// We can just exit 0.
		os.Exit(0)

	case "umount":
		if len(cmdArgs) == 0 {
			os.Exit(1)
		}
		// If args contains -f, it's a force unmount
		// We can just exit 0 to simulate success
		os.Exit(0)

	case "mount":
		// Print fake mount output
		// We need to output lines that contain the mount points we're looking for
		// Ideally we can control this via env var or just output everything we know about
		// For now, let's output a generic list plus any "test-ovl" paths if we can guess them
		// But IsMounted looks for specific overlay.MountPoint.
		// A simple way is to mock "everything is mounted" or control it via env var GO_TEST_MOUNTED_PATHS
		mountedPaths := os.Getenv("GO_TEST_MOUNTED_PATHS")
		if mountedPaths != "" {
			for _, path := range filepath.SplitList(mountedPaths) {
				fmt.Printf("map %s on %s (fuse)\n", path, path)
			}
		}
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
