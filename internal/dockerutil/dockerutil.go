// Package dockerutil wraps the docker CLI (and the `docker compose` plugin)
// as subprocess calls. mcsync never talks to the Docker daemon directly.
package dockerutil

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Run executes `docker <args...>` in dir, streaming stdout/stderr live.
func Run(dir string, args ...string) error {
	cmd := exec.Command("docker", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Compose executes `docker compose <args...>` in dir, streaming output live.
func Compose(dir string, args ...string) error {
	return Run(dir, append([]string{"compose"}, args...)...)
}

// Output executes `docker <args...>` in dir and returns combined trimmed output.
func Output(dir string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// IsInstalled reports whether the docker executable is reachable.
func IsInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// ComposeInstalled reports whether the `docker compose` plugin is available.
func ComposeInstalled() bool {
	_, err := Output("", "compose", "version")
	return err == nil
}

// DaemonRunning reports whether the Docker daemon is reachable.
func DaemonRunning() bool {
	_, err := Output("", "info")
	return err == nil
}

// ComposeOutput executes `docker compose <args...>` in dir and returns
// combined trimmed output.
func ComposeOutput(dir string, args ...string) (string, error) {
	return Output(dir, append([]string{"compose"}, args...)...)
}

// ServiceStatus returns service's container state (e.g. "running",
// "exited") and health ("healthy"/"starting"/"unhealthy", or "" if the
// image defines no healthcheck). Errors if no container exists for it.
func ServiceStatus(dir, service string) (state, health string, err error) {
	id, err := ComposeOutput(dir, "ps", "-q", service)
	if err != nil {
		return "", "", err
	}
	if id == "" {
		return "", "", fmt.Errorf("no container for service %q", service)
	}
	out, err := Output(dir, "inspect", "--format",
		"{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", id)
	if err != nil {
		return "", "", err
	}
	parts := strings.SplitN(out, "|", 2)
	state = parts[0]
	if len(parts) > 1 {
		health = parts[1]
	}
	return state, health, nil
}

var readyLineRE = regexp.MustCompile(`Done \(`)

// WaitHealthy streams service's logs to stdout live (so the caller sees
// real startup progress -- Forge install, world load, etc.) and returns as
// soon as the container reports healthy. If the image defines no
// healthcheck, it falls back to watching for the "Done (" ready line that
// itzg/minecraft-server prints once the server has finished loading.
//
// It also prints a periodic heartbeat during long silent stretches (e.g.
// the Forge installer downloading dependencies with no per-line output),
// so a beginner watching the terminal knows it's still working rather than
// stuck. Returns an error if still not ready after timeout.
func WaitHealthy(dir, service string, timeout time.Duration) error {
	containerID, err := ComposeOutput(dir, "ps", "-q", service)
	if err != nil || containerID == "" {
		return fmt.Errorf("finding container for service %q: %w", service, err)
	}

	healthFormat := "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}"
	if status, _ := Output(dir, "inspect", "--format", healthFormat, containerID); status == "healthy" {
		fmt.Println("Already running.")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	logCtx, stopLogs := context.WithCancel(ctx)
	defer stopLogs()

	var readyOnce sync.Once
	ready := make(chan struct{})
	signalReady := func() { readyOnce.Do(func() { close(ready) }) }

	// --since now: only stream logs from this point forward, so restarting
	// `mcsync start` against an already-running (but not yet "healthy",
	// e.g. still loading) container doesn't replay its entire history.
	since := time.Now().Format(time.RFC3339Nano)
	logCmd := exec.CommandContext(logCtx, "docker", "compose", "logs", "-f", "--no-log-prefix", "--since", since, service)
	logCmd.Dir = dir
	stdout, err := logCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("starting log stream: %w", err)
	}
	logCmd.Stderr = os.Stdout
	if err := logCmd.Start(); err != nil {
		return fmt.Errorf("starting log stream: %w", err)
	}
	go func() { _ = logCmd.Wait() }()
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			fmt.Println(line)
			if readyLineRE.MatchString(line) {
				signalReady()
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		lastHeartbeat := start
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status, _ := Output(dir, "inspect", "--format", healthFormat, containerID)
				if status == "healthy" {
					signalReady()
					return
				}
				if time.Since(lastHeartbeat) >= 20*time.Second {
					fmt.Printf("... still starting (%s elapsed)\n", time.Since(start).Round(time.Second))
					lastHeartbeat = time.Now()
				}
			}
		}
	}()

	select {
	case <-ready:
		stopLogs()
		return nil
	case <-ctx.Done():
		stopLogs()
		return fmt.Errorf("timed out after %s waiting for the server to become healthy (check `docker compose logs mc`)", timeout)
	}
}
