package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
	"mcsync/internal/prompt"
	"mcsync/internal/serverproc"
)

var restoreYes bool

var restoreCmd = &cobra.Command{
	Use:   "restore [commit]",
	Short: "world/config/modsをgit履歴上の過去の保存状態に戻す",
	Long: `world/config/modsをgit履歴上の過去の保存状態に戻します。

「mcsync stop」の度にcommitが作られるため、git履歴がそのまま保存履歴になっています。
引数無しで実行すると直近の保存を一覧表示します。commitハッシュを指定すると、
data/world・data/config・data/mods・server.properties・ops.json・whitelist.jsonを
そのcommit時点の内容に戻し、新しいcommitとして記録します
(何も破棄されません -- 現在の状態も履歴に残ったまま辿れます)。`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRestore,
}

func init() {
	restoreCmd.Flags().BoolVarP(&restoreYes, "yes", "y", false, "確認プロンプトを省略する")
	rootCmd.AddCommand(restoreCmd)
}

func runRestore(c *cobra.Command, args []string) error {
	dir := "."
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		return fmt.Errorf("gitリポジトリではないため復元できません")
	}

	if len(args) == 0 {
		return listRestorePoints(dir)
	}
	commit := args[0]

	if _, err := gitutil.Output(dir, "rev-parse", "--verify", commit+"^{commit}"); err != nil {
		return fmt.Errorf("%q は有効なcommitではないようです(引数無しで `mcsync restore` を実行すると直近の保存を一覧表示します)", commit)
	}

	if running, _ := serverproc.IsRunning(mustAbs(dir)); running {
		return fmt.Errorf("サーバーが起動中です。復元したファイルがすぐ上書きされないよう、先に `mcsync stop` を実行してください")
	}

	hash, date, subject, err := gitutil.CommitInfo(dir, commit)
	if err != nil {
		return err
	}

	// Only restore paths that actually existed at that commit --
	// `git checkout <rev> -- <path>` errors on any pathspec absent there
	// (e.g. an early save made before data/mods was first tracked).
	var paths []string
	for _, p := range trackedStatePaths {
		if gitutil.PathExistsAtRevision(dir, commit, p) {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		return fmt.Errorf("追跡対象のパス(%s)は %s の時点でまだ存在しませんでした", strings.Join(trackedStatePaths, ", "), hash)
	}

	fmt.Printf("現在の %s を、以下の時点のバージョンで上書きします:\n\n  %s  %s\n  %s\n\n"+
		"現在の状態が失われるわけではありません(git履歴には残ります)が、ディスク上の内容は置き換わります。\n\n",
		strings.Join(paths, ", "), hash, date, subject)
	if !restoreYes && !prompt.Confirm("続行しますか？", false) {
		fmt.Println("キャンセルしました。")
		return nil
	}

	if err := gitutil.Run(dir, append([]string{"checkout", commit, "--"}, paths...)...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("既にその状態になっています。復元の必要はありません。")
		return nil
	}

	msg := fmt.Sprintf("Restore world state from %s (%s)", hash, date)
	if err := gitutil.Run(dir, "commit", "-m", msg); err != nil {
		return err
	}

	if gitutil.HasRemote(dir, "origin") {
		fmt.Println("pushしています...")
		if err := gitutil.Run(dir, "push"); err != nil {
			return fmt.Errorf("%w\n(復元はローカルにcommit済みです。解決後に手動でpushしてください)", err)
		}
		pruneLFSCache(dir)
	}

	fmt.Println("\n完了しました。復元した状態でサーバーを起動するには `mcsync start` を実行してください。")
	return nil
}

func listRestorePoints(dir string) error {
	entries, err := gitutil.LogPaths(dir, 20, "data/world")
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Println("保存が見つかりません(data/worldの履歴がありません)。")
		return nil
	}
	fmt.Println("直近の保存(data/worldに関わるcommit):")
	for _, e := range entries {
		fmt.Printf("  %s  %s  %s\n", e.Hash, e.Date, e.Subject)
	}
	fmt.Println("\n`mcsync restore <hash>` でいずれかの状態に戻せます。")
	return nil
}
