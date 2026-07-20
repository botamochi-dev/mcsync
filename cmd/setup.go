package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
	"mcsync/internal/serverproc"
)

var setupCmd = &cobra.Command{
	Use:   "setup <git-url> [dir]",
	Short: "既存のmcsyncプロジェクトをcloneしてサーバーを起動する(新しいPCでの再開用)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(c *cobra.Command, args []string) error {
	repoURL := args[0]
	dir := ""
	if len(args) == 2 {
		dir = args[1]
	} else {
		dir = dirNameFromRepoURL(repoURL)
	}

	if !gitutil.IsInstalled() {
		return fmt.Errorf("gitがインストールされていません。詳細は `mcsync doctor` を実行してください")
	}
	if !gitutil.LFSInstalled() {
		return fmt.Errorf("git-lfsがインストールされていません。mcsyncプロジェクトはmod jar(data/mods)をgit-lfsで管理しているため、" +
			"無いままcloneすると中身の無いプレースホルダーファイルになってしまいます。" +
			"https://git-lfs.com からインストールしてから再度お試しください")
	}

	// Make sure git-lfs's filters are registered even if `git lfs install`
	// was never run on this PC before -- otherwise the clone below would
	// silently leave mod jars as tiny LFS pointer files instead of the
	// real content.
	if err := gitutil.LFSInstall("."); err != nil {
		return fmt.Errorf("`git lfs install` に失敗しました: %w", err)
	}

	fmt.Printf("%s を %s にcloneしています...\n", repoURL, dir)
	if err := gitutil.Run(".", "clone", repoURL, dir); err != nil {
		return err
	}

	javaBinDir, err := prepareServer(dir)
	if err != nil {
		return err
	}

	fmt.Println("サーバーを起動しています...")
	if err := serverproc.Start(mustAbs(dir), javaBinDir); err != nil {
		return err
	}

	fmt.Println("サーバーの起動完了を待っています(新しいPCでの初回起動はForgeのダウンロード・" +
		"インストールが走るため時間がかかります)...")
	if err := waitReady(dir, startupTimeout); err != nil {
		return err
	}
	fmt.Println("\nサーバーが起動しました。Minecraftのマルチプレイ画面から localhost で接続できます。")
	return nil
}

func dirNameFromRepoURL(repoURL string) string {
	name := repoURL
	if i := strings.LastIndexAny(name, "/\\"); i != -1 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	if name == "" {
		name = "mcsync-server"
	}
	return filepath.Clean(name)
}
