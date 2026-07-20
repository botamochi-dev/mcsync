package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
	"mcsync/internal/serverproc"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "サーバーを停止し、world/config/modの変更をcommit・pushする",
	RunE:  runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

// stopTimeout bounds how long we wait for a graceful "stop" (world save +
// clean exit) before forcing termination.
const stopTimeout = 2 * time.Minute

// trackedStatePaths are the parts of data/ that .gitignore keeps (see
// scaffold.Gitignore): world, config, server.properties, ops/whitelist,
// and mods (jars placed directly in data/mods, tracked via Git LFS -- see
// scaffold.ModsLFSPattern).
var trackedStatePaths = []string{
	filepath.Join("data", "world"),
	filepath.Join("data", "config"),
	filepath.Join("data", "server.properties"),
	filepath.Join("data", "ops.json"),
	filepath.Join("data", "whitelist.json"),
	filepath.Join("data", "mods"),
}

func runStop(c *cobra.Command, args []string) error {
	dir := "."

	if running, _ := serverproc.IsRunning(mustAbs(dir)); running {
		fmt.Println("サーバーを停止しています...")
		if err := serverproc.Stop(mustAbs(dir), stopTimeout); err != nil {
			return err
		}
	} else {
		fmt.Println("サーバーは起動していません。")
	}

	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		fmt.Println("gitリポジトリではないため保存をスキップします。world/configを同期したい場合は先に `mcsync init` を実行してください。")
		return nil
	}

	if err := saveTrackedState(dir, false); err != nil {
		return err
	}
	fmt.Println("\n完了しました。world/configを保存しました。")
	return nil
}

// saveTrackedState commits (and pushes, if a remote is configured) any
// changes under trackedStatePaths. Shared by `stop` (server already
// stopped) and `autosave` (server still running -- liveServer best-effort
// asks the server to save-all via its console first, and leaves the start
// lock alone since the session isn't actually over).
func saveTrackedState(dir string, liveServer bool) error {
	if liveServer {
		_ = serverproc.SendCommand(mustAbs(dir), "save-all")
		// Give the write a moment to land before snapshotting; best-effort,
		// same spirit as before -- there's no ack to wait on without RCON.
		time.Sleep(2 * time.Second)
	}

	if !liveServer && worldLocked(dir) {
		return fmt.Errorf("data/worldが起動中のMinecraftサーバーによってまだ開かれているため、今は保存できません。\n" +
			"多くの場合、mcsyncを使わずに(例えばrun.batを直接ダブルクリックして)サーバーを起動しており、" +
			"mcsyncがそれを把握できず上で停止できなかったことが原因です。\n" +
			"そちらのウィンドウ/コンソールでサーバーを停止してから、もう一度 `mcsync stop` を実行してください。")
	}

	var toAdd []string
	for _, p := range trackedStatePaths {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			toAdd = append(toAdd, p)
		}
	}
	if !liveServer {
		lockFiles, err := removeLockAndStage(dir)
		if err != nil {
			fmt.Printf("警告: ロックファイルの削除に失敗しました: %v\n", err)
		}
		toAdd = append(toAdd, lockFiles...)
	}
	if len(toAdd) == 0 {
		fmt.Println("保存対象のworld/config/modデータが見つかりませんでした。")
		return nil
	}

	if err := gitutil.Run(dir, append([]string{"add"}, toAdd...)...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("前回の保存からworld/config/modに変更はありません。")
		return nil
	}

	msg := fmt.Sprintf("Save world state: %s", time.Now().Format("2006-01-02 15:04:05"))
	if err := gitutil.Run(dir, "commit", "-m", msg); err != nil {
		return err
	}

	if gitutil.HasRemote(dir, "origin") {
		fmt.Println("pushしています...")
		if err := gitutil.Run(dir, "push"); err != nil {
			return fmt.Errorf("%w\n(保存はローカルにcommit済みです。解決後に手動でpushしてください)", err)
		}
		pruneLFSCache(dir)
	} else {
		fmt.Println("Gitリモートが設定されていないため、ローカルにのみ保存しました。")
	}
	return nil
}

// worldLocked reports whether data/world/session.lock is currently held
// by a running Minecraft process. Minecraft keeps a byte-range lock on
// that file for as long as the world is loaded (it's how the game itself
// detects "is this world already in use"). On Windows, merely opening the
// file still succeeds -- the violation only surfaces once something
// actually tries to read the locked range, which is exactly what trips up
// `git add` (and what git's own error above came from). So we do the same
// read here, up front, to turn that into one clear error instead of `git
// add` failing confusingly partway through indexing data/world -- any
// server holding the lock counts, not just one mcsync itself started.
func worldLocked(dir string) bool {
	path := filepath.Join(dir, "data", "world", "session.lock")
	f, err := os.Open(path)
	if err != nil {
		return !os.IsNotExist(err)
	}
	defer f.Close()
	var buf [1]byte
	_, err = f.Read(buf[:])
	return err != nil && err != io.EOF
}
