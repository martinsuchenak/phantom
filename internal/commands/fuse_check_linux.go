//go:build linux

package commands

// On Linux the FUSE device is always present in the kernel; no pre-check needed.
func checkFUSEAvailable() error { return nil }
