package notify

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToastBurstAdditionsAreBoundedAndUseOneTimer(t *testing.T) {
	for _, count := range []int{10, 100, 1000} {
		t.Run(fmt.Sprintf("%d", count), func(t *testing.T) {
			m := NewManager()
			timerCommands := 0

			for i := range count {
				if cmd := m.AddToast(fmt.Sprintf("toast-%d", i), Info); cmd != nil {
					timerCommands++
				}
			}

			assert.Equal(t, 1, timerCommands)
			assert.Equal(t, maxVisibleToasts, m.ToastCount())
			for i := range maxVisibleToasts {
				want := fmt.Sprintf("toast-%d", count-maxVisibleToasts+i)
				assert.Equal(t, want, m.toastMessage(i))
			}
		})
	}
}

func TestToastCapEvictsOldestAndPreservesSeverityOrder(t *testing.T) {
	m := NewManager()
	levels := []Level{Info, Warn, Error, Success, Info, Warn}

	for i, level := range levels {
		m.AddToast(fmt.Sprintf("toast-%d", i), level)
	}

	require.Equal(t, maxVisibleToasts, m.ToastCount())
	assert.Equal(t, "toast-1", m.toastMessage(0))
	assert.Equal(t, "toast-5", m.toastMessage(maxVisibleToasts-1))

	var retainedLevels []Level
	m.mu.RLock()
	for element := m.toasts.order.Front(); element != nil; element = element.Next() {
		retainedLevels = append(retainedLevels, element.Value.(*toast).notification.Level)
	}
	m.mu.RUnlock()
	assert.Equal(t, levels[1:], retainedLevels)

	view := m.View(80)
	assert.NotContains(t, view, "toast-0")
	for i := 1; i < len(levels); i++ {
		assert.Contains(t, view, fmt.Sprintf("toast-%d", i))
	}
}

func TestToastExpiryUsesNearestDeadlineAndPreservesOrder(t *testing.T) {
	base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	now := base
	m := NewManager()
	m.now = func() time.Time { return now }

	m.AddToastWithDuration("first", Info, 30*time.Second)
	m.AddToastWithDuration("second", Warn, 10*time.Second)
	m.AddToastWithDuration("third", Error, 20*time.Second)

	generation, deadline := toastTimerState(m)
	assert.Equal(t, base.Add(10*time.Second), deadline)
	now = deadline
	next := m.Update(ToastExpiredMsg{ID: -generation})
	require.NotNil(t, next)
	assert.Equal(t, []int64{0, 2}, m.toastIDs())

	generation, deadline = toastTimerState(m)
	assert.Equal(t, base.Add(20*time.Second), deadline)
	now = deadline
	next = m.Update(ToastExpiredMsg{ID: -generation})
	require.NotNil(t, next)
	assert.Equal(t, []int64{0}, m.toastIDs())

	generation, deadline = toastTimerState(m)
	assert.Equal(t, base.Add(30*time.Second), deadline)
	now = deadline
	assert.Nil(t, m.Update(ToastExpiredMsg{ID: -generation}))
	assert.Empty(t, m.toastIDs())
}

func TestDelayedToastTimerMessageExpiresEveryPastDeadline(t *testing.T) {
	base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	now := base
	m := NewManager()
	m.now = func() time.Time { return now }

	m.AddToastWithDuration("first", Info, 10*time.Second)
	m.AddToastWithDuration("second", Warn, 20*time.Second)
	generation, _ := toastTimerState(m)

	now = base.Add(25 * time.Second)
	assert.Nil(t, m.Update(ToastExpiredMsg{ID: -generation}))
	assert.Empty(t, m.toastIDs())
}

func TestToastNonPositiveDurationExpiresOnFirstTimer(t *testing.T) {
	for _, duration := range []time.Duration{0, -time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
			m := NewManager()
			m.now = func() time.Time { return base }

			cmd := m.AddToastWithDuration("immediate", Info, duration)
			require.NotNil(t, cmd)
			_, deadline := toastTimerState(m)
			assert.False(t, deadline.After(base))

			msg, ok := cmd().(ToastExpiredMsg)
			require.True(t, ok)
			assert.Negative(t, msg.ID)
			assert.Nil(t, m.Update(msg))
			assert.Empty(t, m.toastIDs())
		})
	}
}

func TestToastTimerResetReusesWaitingCommand(t *testing.T) {
	m := NewManager()
	cmd := m.AddToastWithDuration("later", Info, time.Hour)
	require.NotNil(t, cmd)
	timer := m.toastTimer

	result := make(chan ToastExpiredMsg, 1)
	go func() {
		result <- cmd().(ToastExpiredMsg)
	}()

	assert.Nil(t, m.AddToastWithDuration("now", Warn, 0))
	assert.Same(t, timer, m.toastTimer)

	msg := <-result
	assert.Negative(t, msg.ID)
	assert.NotNil(t, m.Update(msg))
	assert.Equal(t, []int64{0}, m.toastIDs())
}

func TestStaleToastTimerMessageDoesNotExpireOrRescheduleCurrentToast(t *testing.T) {
	base := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	m := NewManager()
	m.now = func() time.Time { return base }

	m.AddToastWithDuration("later", Info, 30*time.Second)
	staleGeneration, _ := toastTimerState(m)
	m.AddToastWithDuration("sooner", Warn, 10*time.Second)
	currentGeneration, currentDeadline := toastTimerState(m)
	require.NotEqual(t, staleGeneration, currentGeneration)

	assert.Nil(t, m.Update(ToastExpiredMsg{ID: -staleGeneration}))
	assert.Equal(t, 2, m.ToastCount())

	afterGeneration, afterDeadline := toastTimerState(m)
	assert.Equal(t, currentGeneration, afterGeneration)
	assert.Equal(t, currentDeadline, afterDeadline)

	assert.Nil(t, m.Update(ToastExpiredMsg{ID: -currentGeneration}))
	assert.Equal(t, 2, m.ToastCount())
}

func TestToastBurstOperationsAreRaceSafe(t *testing.T) {
	m := NewManager()
	timerCmd := m.AddToastWithDuration("race-timer", Info, time.Hour)
	require.NotNil(t, timerCmd)
	timerResult := make(chan ToastExpiredMsg, 1)
	go func() {
		timerResult <- timerCmd().(ToastExpiredMsg)
	}()
	assert.Nil(t, m.AddToastWithDuration("race-trigger", Warn, 0))

	const workers = 8
	const additionsPerWorker = 250

	var wg sync.WaitGroup
	wg.Add(workers + 2)
	for worker := range workers {
		go func(worker int) {
			defer wg.Done()
			for i := range additionsPerWorker {
				m.AddToast(fmt.Sprintf("worker-%d-%d", worker, i), Level(i%4))
			}
		}(worker)
	}
	go func() {
		defer wg.Done()
		for range workers * additionsPerWorker {
			_ = m.View(80)
			_ = m.ToastCount()
		}
	}()
	go func() {
		defer wg.Done()
		for i := range workers * additionsPerWorker {
			m.Update(ToastExpiredMsg{ID: int64(i)})
		}
	}()
	wg.Wait()
	m.Update(<-timerResult)

	assert.LessOrEqual(t, m.ToastCount(), maxVisibleToasts)
}

func toastTimerState(m *Manager) (int64, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.timerGen, m.timerDeadline
}
