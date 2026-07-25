package crashlog

import (
	"fmt"
	"os"
	"runtime/debug"
	"sync/atomic"
)

// RecoverAndReport is intended to be called inside a deferred function at
// the top of main (or any critical goroutine). If a panic occurred it
// captures a crash report, writes it to disk, and prints guidance to
// stderr. If tail is non-nil its entries override the default log tail.
//
// Usage:
//
//	defer func() { crashlog.RecoverAndReport("running root command", tail) }()
func RecoverAndReport(ctx string, tail *LogTailHandler) {
	r := recover()
	if r == nil {
		return
	}

	report := NewReport(r, debug.Stack(), ctx)
	if tail != nil {
		report.LogTail = scrubLogTail(tail.Entries())
	}

	path, err := Write(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\ngrut crashed. Failed to save crash report: %v\n", err)
		fmt.Fprintf(os.Stderr, "Panic: %s\n", ScrubPII(fmt.Sprint(r)))
		return
	}

	fmt.Fprintf(os.Stderr, "\ngrut crashed unexpectedly.\n")
	fmt.Fprintf(os.Stderr, "Crash report saved to: %s\n", ScrubPII(path))
	fmt.Fprintf(os.Stderr, "Run 'grut report' to file a GitHub issue.\n")
}

// WriteRecovery creates and writes a crash report from an already-recovered
// panic value. Unlike RecoverAndReport, it does not call recover() itself,
// so the caller retains control of recovery flow (e.g., setting exit codes).
func WriteRecovery(panicVal any, ctx string, tail *LogTailHandler) {
	report := NewReport(panicVal, debug.Stack(), ctx)
	if tail != nil {
		report.LogTail = scrubLogTail(tail.Entries())
	}

	path, err := Write(report)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\ngrut crashed. Failed to save crash report: %v\n", err)
		fmt.Fprintf(os.Stderr, "Panic: %s\n", ScrubPII(fmt.Sprint(panicVal)))
		return
	}

	fmt.Fprintf(os.Stderr, "\ngrut crashed unexpectedly.\n")
	fmt.Fprintf(os.Stderr, "Crash report saved to: %s\n", ScrubPII(path))
	fmt.Fprintf(os.Stderr, "Run 'grut report' to file a GitHub issue.\n")
}

// CaptureCmdPanic writes a crash report for a panic recovered inside a Bubble
// Tea command goroutine and records it as the last crash path. Unlike
// WriteRecovery it prints nothing to stderr: the TUI still owns the terminal
// (alternate screen, raw mode), so stray output would corrupt the display.
// It returns the crash report path, or "" if the report could not be written.
//
// Bubble Tea runs each command in its own goroutine and turns any panic there
// into a fatal ErrProgramPanic that tears the whole program down. GuardTUI only
// covers the model's Init/Update/View on the main goroutine, so a command that
// panics — e.g. on a nil field in live API data — would otherwise kill the TUI.
// Recovering in the command closure and reporting via this function keeps the
// TUI alive while preserving the stack for diagnosis. Use it from a deferred
// recover in the command:
//
//	return func() (msg tea.Msg) {
//		defer func() {
//			if r := recover(); r != nil {
//				crashlog.CaptureCmdPanic(r, "gitinfo.loadGitHubData")
//				msg = errorToast(r)
//			}
//		}()
//		...
//	}
func CaptureCmdPanic(panicVal any, ctx string) string {
	report := NewReport(panicVal, debug.Stack(), ctx)
	path, err := Write(report)
	if err != nil {
		return ""
	}
	lastCrashPath.Store(&path)
	return path
}

// lastCrashPath records the path of the most recent crash report written by
// GuardTUI. It lets the program surface the report location after Bubble Tea
// has restored the terminal, since anything GuardTUI itself prints would land
// on the (about to be torn down) alternate screen while still in raw mode.
var lastCrashPath atomic.Pointer[string]

// LastCrashPath returns the filesystem path of the most recent crash report
// captured by GuardTUI during this run, or "" if no TUI panic was captured.
func LastCrashPath() string {
	if p := lastCrashPath.Load(); p != nil {
		return *p
	}
	return ""
}

// GuardTUI is a deferred panic guard for the Bubble Tea model's Update, View,
// and Init methods. Bubble Tea catches panics inside those methods itself and
// returns only a generic ErrProgramPanic, so the top-level recover in main
// never sees the panic value and no crash report is written. GuardTUI closes
// that gap: it recovers the in-flight panic, writes a crash report with the
// preserved stack (debug.Stack in a deferred recover still points at the
// original panic site) and the recent log tail, records the report path, then
// re-panics with the original value so Bubble Tea still restores the terminal.
//
// Use it as the first statement of the guarded method:
//
//	func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
//		defer crashlog.GuardTUI("tui.Update")
//		...
//	}
func GuardTUI(ctx string) {
	r := recover()
	if r == nil {
		return
	}
	report := NewReport(r, debug.Stack(), ctx)
	if path, err := Write(report); err == nil {
		lastCrashPath.Store(&path)
	}
	panic(r)
}
