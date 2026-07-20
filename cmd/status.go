package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"mcsync/internal/gitutil"
	"mcsync/internal/manifest"
	"mcsync/internal/serverproc"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "サーバーの状態・保存履歴・リポジトリの状況を一目で確認する",
	RunE:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(c *cobra.Command, args []string) error {
	dir := "."

	fmt.Println("サーバー:")
	printServerStatus(dir)

	fmt.Println("\nGit:")
	if !gitutil.IsInstalled() || !gitutil.IsRepo(dir) {
		fmt.Println("  gitリポジトリではありません(先に `mcsync init` を実行してください)")
	} else {
		printGitStatus(dir)
		printLockStatus(dir)
	}

	fmt.Println("\nディスク使用量:")
	printSize("  data/world", dirSize(filepath.Join(dir, "data", "world")))
	gitSize := dirSize(filepath.Join(dir, ".git"))
	lfsSize := dirSize(filepath.Join(dir, ".git", "lfs", "objects"))
	printSize("  .git", gitSize)
	if lfsSize > 0 {
		printSize("    うちLFSキャッシュ", lfsSize)
	}

	return nil
}

func printServerStatus(dir string) {
	running, pid := serverproc.IsRunning(mustAbs(dir))
	if !running {
		fmt.Println("  起動していません")
		return
	}
	fmt.Printf("  起動中 (pid %d)", pid)
	if javaPID, ok := serverproc.JavaPID(mustAbs(dir)); ok {
		if mem, ok := serverproc.MemoryUsage(javaPID); ok {
			fmt.Printf(", メモリ %s", formatSize(mem))
		}
	}
	fmt.Println()
}

func printGitStatus(dir string) {
	hash, date, subject, err := gitutil.CommitInfo(dir, "HEAD")
	if err == nil {
		fmt.Printf("  最終保存: %s  %s  (%s)\n", date, subject, hash)
	}

	// Scoped to the paths mcsync actually manages, so a stray unrelated
	// file sitting in the project folder doesn't get reported as an
	// "uncommitted change" to the server's saved state.
	relevantPaths := append([]string{manifest.FileName, ".gitignore", ".gitattributes"}, trackedStatePaths...)
	dirty, err := gitutil.HasChanges(dir, relevantPaths...)
	if err == nil && dirty {
		fmt.Println("  未commitの変更があります(`mcsync stop` を実行して保存してください)")
	}

	if !gitutil.HasRemote(dir, "origin") {
		fmt.Println("  リモート未設定")
		return
	}
	ahead, behind, err := gitutil.AheadBehind(dir)
	if err != nil {
		fmt.Println("  リモートは設定されていますが比較できません(upstreamブランチが未設定?)")
		return
	}
	switch {
	case ahead > 0 && behind > 0:
		fmt.Printf("  origin より %d commit進んでいて %d commit遅れています -- push・pullが必要です\n", ahead, behind)
	case ahead > 0:
		fmt.Printf("  未pushのcommitが %d 件あります\n", ahead)
	case behind > 0:
		fmt.Printf("  originより %d commit遅れています -- `mcsync start` でpullしてください\n", behind)
	default:
		fmt.Println("  originと同期済みです")
	}
}

func printLockStatus(dir string) {
	info, err := readLock(dir)
	if err != nil || info == nil {
		fmt.Println("  ロック: なし(このサーバーを起動中のPCはありません)")
		return
	}
	host, _ := os.Hostname()
	who := info.Host
	if info.Host == host {
		who += " (このPC)"
	}
	fmt.Printf("  ロック: %s%s が %s から保持中\n", who, userSuffix(info.User), info.StartedAt.Format("2006-01-02 15:04"))
}

func dirSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func printSize(label string, bytes int64) {
	fmt.Printf("%s: %s\n", label, formatSize(bytes))
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
