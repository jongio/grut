package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/jongio/grut/cmd"
	"github.com/jongio/grut/internal/crashlog"
)

func main() {
	os.Exit(run())
}

// run executes the root command with top-level panic recovery.
func run() (code int) {
	// Feed the crash tail from a concrete discard handler rather than wrapping
	// slog's built-in default handler. slog.SetDefault rewires the stdlib log
	// package to route back through the new default handler, so wrapping the
	// old default (which writes via that same log package) would create a
	// cycle that self-deadlocks the first time anything logs at INFO or above.
	// A LevelDebug handler keeps the ring buffer populated for crash reports
	// while writing nothing to the console. The TUI swaps this out for a file
	// or discard handler (honoring GRUT_LOG) before rendering; CLI subcommands
	// keep this quiet default so diagnostic logs never contaminate their output.
	baseHandler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	tail := crashlog.NewLogTailHandler(baseHandler, 50)
	slog.SetDefault(slog.New(tail))
	crashlog.SetDefaultLogTail(tail)

	defer func() {
		if r := recover(); r != nil {
			crashlog.WriteRecovery(r, "top-level", tail)
			code = 2
		}
	}()

	if err := cmd.Execute(); err != nil {
		return 1
	}
	return 0
}
