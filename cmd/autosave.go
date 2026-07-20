package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
)

var autosaveInterval time.Duration

var autosaveCmd = &cobra.Command{
	Use:   "autosave",
	Short: "サーバーを起動したまま定期的にworld/config/modsを保存(commit/push)する",
	Long: `サーバーを起動したまま定期的にworld/config/modsを保存(commit/push)します。
「mcsync stop」と違い、サーバーは停止しません。

Ctrl+Cで中断するまでフォアグラウンドで動き続けます。保存の直前にベストエフォートで
"save-all"をサーバーのコンソールに送るので、書き込み中の中途半端な状態のまま
保存してしまう可能性を減らしています。`,
	RunE: runAutosave,
}

func init() {
	autosaveCmd.Flags().DurationVar(&autosaveInterval, "interval", 15*time.Minute, "保存の間隔")
	rootCmd.AddCommand(autosaveCmd)
}

func runAutosave(c *cobra.Command, args []string) error {
	dir := "."
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		return fmt.Errorf("gitリポジトリではありません。先に `mcsync init` を実行してください")
	}
	fmt.Printf("%s おきに自動保存します。サーバーは停止しません -- 中断するには Ctrl+C を押してください。\n", autosaveInterval)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)

	ticker := time.NewTicker(autosaveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sigCh:
			fmt.Println("\n自動保存を停止しました(サーバーは起動したままです)。")
			return nil
		case <-ticker.C:
			fmt.Printf("\n[%s] 自動保存しています...\n", time.Now().Format("15:04:05"))
			if err := saveTrackedState(dir, true); err != nil {
				fmt.Printf("自動保存に失敗しました: %v\n", err)
			}
		}
	}
}
