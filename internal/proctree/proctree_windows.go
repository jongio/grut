//go:build windows

package proctree

import (
	"log/slog"
	"os/exec"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobHandles maps a running *exec.Cmd to the Windows Job Object handle that
// contains its process tree. The handle is stored after the process starts
// (AfterStart) and removed when the tree is killed (Kill).
var jobHandles sync.Map // map[*exec.Cmd]windows.Handle

// Configure is a no-op on Windows; process-tree containment is established
// after start via a Job Object in AfterStart.
func Configure(_ *exec.Cmd) {}

// AfterStart creates a Windows Job Object configured with
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE and assigns the newly started process to
// it. When the job handle is later closed (in Kill), Windows terminates every
// process in the job — the direct child and all its descendants — preventing
// orphaned process trees (CWE-269). It must be called after cmd.Start succeeds.
func AfterStart(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		slog.Warn("proctree: CreateJobObject failed, child processes may orphan",
			"pid", cmd.Process.Pid, "error", err)
		return
	}

	// Configure the job to kill all contained processes when the last handle
	// is closed.
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
		slog.Warn("proctree: SetInformationJobObject failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(job)
		return
	}

	// Open the process handle with the rights needed to assign it to the job.
	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		slog.Warn("proctree: OpenProcess failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(job)
		return
	}

	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		slog.Warn("proctree: AssignProcessToJobObject failed",
			"pid", cmd.Process.Pid, "error", err)
		_ = windows.CloseHandle(proc)
		_ = windows.CloseHandle(job)
		return
	}

	_ = windows.CloseHandle(proc)
	jobHandles.Store(cmd, job)

	slog.Debug("proctree: process assigned to job object", "pid", cmd.Process.Pid)
}

// Kill closes the Windows Job Object handle for cmd, which causes Windows to
// terminate every process in the job tree. It is safe to call multiple times
// and on a process that has already exited (subsequent calls are no-ops).
func Kill(cmd *exec.Cmd) {
	v, ok := jobHandles.LoadAndDelete(cmd)
	if !ok {
		return
	}
	job, ok := v.(windows.Handle)
	if !ok {
		return
	}
	if err := windows.CloseHandle(job); err != nil {
		slog.Warn("proctree: CloseHandle(job) failed", "error", err)
	}
}
