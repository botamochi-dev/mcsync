package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"mcsync/internal/forgeinstall"
	"mcsync/internal/serverproc"
)

// superviseCmd is an internal command: `mcsync start`/`setup` re-invokes
// itself as `mcsync __supervise <project-dir> <java-bin-dir>`, detached, so
// something keeps holding the Forge server's stdin open (to send it a
// graceful "stop" later) after the original `start` invocation has already
// returned control to the user's shell. Not meant to be run by hand.
var superviseCmd = &cobra.Command{
	Use:    "__supervise <project-dir> <java-bin-dir>",
	Hidden: true,
	Args:   cobra.ExactArgs(2),
	RunE:   runSupervise,
}

func init() {
	rootCmd.AddCommand(superviseCmd)
}

func runSupervise(c *cobra.Command, args []string) error {
	projectDir, javaBinDir := args[0], args[1]
	dataDir := filepath.Join(projectDir, "data")
	stateDir := filepath.Join(projectDir, ".mcsync")

	launcher, err := forgeinstall.FindLauncher(dataDir)
	if err != nil {
		return err
	}

	javaExe := filepath.Join(javaBinDir, javaExeName())
	var javaArgs []string
	if launcher.JarPath != "" {
		javaArgs = []string{"-jar", launcher.JarPath}
	} else {
		javaArgs = append([]string{}, launcher.Args...)
	}
	// Deliberately not passing "nogui": leaving it off is what makes
	// Minecraft/Forge show its own server GUI (memory/tick stats, player
	// list, log & chat), same as double-clicking run.bat directly. The
	// graceful "stop" path below doesn't depend on that GUI at all -- it
	// writes to the process's real stdin, which Minecraft's console
	// command reader thread consumes regardless of whether the GUI is
	// also showing.
	//
	// Run java directly (no cmd.exe/sh wrapper): Forge's own run.bat ends
	// with "pause", which would otherwise block forever reading from
	// stdin after Minecraft itself has already exited, breaking graceful
	// stop (see forgeinstall.Launcher).
	serverCmd := exec.Command(javaExe, javaArgs...)
	serverCmd.Dir = dataDir
	serverCmd.Env = append(os.Environ(), "PATH="+javaBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// The supervisor itself has no console (it's spawned detached -- see
	// serverproc.Start), so without this, Windows would additionally pop
	// up a plain console window for java.exe (a console-subsystem
	// process) alongside its own GUI. This only suppresses that extra
	// console, not the Minecraft/Swing GUI window itself.
	serverproc.HideWindow(serverCmd)

	consoleLog, err := os.OpenFile(filepath.Join(stateDir, "server-console.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer consoleLog.Close()
	serverCmd.Stdout = consoleLog
	serverCmd.Stderr = consoleLog

	stdin, err := serverCmd.StdinPipe()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("制御ソケットのオープンに失敗しました: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	portPath := filepath.Join(stateDir, "control.port")
	if err := os.WriteFile(portPath, []byte(strconv.Itoa(port)), 0o644); err != nil {
		return err
	}
	defer os.Remove(portPath)

	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("サーバープロセスの起動に失敗しました: %w", err)
	}
	// Recorded separately from our own (the supervisor's) PID in
	// server.pid, so `mcsync status` can report java's actual memory
	// usage instead of the tiny supervisor wrapper's.
	javaPIDPath := filepath.Join(stateDir, "java.pid")
	if err := os.WriteFile(javaPIDPath, []byte(strconv.Itoa(serverCmd.Process.Pid)), 0o644); err != nil {
		return fmt.Errorf("javaプロセスのPID記録に失敗しました: %w", err)
	}
	defer os.Remove(javaPIDPath)

	exited := make(chan struct{})
	go func() {
		_ = serverCmd.Wait()
		close(exited)
	}()

	for {
		connCh := make(chan net.Conn, 1)
		go func() {
			conn, err := listener.Accept()
			if err == nil {
				connCh <- conn
			}
		}()

		select {
		case <-exited:
			return nil
		case conn := <-connCh:
			forwardCommand(conn, stdin)
		}
	}
}

// forwardCommand copies whatever lines a control-socket client sends
// straight into the server's console stdin, same as if they'd been typed
// by hand (e.g. "stop", "save-all").
func forwardCommand(conn net.Conn, stdin io.Writer) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fmt.Fprintln(stdin, scanner.Text())
	}
}
