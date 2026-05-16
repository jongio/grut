//go:build windows

package runtime

import (
	"log/slog"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHandles maps running exec.Cmd pointers to the Windows Job Object handle
// that contains their process tree. The handle is stored after process start
// (postStartProcGroup) and removed on kill (killProcGroup).
var jobHandles sync.Map // map[*exec.Cmd]windows.Handle

// setProcGroup is a no-op on Windows; process-group containment is handled
// via Job Objects in postStartProcGroup after the process has started.
func setProcGroup(_ *exec.Cmd) {}

// postStartProcGroup creates a Windows Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, then assigns the newly started
// process to it. When the Job Object handle is closed (in killProcGroup),
// Windows terminates every process in the job — the direct child and all
// its descendants — preventing orphaned process trees (CWE-269).
//
// Must be called after cmd.Start() succeeds.
func postStartProcGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		slog.Warn("mcp runtime: CreateJobObject failed, child processes may orphan",
			"pid", cmd.Process.Pid, "error", err)
		return
	}

	// Configure the job to kill all contained processes when the last
	// handle is closed.
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		slog.Warn("mcp runtime: SetInformationJobObject failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(job)
		return
	}

	// Open the process handle with ASSIGN rights and assign to the job.
	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		slog.Warn("mcp runtime: OpenProcess failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(job)
		return
	}

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		slog.Warn("mcp runtime: AssignProcessToJobObject failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(proc)
		_ = windows.CloseHandle(job)
		return
	}

	_ = windows.CloseHandle(proc)
	jobHandles.Store(cmd, job)

	slog.Debug("mcp runtime: process assigned to job object",
		"pid", cmd.Process.Pid)
}

// killProcGroup closes the Windows Job Object handle for the given command,
// which causes Windows to terminate every process in the job tree.
func killProcGroup(cmd *exec.Cmd) {
	v, ok := jobHandles.LoadAndDelete(cmd)
	if !ok {
		return
	}
	job, ok := v.(windows.Handle)
	if !ok {
		return
	}
	if err := windows.CloseHandle(job); err != nil {
		slog.Warn("mcp runtime: CloseHandle(job) failed",
			"pid", cmd.Process.Pid, "error", err)
	}
}
