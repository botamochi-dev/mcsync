//go:build windows

package serverproc

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NEW_PROCESS_GROUP | DETACHED_PROCESS: lets the supervisor outlive
// the `mcsync start` console and not receive Ctrl+C/Ctrl+Break from it.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
	createNoWindow        = 0x08000000
	stillActive           = 259
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
}

// HideWindow prevents a console-subsystem child (java.exe) from getting a
// new, visible console window of its own. Redirecting its stdio handles
// (as the supervisor does) isn't enough by itself -- Windows still pops up
// a console for a console-subsystem process unless CREATE_NO_WINDOW is set
// explicitly, since the supervisor that spawns it has no console at all
// (see detach above) for it to inherit.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// forceKill uses `taskkill /T` so the whole tree is terminated -- the
// supervisor's child may itself be a cmd.exe running run.bat, which would
// otherwise be left as an orphan holding java.exe alive.
func forceKill(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
