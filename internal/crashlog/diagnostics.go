package crashlog

import (
	"os"
	"runtime"
	"strconv"

	"github.com/jongio/grut/internal/config"
)

// CollectDiagnostics gathers safe, non-PII system information that is
// useful for triaging crash reports.
func CollectDiagnostics() map[string]string {
	return map[string]string{
		"go_version":    runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"num_cpu":       strconv.Itoa(runtime.NumCPU()),
		"num_goroutine": strconv.Itoa(runtime.NumGoroutine()),
		"terminal":      os.Getenv("TERM_PROGRAM"),
		"version":       config.AppVersion,
		"compiler":      runtime.Compiler,
	}
}
