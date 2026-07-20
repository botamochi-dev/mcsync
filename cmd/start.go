package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
	"mcsync/internal/serverproc"
)

var (
	startNoWait bool
	startForce  bool
)

// startupTimeout is generous because the first boot on a fresh PC
// downloads Java and Forge (and installs Forge) from scratch.
const startupTimeout = 30 * time.Minute

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "gitから最新のworld/configを取り込んでからサーバーを起動する",
	RunE:  runStart,
}

func init() {
	startCmd.Flags().BoolVar(&startNoWait, "no-wait", false, "起動完了を待たずにすぐ戻る")
	startCmd.Flags().BoolVar(&startForce, "force", false, "他PCのロックが残っていても強制的に起動する")
	rootCmd.AddCommand(startCmd)
}

func runStart(c *cobra.Command, args []string) error {
	dir := "."
	tracked := gitutil.IsInstalled() && gitutil.IsRepo(dir)
	remoteConfigured := tracked && gitutil.HasRemote(dir, "origin")

	if tracked {
		if remoteConfigured {
			fmt.Println("最新の変更を取り込んでいます...")
			if err := gitutil.Run(dir, "pull", "--ff-only"); err != nil {
				return fmt.Errorf("%w\n(マージコンフリクトの場合は、world/configの競合を解決してからサーバーを起動してください)", err)
			}
			pruneLFSCache(dir)
			if err := checkLock(dir, startForce); err != nil {
				return err
			}
		} else {
			fmt.Println("Gitリモートが設定されていないため、pullをスキップします。")
		}
	}

	javaBinDir, err := prepareServer(dir)
	if err != nil {
		return err
	}

	fmt.Println("サーバーを起動しています...")
	if err := serverproc.Start(mustAbs(dir), javaBinDir); err != nil {
		return err
	}

	if remoteConfigured {
		if err := writeLockAndCommit(dir); err != nil {
			fmt.Printf("警告: 起動ロックの記録に失敗しました: %v\n", err)
		}
	}

	if startNoWait {
		fmt.Println("\nバックグラウンドでサーバーを起動しています。進捗は `mcsync status` で確認できます。")
		return nil
	}

	fmt.Println("サーバーの起動完了を待っています(新しいPCでの初回起動はForgeのダウンロード・" +
		"インストールが走るため時間がかかります)...")
	if err := waitReady(dir, startupTimeout); err != nil {
		return err
	}
	fmt.Println("\nサーバーが起動しました。Minecraftのマルチプレイ画面から localhost で接続できます。")
	return nil
}
