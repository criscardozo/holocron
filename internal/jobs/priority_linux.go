//go:build linux

package jobs

import (
	"runtime"
	"syscall"
)

// ioprioSet is the raw syscall number for ioprio_set on Linux.
const ioprioSet = 251

// I/O priority encoding: the class lives in the top 3 bits of the 16-bit value.
const (
	ioprioClassShift = 13
	ioprioClassIdle  = 3
	ioprioWhoProcess = 1 // applies to the calling thread
)

// Deprioritise lowers the calling goroutine's I/O and CPU priority for the rest
// of its life, and returns a function that restores them.
//
// This is per-thread on Linux, which is exactly what we want: only the thread
// running a heavy scan yields, while the HTTP handlers and the interactive
// drill-down keep normal priority. Niceness only ever goes down, so this needs
// no privileges and works under NoNewPrivileges.
//
// The Pi's media disk is a 5400 rpm USB drive that thrashes under two
// concurrent readers, and its scheduler is BFQ — which honours these hints.
// Failures are ignored: a scan that cannot be deprioritised is still a scan.
func Deprioritise() (restore func()) {
	runtime.LockOSThread()

	// ioprio_set(IOPRIO_WHO_PROCESS, 0 /* this thread */, IDLE)
	idle := uintptr(ioprioClassIdle << ioprioClassShift)
	_, _, _ = syscall.Syscall(ioprioSet, ioprioWhoProcess, 0, idle)

	// Nice only matters when the CPU is contended; +5 keeps us behind
	// playback and behind Jellyfin's transcodes.
	previous, err := syscall.Getpriority(syscall.PRIO_PROCESS, 0)
	if err == nil {
		_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 5)
	}

	return func() {
		if err == nil {
			// Getpriority returns nice as 20-nice; convert back before setting.
			_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 20-previous)
		}
		runtime.UnlockOSThread()
	}
}
