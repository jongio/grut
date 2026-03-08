package main

import (
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
	tail := crashlog.NewLogTailHandler(slog.Default().Handler(), 50)
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
