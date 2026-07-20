//go:build !windows

package serverproc

import (
	"os"
	"os/exec"
	"syscall"
)

// Setsid: gives the supervisor (and everything it spawns) its own session,
// so it outlives the `mcsync start` shell and doesn't receive signals sent
// to that shell's process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

// HideWindow is a no-op outside Windows -- there's no console-window
// concept to suppress.
func HideWindow(cmd *exec.Cmd) {}

func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

// forceKill signals the whole process group (Setsid above makes pid equal
// to the group id) so a shell wrapper (run.sh) can't leave java running as
// an orphan.
func forceKill(pid int) error {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
