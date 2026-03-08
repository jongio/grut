// Package filetree provides a filesystem watcher for the grut file explorer.
// It uses fsnotify for event-based watching with a debounce window, and
// falls back to polling when fsnotify is unavailable or fails.
package filetree

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// RefreshMsg tells the filetree to reload its directory listing.
type RefreshMsg struct{}

// defaultDebounce is the default debounce interval for filesystem events.
const defaultDebounce = 100 * time.Millisecond

// defaultPollInterval is the fallback polling interval when fsnotify fails.
const defaultPollInterval = 5 * time.Second

// watcher watches the filesystem for changes and emits RefreshMsg via a
// Bubble Tea command. It debounces rapid events and falls back to polling
// when fsnotify cannot be used.
type watcher struct {
	mu       sync.Mutex
	fsw      *fsnotify.Watcher
	dirs     map[string]bool // currently watched directories
	debounce time.Duration
	polling  bool // true if using fallback polling
	pollInt  time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
	stopOnce sync.Once // ensures stop() cleanup runs exactly once (F29)
}

// newWatcher creates a filesystem watcher. If fsnotify initialisation
// fails, it silently falls back to polling at pollInterval.
func newWatcher(debounce, pollInterval time.Duration) *watcher {
	if debounce <= 0 {
		debounce = defaultDebounce
	}
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}

	w := &watcher{
		dirs:     make(map[string]bool),
		debounce: debounce,
		pollInt:  pollInterval,
		done:     make(chan struct{}),
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		// fsnotify unavailable — will use polling fallback.
		w.polling = true
		return w
	}
	w.fsw = fsw
	return w
}

// start begins watching. It returns a tea.Cmd that will produce RefreshMsg
// when filesystem changes are detected.
func (w *watcher) start(ctx context.Context) tea.Cmd {
	w.mu.Lock()
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{}) // fresh channel per start (F28)
	w.mu.Unlock()

	// F29: Spawn a shutdown goroutine that invokes stop() when the context
	// is cancelled. This ensures resources are freed even when the
	// application shuts down via context cancellation alone. The stop()
	// method is idempotent (sync.Once), so calling it from here AND from
	// an explicit Close() is safe.
	go func() {
		<-ctx.Done()
		w.stop()
	}()

	if w.polling {
		return w.startPolling(ctx)
	}
	return w.startFSNotify(ctx)
}

// startFSNotify runs the fsnotify event loop with debouncing.
func (w *watcher) startFSNotify(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		defer close(w.done)

		var timer *time.Timer
		var timerC <-chan time.Time

		for {
			select {
			case <-ctx.Done():
				if timer != nil {
					timer.Stop()
				}
				return nil

			case event, ok := <-w.fsw.Events:
				if !ok {
					return nil
				}
				// Only react to meaningful operations.
				if event.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Write) == 0 {
					continue
				}
				// Reset debounce timer.
				if timer == nil {
					timer = time.NewTimer(w.debounce)
					timerC = timer.C
				} else {
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(w.debounce)
					timerC = timer.C
				}

			case fsErr, ok := <-w.fsw.Errors:
				if !ok {
					return nil
				}
				slog.Warn("fsnotify: watcher error", "error", fsErr)

			case <-timerC:
				return RefreshMsg{}
			}
		}
	}
}

// startPolling runs a simple polling loop.
func (w *watcher) startPolling(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		defer close(w.done)

		ticker := time.NewTicker(w.pollInt)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				return RefreshMsg{}
			}
		}
	}
}

// addDir adds a directory to the watch list. Safe to call multiple times
// for the same directory.
func (w *watcher) addDir(dir string) {
	// Skip .git directories — git commands constantly modify files under
	// .git/ which would cause an infinite refresh loop.
	if isGitInternalDir(dir) {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dirs[dir] {
		return
	}
	w.dirs[dir] = true

	if w.fsw != nil {
		if err := w.fsw.Add(dir); err != nil {
			slog.Warn("fsnotify: failed to add directory", "dir", dir, "error", err)
		}
	}
}

// removeDir removes a directory from the watch list.
func (w *watcher) removeDir(dir string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.dirs[dir] {
		return
	}
	delete(w.dirs, dir)

	if w.fsw != nil {
		if err := w.fsw.Remove(dir); err != nil {
			slog.Warn("fsnotify: failed to remove directory", "dir", dir, "error", err)
		}
	}
}

// stop shuts down the watcher and releases resources. It is safe to call
// multiple times — only the first invocation performs cleanup (F29).
func (w *watcher) stop() {
	w.stopOnce.Do(func() {
		w.mu.Lock()
		cancelFn := w.cancel
		w.mu.Unlock()

		if cancelFn != nil {
			cancelFn()
		}
		if w.fsw != nil {
			_ = w.fsw.Close()
		}
	})
}

// isPolling returns true if the watcher is using the polling fallback.
func (w *watcher) isPolling() bool {
	return w.polling
}

// isGitInternalDir returns true if dir is the .git directory itself or a
// subdirectory within it. This prevents the watcher from reacting to
// changes caused by git commands (e.g. writing lock files, updating refs).
func isGitInternalDir(dir string) bool {
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		if part == ".git" {
			return true
		}
	}
	return false
}
