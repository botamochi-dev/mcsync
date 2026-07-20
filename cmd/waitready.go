package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"mcsync/internal/serverproc"
)

var readyLineRE = regexp.MustCompile(`Done \(`)

// waitReady tails dir/data/logs/latest.log, printing lines live (so
// startup progress -- Forge install, world load, etc. -- is visible just
// like it was via `docker compose logs -f`), until the "Done (" ready line
// appears or the timeout elapses. It bails out early if the supervised
// process is no longer running.
func waitReady(dir string, timeout time.Duration) error {
	projectDir := mustAbs(dir)
	logPath := filepath.Join(dir, "data", "logs", "latest.log")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		if _, err := os.Stat(logPath); err == nil {
			break
		}
		if running, _ := serverproc.IsRunning(projectDir); !running {
			return fmt.Errorf("起動完了前にサーバープロセスが終了しました。%s を確認してください",
				filepath.Join(dir, ".mcsync", "server-console.log"))
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("サーバーがログの出力を始めるのを待っている間にタイムアウトしました")
		case <-time.After(1 * time.Second):
		}
	}

	f, err := os.Open(logPath)
	if err != nil {
		return err
	}
	defer f.Close()

	reader := bufio.NewReader(f)
	start := time.Now()
	lastHeartbeat, lastLine := start, start
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			fmt.Print(line)
			if readyLineRE.MatchString(line) {
				return nil
			}
			lastLine = time.Now()
		}
		if readErr == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("起動完了を %s 待ちましたがタイムアウトしました(%s を確認してください)", timeout, logPath)
		default:
		}
		if running, _ := serverproc.IsRunning(projectDir); !running {
			return fmt.Errorf("起動完了前にサーバープロセスが終了しました。%s と %s を確認してください",
				logPath, filepath.Join(dir, ".mcsync", "server-console.log"))
		}
		if time.Since(lastHeartbeat) >= 20*time.Second && time.Since(lastLine) >= 20*time.Second {
			fmt.Printf("... 起動中です(経過 %s)\n", time.Since(start).Round(time.Second))
			lastHeartbeat = time.Now()
		}
		time.Sleep(300 * time.Millisecond)
	}
}
