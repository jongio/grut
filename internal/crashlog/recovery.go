package crashlog

import (
	"fmt"
	"os"
	"runtime/debug"
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
