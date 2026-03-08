package git

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueueExec(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("boom")

	tests := []struct {
		name         string
		ctx          func() context.Context
		fn           func(ran *atomic.Int32) func() error
		wantErr      error
		wantErrText  string
		wantRanCount int32
	}{
		{
			name: "runs function and returns nil",
			ctx:  context.Background,
			fn: func(ran *atomic.Int32) func() error {
				return func() error {
					ran.Add(1)
					return nil
				}
			},
			wantRanCount: 1,
		},
		{
			name: "runs function and propagates error",
			ctx:  context.Background,
			fn: func(ran *atomic.Int32) func() error {
				return func() error {
					ran.Add(1)
					return expectedErr
				}
			},
			wantErr:      expectedErr,
			wantRanCount: 1,
		},
		{
			name: "cancelled context before call returns context error without running function",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			fn: func(ran *atomic.Int32) func() error {
				return func() error {
					ran.Add(1)
					return nil
				}
			},
			wantErr:      context.Canceled,
			wantRanCount: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := &OpQueue{}
			var ran atomic.Int32
			ctx := tt.ctx()
			require.NotNil(t, ctx)

			err := queue.Exec(ctx, tt.fn(&ran))

			assert.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantRanCount, ran.Load())
		})
	}
}

func TestQueueExecSerializesOperations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "concurrent exec calls run sequentially"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := &OpQueue{}
			start := make(chan struct{})
			var wg sync.WaitGroup
			var current atomic.Int32
			var maxConcurrent atomic.Int32
			var runs atomic.Int32

			worker := func() {
				defer wg.Done()
				<-start
				err := queue.Exec(context.Background(), func() error {
					runs.Add(1)
					active := current.Add(1)
					for {
						seen := maxConcurrent.Load()
						if active <= seen || maxConcurrent.CompareAndSwap(seen, active) {
							break
						}
					}
					defer current.Add(-1)
					time.Sleep(20 * time.Millisecond)
					return nil
				})
				assert.NoError(t, err)
			}

			wg.Add(2)
			go worker()
			go worker()
			close(start)
			wg.Wait()

			assert.Equal(t, int32(2), runs.Load())
			assert.Equal(t, int32(1), maxConcurrent.Load())
		})
	}
}

func TestQueueExecContextCancelledAfterLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "context is checked again after lock is acquired"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := &OpQueue{}
			release := make(chan struct{})
			firstEntered := make(chan struct{})
			firstDone := make(chan error, 1)

			go func() {
				firstDone <- queue.Exec(context.Background(), func() error {
					close(firstEntered)
					<-release
					return nil
				})
			}()

			<-firstEntered

			ctx, cancel := context.WithCancel(context.Background())
			require.NotNil(t, ctx)
			secondDone := make(chan error, 1)
			started := make(chan struct{})
			var ran atomic.Bool

			go func() {
				close(started)
				secondDone <- queue.Exec(ctx, func() error {
					ran.Store(true)
					return nil
				})
			}()

			<-started
			time.Sleep(10 * time.Millisecond)
			cancel()
			close(release)

			assert.NoError(t, <-firstDone)
			assert.ErrorIs(t, <-secondDone, context.Canceled)
			assert.False(t, ran.Load())
		})
	}
}

func TestQueueExecPanicsAreNotRecovered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "panic escapes exec"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			queue := &OpQueue{}

			assert.PanicsWithValue(t, "panic from fn", func() {
				_ = queue.Exec(context.Background(), func() error {
					panic("panic from fn")
				})
			})
		})
	}
}
