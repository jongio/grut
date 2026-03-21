package mcp

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleepCmd returns a platform-appropriate command that sleeps for the given
// number of seconds.
func sleepCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		// PowerShell's Start-Sleep accepts fractional seconds.
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Milliseconds 5000"}
	}
	return "sleep", []string{"5"}
}

// echoCmd returns a platform-appropriate command that prints output to stdout.
func echoCmd(text string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{
			"-NoProfile", "-NonInteractive", "-Command",
			"Write-Output '" + text + "'",
		}
	}
	return "echo", []string{text}
}

// failCmd returns a command that exits with a non-zero exit code.
func failCmd() (string, []string) {
	if runtime.GOOS == "windows" {
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", "exit 42"}
	}
	return "sh", []string{"-c", "exit 42"}
}

func TestNewAgentTracker_Defaults(t *testing.T) {
	tracker := NewAgentTracker(0, 0)
	assert.Equal(t, 5, tracker.maxProcesses)
	assert.Equal(t, 1800, tracker.timeoutSeconds)
	assert.NotNil(t, tracker.agents)
}

func TestNewAgentTracker_CustomLimits(t *testing.T) {
	tracker := NewAgentTracker(10, 600)
	assert.Equal(t, 10, tracker.maxProcesses)
	assert.Equal(t, 600, tracker.timeoutSeconds)
}

func TestSpawn_TracksProcess(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	cmd, args := sleepCmd()
	pid, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)
	assert.Greater(t, pid, 0)

	agents := tracker.List()
	require.Len(t, agents, 1)
	assert.Equal(t, pid, agents[0].PID)
	assert.Equal(t, AgentRunning, agents[0].Status)
	assert.Equal(t, cmd, agents[0].Command)
}

func TestSpawn_MaxProcessesLimit(t *testing.T) {
	tracker := NewAgentTracker(2, 30)
	defer tracker.KillAll()

	cmd, args := sleepCmd()

	// Spawn up to the limit.
	_, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)
	_, err = tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	// Third should fail.
	_, err = tracker.Spawn(context.Background(), cmd, args)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent limit reached")
}

func TestKill_SpecificAgent(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	cmd, args := sleepCmd()
	pid, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	err = tracker.Kill(pid)
	require.NoError(t, err)

	// Wait for the process to actually exit.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		agents := tracker.List()
		if len(agents) > 0 && agents[0].Status != AgentRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	agents := tracker.List()
	require.Len(t, agents, 1)
	assert.NotEqual(t, AgentRunning, agents[0].Status)
}

func TestKill_NotFound(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	err := tracker.Kill(99999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestKillAll(t *testing.T) {
	tracker := NewAgentTracker(5, 30)

	cmd, args := sleepCmd()
	_, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)
	_, err = tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	tracker.KillAll()

	// Wait for processes to exit.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, a := range tracker.List() {
			if a.Status == AgentRunning {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	for _, a := range tracker.List() {
		assert.NotEqual(t, AgentRunning, a.Status, "all agents should be stopped after KillAll")
	}
}

func TestOutputCapture(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	cmd, args := echoCmd("hello agent world")
	pid, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	// Wait for the process to exit.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		agents := tracker.List()
		if len(agents) > 0 && agents[0].Status != AgentRunning {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	stdout, _ := tracker.Output(pid)
	require.NotEmpty(t, stdout, "should have captured stdout output")
	assert.Contains(t, stdout[0], "hello agent world")
}

func TestOutputCapture_NotFound(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	stdout, stderr := tracker.Output(99999)
	assert.Nil(t, stdout)
	assert.Nil(t, stderr)
}

func TestTimeout_KillsAgentAutomatically(t *testing.T) {
	// Use a very short timeout (1 second) with a longer sleep.
	tracker := NewAgentTracker(5, 1)
	defer tracker.KillAll()

	cmd, args := sleepCmd()
	pid, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	// Wait for the timeout to kill the process.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		agents := tracker.List()
		for _, a := range agents {
			if a.PID == pid && a.Status != AgentRunning {
				// Process was killed by timeout.
				assert.NotEqual(t, AgentRunning, a.Status)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("agent was not killed by timeout within expected window")
}

func TestExitCode_NonZero(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	cmd, args := failCmd()
	pid, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	// Wait for the process to exit.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		agents := tracker.List()
		for _, a := range agents {
			if a.PID == pid && a.Status != AgentRunning {
				assert.Equal(t, AgentFailed, a.Status)
				assert.Equal(t, 42, a.ExitCode)
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("agent did not exit within expected window")
}

func TestRunningCount(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	assert.Equal(t, 0, tracker.RunningCount())

	cmd, args := sleepCmd()
	_, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)
	assert.Equal(t, 1, tracker.RunningCount())

	_, err = tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)
	assert.Equal(t, 2, tracker.RunningCount())
}

func TestRingBuffer(t *testing.T) {
	rb := newRingBuffer(3)

	// Empty buffer.
	assert.Equal(t, []string{}, rb.snapshot())

	// Add fewer than max.
	rb.write("a")
	rb.write("b")
	assert.Equal(t, []string{"a", "b"}, rb.snapshot())

	// Fill to max.
	rb.write("c")
	assert.Equal(t, []string{"a", "b", "c"}, rb.snapshot())

	// Overflow — oldest should be dropped.
	rb.write("d")
	assert.Equal(t, []string{"b", "c", "d"}, rb.snapshot())

	rb.write("e")
	assert.Equal(t, []string{"c", "d", "e"}, rb.snapshot())
}

func TestAgentStatus_String(t *testing.T) {
	assert.Equal(t, "running", AgentRunning.String())
	assert.Equal(t, "exited", AgentExited.String())
	assert.Equal(t, "failed", AgentFailed.String())
	assert.Equal(t, "unknown", AgentStatus(99).String())
}

func TestSpawn_InvalidCommand(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	_, err := tracker.Spawn(context.Background(), "nonexistent-command-xyz-12345", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start agent")
}

func TestList_UpdatesDurationForRunning(t *testing.T) {
	tracker := NewAgentTracker(5, 30)
	defer tracker.KillAll()

	cmd, args := sleepCmd()
	_, err := tracker.Spawn(context.Background(), cmd, args)
	require.NoError(t, err)

	// First list call.
	agents1 := tracker.List()
	require.Len(t, agents1, 1)
	dur1 := agents1[0].Duration

	time.Sleep(100 * time.Millisecond)

	// Second list call should show increased duration.
	agents2 := tracker.List()
	dur2 := agents2[0].Duration
	assert.Greater(t, dur2, dur1, "duration should increase for running agents")
}
