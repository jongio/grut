//go:build unix && !linux && !darwin

package cmd

import (
	"os"
	"syscall"
)

// captureOriginalStderr duplicates file descriptor 2 so that error messages
// can still be written to the real console after redirection.
func captureOriginalStderr() *os.File {
	dupFD, err := syscall.Dup(2)
	if err != nil {
		return os.Stderr
	}
	return os.NewFile(uintptr(dupFD), "/dev/stderr")
}

// redirectStderr replaces file descriptor 2 so that child processes
// inherit the redirected fd instead of the real console stderr.
func redirectStderr(target *os.File) error {
	//nolint:staticcheck // syscall.Dup2 is the portable fallback for non-Linux Unix.
	if err := syscall.Dup2(int(target.Fd()), 2); err != nil {
		return err
	}
	os.Stderr = target
	return nil
}
