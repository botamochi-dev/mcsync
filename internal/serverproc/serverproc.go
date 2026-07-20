// Package serverproc manages the Forge server process directly (no
// Docker): it launches a small detached "supervisor" process (mcsync
// re-invoking itself as `__supervise`) that owns the actual java process
// and its stdin, and exposes a tiny loopback-only control socket so a
// later, separate `mcsync stop`/`autosave` invocation can ask it to send a
// command (graceful "stop", best-effort "save-all") to the server's
// console -- the same thing typing into the server console by hand would
// do. There's no Minecraft RCON involved: the control socket is mcsync's
// own, ephemeral, 127.0.0.1-only, and torn down with the server.
package serverproc

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func buildSupervisorCmd(exe, projectDir, javaBinDir string) *exec.Cmd {
	return exec.Command(exe, "__supervise", projectDir, javaBinDir)
}

// StateDir returns the per-project directory holding local-only runtime
// state (PID file, control port, logs, install markers). Never synced.
func StateDir(projectDir string) string { return filepath.Join(projectDir, ".mcsync") }

func pidFilePath(projectDir string) string     { return filepath.Join(StateDir(projectDir), "server.pid") }
func javaPIDFilePath(projectDir string) string { return filepath.Join(StateDir(projectDir), "java.pid") }
func portFilePath(projectDir string) string    { return filepath.Join(StateDir(projectDir), "control.port") }
func logFilePath(projectDir string) string     { return filepath.Join(StateDir(projectDir), "supervisor.log") }

// JavaPID returns the PID of the actual java process the supervisor for
// projectDir is running, if it's currently up. This is distinct from the
// supervisor's own PID (see IsRunning) -- callers that want the real
// server's resource usage (e.g. `mcsync status`'s memory reading) need
// this one, not the tiny supervisor wrapper's.
func JavaPID(projectDir string) (int, bool) {
	data, err := os.ReadFile(javaPIDFilePath(projectDir))
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

// IsRunning reports whether a supervised server process is currently
// running for projectDir (an absolute path).
func IsRunning(projectDir string) (bool, int) {
	pid, err := readPID(projectDir)
	if err != nil {
		return false, 0
	}
	if !processAlive(pid) {
		return false, 0
	}
	return true, pid
}

func readPID(projectDir string) (int, error) {
	data, err := os.ReadFile(pidFilePath(projectDir))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func readPort(projectDir string) (int, error) {
	data, err := os.ReadFile(portFilePath(projectDir))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// Start launches the supervisor as a detached background process, which in
// turn launches the Forge server (found via forgeinstall.FindLauncher) with
// javaBinDir on its PATH. It returns as soon as the supervisor has been
// spawned; it does not wait for Minecraft to finish booting -- callers
// should watch projectDir/data/logs/latest.log for that (see waitReady in
// package cmd), same as before.
func Start(projectDir, javaBinDir string) error {
	if running, _ := IsRunning(projectDir); running {
		return fmt.Errorf("サーバーは既に起動しています(`mcsync status` で確認できます)")
	}
	if err := os.MkdirAll(StateDir(projectDir), 0o755); err != nil {
		return err
	}
	_ = os.Remove(portFilePath(projectDir))

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("mcsync実行ファイルの場所を特定できませんでした: %w", err)
	}

	logFile, err := os.OpenFile(logFilePath(projectDir), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := buildSupervisorCmd(exe, projectDir, javaBinDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	detach(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("supervisorプロセスの起動に失敗しました: %w", err)
	}
	if err := os.WriteFile(pidFilePath(projectDir), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644); err != nil {
		return err
	}
	// Detached: we don't Wait() on the supervisor, just stop tracking it.
	_ = cmd.Process.Release()
	return nil
}

// Stop asks the running supervised server to shut down gracefully (via the
// control socket, equivalent to typing "stop" into the console) and waits
// up to timeout for it to exit, force-killing the process tree as a last
// resort.
func Stop(projectDir string, timeout time.Duration) error {
	running, pid := IsRunning(projectDir)
	if !running {
		cleanup(projectDir)
		return nil
	}

	if port, err := readPort(projectDir); err == nil {
		if err := sendControl(port, "stop"); err != nil {
			fmt.Printf("警告: サーバーの制御チャンネルに接続できませんでした(%v)。念のため停止を待ちます。\n", err)
		}
	} else {
		fmt.Println("警告: このサーバーの制御チャンネルが記録されていません。少し待ってから強制終了します。")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			cleanup(projectDir)
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("警告: 時間内に正常停止しなかったため、強制終了します。")
	if err := forceKill(pid); err != nil {
		return fmt.Errorf("サーバー(pid %d)の強制終了に失敗しました: %w", pid, err)
	}
	cleanup(projectDir)
	return nil
}

func cleanup(projectDir string) {
	_ = os.Remove(pidFilePath(projectDir))
	_ = os.Remove(portFilePath(projectDir))
}

// SendCommand best-effort forwards a single console command (e.g.
// "save-all") to the running server's stdin, equivalent to typing it into
// the console by hand. Failures are non-fatal to the caller.
func SendCommand(projectDir, command string) error {
	port, err := readPort(projectDir)
	if err != nil {
		return err
	}
	return sendControl(port, command)
}

func sendControl(port int, line string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = fmt.Fprintln(conn, line)
	return err
}
