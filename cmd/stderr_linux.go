//go:build linux

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
	if err := syscall.Dup3(int(target.Fd()), 2, 0); err != nil {
		return err
	}
	os.Stderr = target
	return nil
}
