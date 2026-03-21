// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements the AgentTracker, which manages spawned agent processes
// with resource limits, timeout enforcement, and output capture.
package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// maxAgentOutputSize caps stdout/stderr capture buffers at 10 MiB to
// prevent OOM from runaway agent output.
const maxAgentOutputSize = 10 * 1024 * 1024

// limitedBuffer wraps bytes.Buffer with a size cap to prevent OOM from
// unexpectedly large output. Once the cap is reached, additional writes
// are silently discarded (no error to avoid breaking cmd.Stdout/Stderr).
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	if lb.buf.Len()+len(p) > lb.max {
		// Silently discard overflow — returning an error would cause
		// exec.Cmd to abort the process, which is not desired.
		remaining := lb.max - lb.buf.Len()
		if remaining > 0 {
			lb.buf.Write(p[:remaining])
		}
		return len(p), nil
	}
	return lb.buf.Write(p)
}

func (lb *limitedBuffer) Bytes() []byte {
	return lb.buf.Bytes()
}

// AgentStatus represents the lifecycle state of a tracked agent.
type AgentStatus int

const (
	// AgentRunning indicates the agent process is still executing.
	AgentRunning AgentStatus = iota
	// AgentExited indicates the agent process exited successfully (code 0).
	AgentExited
	// AgentFailed indicates the agent process exited with a non-zero code.
	AgentFailed
)

// String returns a human-readable label for the agent status.
func (s AgentStatus) String() string {
	switch s {
	case AgentRunning:
		return "running"
	case AgentExited:
		return "exited"
	case AgentFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// AgentInfo holds metadata about a tracked agent process.
type AgentInfo struct {
	StartedAt time.Time
	EndedAt   time.Time
	Command   string
	Args      []string
	PID       int
	Status    AgentStatus
	ExitCode  int
	Duration  time.Duration
}

// ringBuffer is a fixed-size circular buffer that stores the last N lines.
type ringBuffer struct {
	lines []string
	max   int
	count int
}

func newRingBuffer(maxSize int) *ringBuffer {
	return &ringBuffer{
		lines: make([]string, maxSize),
		max:   maxSize,
	}
}

func (rb *ringBuffer) write(line string) {
	rb.lines[rb.count%rb.max] = line
	rb.count++
}

func (rb *ringBuffer) snapshot() []string {
	if rb.count == 0 {
		return []string{}
	}
	if rb.count <= rb.max {
		out := make([]string, rb.count)
		copy(out, rb.lines[:rb.count])
		return out
	}
	// Ring has wrapped — read from oldest to newest.
	out := make([]string, rb.max)
	start := rb.count % rb.max
	copy(out, rb.lines[start:])
	copy(out[rb.max-start:], rb.lines[:start])
	return out
}

// trackedAgent holds the state for a single spawned agent.
type trackedAgent struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	stdout    *ringBuffer
	stderr    *ringBuffer
	stdoutBuf *limitedBuffer // bounded capture buffer fed to cmd.Stdout
	stderrBuf *limitedBuffer // bounded capture buffer fed to cmd.Stderr
	info      AgentInfo
}

// AgentTracker manages spawned agent processes with concurrent access safety,
// resource limits (max concurrent), timeout enforcement, and output capture.
type AgentTracker struct {
	parentCtx      context.Context
	agents         map[int]*trackedAgent
	parentCancel   context.CancelFunc
	maxProcesses   int
	timeoutSeconds int
	maxOutputLines int
	mu             sync.Mutex
}

// Default limits for agent tracking.
const (
	defaultMaxProcesses   = 5
	defaultTimeoutSeconds = 1800
	defaultMaxOutputLines = 1000
)

// NewAgentTracker creates a new AgentTracker with the given limits.
// maxProcesses <= 0 defaults to 5; timeoutSeconds <= 0 defaults to 1800.
func NewAgentTracker(maxProcesses, timeoutSeconds int) *AgentTracker {
	if maxProcesses <= 0 {
		maxProcesses = defaultMaxProcesses
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentTracker{
		agents:         make(map[int]*trackedAgent),
		maxProcesses:   maxProcesses,
		timeoutSeconds: timeoutSeconds,
		maxOutputLines: defaultMaxOutputLines,
		parentCtx:      ctx,
		parentCancel:   cancel,
	}
}

// RunningCount returns the number of currently running agents.
func (t *AgentTracker) RunningCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.runningCountLocked()
}

// runningCountLocked returns the number of running agents.
// Caller must hold t.mu.
func (t *AgentTracker) runningCountLocked() int {
	n := 0
	for _, a := range t.agents {
		if a.info.Status == AgentRunning {
			n++
		}
	}
	return n
}

// Spawn starts a new agent process and tracks it. Returns the assigned PID
// (or internal ID in test mode) and an error if limits are exceeded.
func (t *AgentTracker) Spawn(ctx context.Context, command string, args []string) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Check concurrent process limit.
	running := t.runningCountLocked()
	if running >= t.maxProcesses {
		return 0, fmt.Errorf("agent limit reached: %d/%d processes running", running, t.maxProcesses)
	}
	// Build context with timeout.
	timeout := time.Duration(t.timeoutSeconds) * time.Second
	agentCtx, cancel := context.WithTimeout(t.parentCtx, timeout)
	// Also respect caller context cancellation.
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-agentCtx.Done():
		}
	}()
	cmd := exec.CommandContext(agentCtx, command, args...)
	cmd.Env = filterEnvForAgent()
	stdoutBuf := &limitedBuffer{max: maxAgentOutputSize}
	stderrBuf := &limitedBuffer{max: maxAgentOutputSize}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	if err := cmd.Start(); err != nil {
		cancel()
		return 0, fmt.Errorf("failed to start agent %q: %w", command, err)
	}
	pid := cmd.Process.Pid
	agent := &trackedAgent{
		info: AgentInfo{
			PID:       pid,
			Command:   command,
			Args:      args,
			Status:    AgentRunning,
			StartedAt: time.Now(),
		},
		cmd:       cmd,
		cancel:    cancel,
		stdout:    newRingBuffer(t.maxOutputLines),
		stderr:    newRingBuffer(t.maxOutputLines),
		stdoutBuf: stdoutBuf,
		stderrBuf: stderrBuf,
	}
	t.agents[pid] = agent
	// Monitor the process in a background goroutine.
	go t.monitor(pid, agentCtx, cancel)
	return pid, nil
}

// monitor waits for a process to exit and updates its status.
func (t *AgentTracker) monitor(pid int, _ context.Context, cancel context.CancelFunc) {
	defer cancel()
	t.mu.Lock()
	agent, ok := t.agents[pid]
	if !ok {
		t.mu.Unlock()
		return
	}
	cmd := agent.cmd
	t.mu.Unlock()
	err := cmd.Wait()
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	agent, ok = t.agents[pid]
	if !ok {
		return
	}
	agent.info.EndedAt = now
	agent.info.Duration = now.Sub(agent.info.StartedAt)
	// Parse stdout/stderr line-by-line into ring buffers.
	t.parseOutput(agent)
	if err != nil {
		agent.info.Status = AgentFailed
		if exitErr, ok := err.(*exec.ExitError); ok {
			agent.info.ExitCode = exitErr.ExitCode()
		} else {
			agent.info.ExitCode = -1
		}
	} else {
		agent.info.Status = AgentExited
		agent.info.ExitCode = 0
	}
}

// parseOutput splits raw capture buffers into ring buffer lines.
func (t *AgentTracker) parseOutput(agent *trackedAgent) {
	for _, line := range bytes.Split(agent.stdoutBuf.Bytes(), []byte("\n")) {
		if s := string(line); s != "" {
			agent.stdout.write(s)
		}
	}
	for _, line := range bytes.Split(agent.stderrBuf.Bytes(), []byte("\n")) {
		if s := string(line); s != "" {
			agent.stderr.write(s)
		}
	}
}

// List returns info for all tracked agents, in no particular order.
func (t *AgentTracker) List() []AgentInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Update durations for running agents.
	now := time.Now()
	infos := make([]AgentInfo, 0, len(t.agents))
	for _, a := range t.agents {
		info := a.info
		if info.Status == AgentRunning {
			info.Duration = now.Sub(info.StartedAt)
		}
		infos = append(infos, info)
	}
	return infos
}

// Kill terminates a specific agent by PID.
func (t *AgentTracker) Kill(pid int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	agent, ok := t.agents[pid]
	if !ok {
		return fmt.Errorf("agent with PID %d not found", pid)
	}
	if agent.info.Status != AgentRunning {
		return nil // already exited
	}
	// cancel is a context.CancelFunc — non-blocking and safe to call under lock.
	agent.cancel()
	return nil
}

// KillAll terminates all running agents. Called on application shutdown.
func (t *AgentTracker) KillAll() {
	t.parentCancel()
}

// Output returns captured stdout and stderr lines for the given PID.
// For running agents, only previously-parsed output is returned because the
// underlying bytes.Buffer is concurrently written to by the exec package
// and reading it here without additional synchronisation would be a data race.
func (t *AgentTracker) Output(pid int) (stdout, stderr []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	agent, ok := t.agents[pid]
	if !ok {
		return nil, nil
	}
	return agent.stdout.snapshot(), agent.stderr.snapshot()
}
