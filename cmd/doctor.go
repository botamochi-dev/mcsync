package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"mcsync/internal/ghutil"
	"mcsync/internal/gitutil"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "gitとgit-lfsが正しくインストールされているか確認する",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

type check struct {
	label    string
	ok       bool
	hint     string
	required bool
}

func runDoctor(c *cobra.Command, args []string) error {
	checks := []check{
		{"git がインストールされています", gitutil.IsInstalled(), "gitをインストールしてください: https://git-scm.com/downloads", true},
	}
	if checks[0].ok {
		checks = append(checks,
			check{"git-lfs がインストールされています", gitutil.LFSInstalled(), "git-lfs(data/modsのmod jar同期に使用)をインストールしてください: https://git-lfs.com", true},
		)
	}

	ghInstalled := ghutil.IsInstalled()
	ghCheck := check{label: "gh (GitHub CLI) がインストールされています(任意)", ok: ghInstalled,
		hint: "`mcsync init`でのGitHubリポジトリ自動作成に使います。無くても他の機能には影響しません: https://cli.github.com/"}
	if ghInstalled {
		if _, ok := ghutil.CurrentAccount(); !ok {
			ghCheck.ok = false
			ghCheck.hint = "`gh auth login` で認証してください(GitHubリポジトリ自動作成に必要)"
		}
	}
	checks = append(checks, ghCheck)

	allOK := true
	for _, ch := range checks {
		mark := "✅"
		if !ch.ok {
			mark = "❌"
			if ch.required {
				allOK = false
			}
		}
		fmt.Printf("%s %s\n", mark, ch.label)
		if !ch.ok && ch.hint != "" {
			fmt.Printf("   -> %s\n", ch.hint)
		}
	}

	fmt.Println("\n注記: JavaとForgeはここではチェックしません。`mcsync start`/`setup`実行時にmcsyncが自動で" +
		"ダウンロード・管理します(ユーザープロファイル配下にキャッシュ)。")

	if !allOK {
		return fmt.Errorf("必須のチェック項目に問題があります")
	}
	fmt.Println("問題ありません。")
	return nil
}
