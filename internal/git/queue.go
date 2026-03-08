package git

import (
	"context"
	"sync"
)

// OpQueue serializes index-mutating operations to prevent git lock contention.
// Read-only operations bypass the queue entirely.
type OpQueue struct {
	mu sync.Mutex
}

// Exec runs fn while holding the queue lock. Only one index-mutating
// operation can run at a time per repository. The context is checked
// before acquiring the lock; if the context is already cancelled the
// operation is skipped.
func (q *OpQueue) Exec(ctx context.Context, fn func() error) error {
	// Fast check: bail if context is already cancelled.
	if err := ctx.Err(); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Re-check after acquiring the lock.
	if err := ctx.Err(); err != nil {
		return err
	}

	return fn()
}
