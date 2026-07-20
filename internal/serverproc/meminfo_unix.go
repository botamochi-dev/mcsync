//go:build !windows

package serverproc

// MemoryUsage isn't implemented outside Windows -- use `ps`/`top`/Activity
// Monitor with the PID `mcsync status` prints instead.
func MemoryUsage(pid int) (bytes int64, ok bool) { return 0, false }
