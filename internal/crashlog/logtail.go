package crashlog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
)

// ringBuffer is the shared state for a ring buffer of log entries.
// All LogTailHandler instances created via WithAttrs/WithGroup share
// a pointer to the same ringBuffer, ensuring a single mutex guards
// concurrent access.
type ringBuffer struct {
	mu      sync.Mutex
	entries []string
	size    int
	pos     int
	full    bool
}

// write stores entry at the current position and advances the cursor.
func (rb *ringBuffer) write(entry string) {
	rb.mu.Lock()
	rb.entries[rb.pos] = entry
	rb.pos++
	if rb.pos >= rb.size {
		rb.pos = 0
		rb.full = true
	}
	rb.mu.Unlock()
}

// snapshot returns the buffered entries in chronological order
// (oldest first).
func (rb *ringBuffer) snapshot() []string {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.full {
		out := make([]string, rb.pos)
		copy(out, rb.entries[:rb.pos])
		return out
	}

	out := make([]string, rb.size)
	copy(out, rb.entries[rb.pos:])
	copy(out[rb.size-rb.pos:], rb.entries[:rb.pos])
	return out
}

// LogTailHandler is a slog.Handler that keeps a ring buffer of recent log
// entries while forwarding all records to an inner handler. The buffered
// entries can be retrieved after a crash to include in the report.
type LogTailHandler struct {
	inner slog.Handler
	buf   *ringBuffer
}

// Ensure LogTailHandler implements slog.Handler at compile time.
var _ slog.Handler = (*LogTailHandler)(nil)

// defaultTailSize is used when the caller passes a non-positive size.
const defaultTailSize = 50

// NewLogTailHandler creates a handler that buffers the most recent size log
// records in a ring buffer and delegates to inner for actual output.
func NewLogTailHandler(inner slog.Handler, size int) *LogTailHandler {
	if size <= 0 {
		size = defaultTailSize
	}
	return &LogTailHandler{
		inner: inner,
		buf: &ringBuffer{
			entries: make([]string, size),
			size:    size,
		},
	}
}

// Enabled reports whether the inner handler is enabled for the given level.
func (h *LogTailHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle formats the record, stores it in the ring buffer, then delegates
// to the inner handler.
func (h *LogTailHandler) Handle(ctx context.Context, r slog.Record) error {
	h.buf.write(formatRecord(r))
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new LogTailHandler whose inner handler has the
// additional attributes. The ring buffer is shared so all derived
// handlers feed the same tail.
func (h *LogTailHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &LogTailHandler{
		inner: h.inner.WithAttrs(attrs),
		buf:   h.buf,
	}
}

// WithGroup returns a new LogTailHandler whose inner handler uses the
// named group. The ring buffer is shared so all derived handlers feed
// the same tail.
func (h *LogTailHandler) WithGroup(name string) slog.Handler {
	return &LogTailHandler{
		inner: h.inner.WithGroup(name),
		buf:   h.buf,
	}
}

// Entries returns the buffered log entries in chronological order
// (oldest first). It is safe to call from any goroutine.
func (h *LogTailHandler) Entries() []string {
	return h.buf.snapshot()
}

// defaultTail is the package-level LogTailHandler set by SetDefaultLogTail.
// Access is synchronised via atomic.Pointer to avoid races between
// main-goroutine setup and panic-recovery reads.
var defaultTail atomic.Pointer[LogTailHandler]

// SetDefaultLogTail stores h as the package-level log tail handler so
// that DefaultLogTail can retrieve buffered entries without an explicit
// reference.
func SetDefaultLogTail(h *LogTailHandler) {
	defaultTail.Store(h)
}

// DefaultLogTail returns the entries from the package-level log tail
// handler. If no handler has been set, it returns nil.
func DefaultLogTail() []string {
	t := defaultTail.Load()
	if t == nil {
		return nil
	}
	return t.Entries()
}

// formatRecord produces a single-line representation of a slog.Record:
//
//	[LEVEL] message key=val key=val ...
func formatRecord(r slog.Record) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s", r.Level.String(), r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%s", a.Key, a.Value.String())
		return true
	})
	return b.String()
}
