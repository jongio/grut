//go:build windows

package cmd

import (
	"os"

	"golang.org/x/sys/windows"
)

// captureOriginalStderr duplicates the process-level stderr handle so that
// error messages can still be written to the real console after redirection.
func captureOriginalStderr() *os.File {
	proc := windows.CurrentProcess()

	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, windows.Handle(os.Stderr.Fd()), proc, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return os.Stderr
	}
	return os.NewFile(uintptr(dup), "stderr")
}

// redirectStderr replaces the process-level stderr handle so that child
// processes inherit the redirected handle instead of the real console stderr.
func redirectStderr(target *os.File) error {
	if err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(target.Fd())); err != nil {
		return err
	}
	os.Stderr = target
	return nil
}
