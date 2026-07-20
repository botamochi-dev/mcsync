// Package cmd implements the mcsync CLI. mcsync runs a self-hosted Forge
// Minecraft server directly on the host (no Docker): mcsync.yml is the
// manifest (MC/Forge version, memory), and mcsync itself downloads/caches
// Java and Forge, manages the server process, and wraps the git commands
// needed to sync world/config across PCs.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "mcsync",
	Short: "自宅Forge Minecraftサーバー(world + config)を複数PC間でgit同期する",
	Long: `mcsyncは、自宅Forge Minecraftサーバーをホスト上で直接実行し
(JavaとForgeはmcsync自身がダウンロード・管理するのでDockerは不要です)、
world/configを複数PC間で同期するためのgitコマンドをまとめて実行します。

mcsync.ymlがマニフェストで、Minecraft/Forgeのバージョンとメモリ量を記述します。`,
	SilenceUsage: true,
}

// Execute runs the CLI, printing any error and returning it so main can
// set a non-zero exit code.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "エラー:", err)
		return err
	}
	return nil
}
