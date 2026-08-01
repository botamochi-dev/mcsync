package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcsync/internal/serverproc"
)

var restartNoWait bool

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "gitのcommit/pushを行わずにサーバーだけを再起動する(mod追加などの動作確認向け)",
	Long: `サーバーを再起動しますが、` + "`mcsync stop`" + `/` + "`mcsync start`" + `と違いgitのpull・commit・pushを一切行いません。

data/modsにjarを置いて動作確認する時など、サーバーを再起動したいだけなのに
git操作(特にmod jarのpush/pull)の分だけ余計に時間がかかる場面向けです。
動作確認が終わったら、通常通り ` + "`mcsync stop`" + ` を実行して変更を保存してください。`,
	RunE: runRestart,
}

func init() {
	restartCmd.Flags().BoolVar(&restartNoWait, "no-wait", false, "起動完了を待たずにすぐ戻る")
	rootCmd.AddCommand(restartCmd)
}

func runRestart(c *cobra.Command, args []string) error {
	dir := "."

	if running, _ := serverproc.IsRunning(mustAbs(dir)); running {
		fmt.Println("サーバーを停止しています...")
		if err := serverproc.Stop(mustAbs(dir), stopTimeout); err != nil {
			return err
		}
	} else {
		fmt.Println("サーバーは起動していません。")
	}

	javaBinDir, err := prepareServer(dir)
	if err != nil {
		return err
	}

	fmt.Println("サーバーを起動しています...")
	if err := serverproc.Start(mustAbs(dir), javaBinDir); err != nil {
		return err
	}

	if restartNoWait {
		fmt.Println("\nバックグラウンドでサーバーを起動しています。進捗は `mcsync status` で確認できます。")
		return nil
	}

	fmt.Println("サーバーの起動完了を待っています...")
	if err := waitReady(dir, startupTimeout); err != nil {
		return err
	}
	fmt.Println("\nサーバーが再起動しました。")
	return nil
}
