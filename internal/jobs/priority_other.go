//go:build !linux

package jobs

// Deprioritise is a no-op away from Linux: per-thread I/O and CPU priority is a
// Linux facility, and the development machine is not what needs protecting.
func Deprioritise() (restore func()) { return func() {} }
