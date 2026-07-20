package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"mcsync/internal/forgeversion"
	"mcsync/internal/ghutil"
	"mcsync/internal/gitutil"
	"mcsync/internal/manifest"
	"mcsync/internal/prompt"
	"mcsync/internal/scaffold"
)

var (
	initName         string
	initMCVersion    string
	initForgeVersion string
	initMemory       string
	initRemote       string
	initDir          string
	initForce        bool
	initCreateRepo   string
	initRepoPublic   bool
	initSSHHost      string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "新しいmcsyncプロジェクトを作成する(mcsync.yml + .gitignore + gitリポジトリ)",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().StringVar(&initName, "name", "", "プロジェクト名")
	initCmd.Flags().StringVar(&initMCVersion, "mc-version", "", "Minecraftバージョン(例: 1.20.1)")
	initCmd.Flags().StringVar(&initForgeVersion, "forge-version", "", "Forgeバージョン(空欄なら推奨版を自動選択)")
	initCmd.Flags().StringVar(&initMemory, "memory", "", "サーバーに割り当てるメモリ量(例: 4G)")
	initCmd.Flags().StringVar(&initRemote, "remote", "", "既存のGitリモートURL(指定時はリポジトリ自動作成をスキップ)")
	initCmd.Flags().StringVar(&initCreateRepo, "create-repo", "", "この名前でGitHubリポジトリを新規作成する(gh CLIが必要)")
	initCmd.Flags().BoolVar(&initRepoPublic, "repo-public", false, "--create-repoで作成するリポジトリをpublicにする(デフォルトはprivate)")
	initCmd.Flags().StringVar(&initSSHHost, "ssh-host", "", "リモートURLに使うSSH Hostエイリアス(例: github.com-work。空欄ならgithub.com)")
	initCmd.Flags().StringVar(&initDir, "dir", ".", "プロジェクトを作成するディレクトリ")
	initCmd.Flags().BoolVar(&initForce, "force", false, "既存のmcsync.ymlを上書きする")
	rootCmd.AddCommand(initCmd)
}

func runInit(c *cobra.Command, args []string) error {
	dir, err := filepath.Abs(initDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%s の作成に失敗しました: %w", dir, err)
	}

	manifestPath := filepath.Join(dir, manifest.FileName)
	if _, err := os.Stat(manifestPath); err == nil && !initForce {
		return fmt.Errorf("%s は既に存在します(上書きするには --force を指定してください)", manifestPath)
	}
	gitignorePath := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignorePath); err == nil && !initForce {
		return fmt.Errorf("%s は既に存在します(上書きするには --force を指定してください)", gitignorePath)
	}

	if initName == "" {
		initName = prompt.Ask("プロジェクト名", filepath.Base(dir))
	}
	if initMCVersion == "" {
		initMCVersion = prompt.Ask("Minecraftバージョン", "1.20.1")
	}
	if initForgeVersion == "" {
		initForgeVersion = prompt.Ask("Forgeバージョン(空欄で推奨版を自動選択)", "")
	}
	if initForgeVersion == "" {
		fmt.Printf("Minecraft %s の推奨Forgeバージョンを確認しています...\n", initMCVersion)
		v, err := forgeversion.Recommended(initMCVersion)
		if err != nil {
			return fmt.Errorf("Forgeバージョンの自動選択に失敗しました: %w (--forge-version で手動指定してください)", err)
		}
		initForgeVersion = v
		fmt.Printf("Forge %s を使用します\n", initForgeVersion)
	}
	if initMemory == "" {
		initMemory = prompt.Ask("メモリ量", "4G")
	}

	remoteURL, err := resolveRemoteURL(initName)
	if err != nil {
		return err
	}

	projectName := scaffold.SanitizeProjectName(initName)
	manifestContent, err := scaffold.RenderManifest(scaffold.ManifestData{
		ProjectName:  projectName,
		MCVersion:    initMCVersion,
		ForgeVersion: initForgeVersion,
		Memory:       initMemory,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0o644); err != nil {
		return fmt.Errorf("%s の書き込みに失敗しました: %w", manifestPath, err)
	}
	if err := os.WriteFile(gitignorePath, []byte(scaffold.Gitignore), 0o644); err != nil {
		return fmt.Errorf("%s の書き込みに失敗しました: %w", gitignorePath, err)
	}
	fmt.Printf("%s と %s を作成しました\n", manifestPath, gitignorePath)

	if !gitutil.IsInstalled() {
		fmt.Println("gitがインストールされていないため、リポジトリのセットアップをスキップします。詳細は `mcsync doctor` を実行してください。")
		return nil
	}

	if !gitutil.IsRepo(dir) {
		if err := gitutil.Run(dir, "init", "-b", "main"); err != nil {
			return err
		}
	}

	if gitutil.LFSInstalled() {
		if err := gitutil.LFSInstall(dir); err != nil {
			fmt.Printf("警告: `git lfs install` に失敗しました: %v\n", err)
		} else if err := gitutil.LFSTrack(dir, scaffold.ModsLFSPattern); err != nil {
			fmt.Printf("警告: `git lfs track` に失敗しました: %v\n", err)
		} else {
			fmt.Printf("data/mods 配下のmod jarはGit LFSで追跡されます(%s)\n", scaffold.ModsLFSPattern)
		}
	} else {
		fmt.Printf("注意: git-lfsがインストールされていないため、data/modsに置いたmod jarは通常の"+
			"(LFSでない)gitオブジェクトとして追跡されます。動作はしますが、大きい・頻繁に更新されるjarでは"+
			"リポジトリが肥大化しやすくなります。git-lfs(https://git-lfs.com)をインストール後、"+
			"`git lfs track \"%s\"` を実行すれば切り替えられます。\n", scaffold.ModsLFSPattern)
	}

	addArgs := []string{"add", manifest.FileName, ".gitignore"}
	if _, err := os.Stat(filepath.Join(dir, ".gitattributes")); err == nil {
		addArgs = append(addArgs, ".gitattributes")
	}
	if err := gitutil.Run(dir, addArgs...); err != nil {
		return err
	}
	changed, err := gitutil.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if changed {
		if err := gitutil.Run(dir, "commit", "-m", "Initial commit: mcsync project setup"); err != nil {
			return err
		}
	} else {
		fmt.Println("commitする新しい変更はありません。")
	}

	if remoteURL != "" {
		if !gitutil.HasRemote(dir, "origin") {
			if err := gitutil.Run(dir, "remote", "add", "origin", remoteURL); err != nil {
				return err
			}
		}
		branch := gitutil.CurrentBranch(dir)
		if branch == "" {
			branch = "main"
		}
		if err := gitutil.Run(dir, "push", "-u", "origin", branch); err != nil {
			return fmt.Errorf("%s へのpushに失敗しました: %w (後で手動でpushしてください)", remoteURL, err)
		}
	}

	fmt.Println("\n完了しました。次は `mcsync start` でサーバーを起動してください" +
		"(初回はJavaとForgeの自動ダウンロードが走るため数分かかります)。")
	return nil
}

// resolveRemoteURL determines the git remote URL to use, either from an
// existing repository URL the user provides or by creating a brand new
// GitHub repository via gh and constructing its URL ourselves (so a
// custom SSH config Host alias, e.g. "github.com-work" for juggling
// multiple GitHub accounts, is honored -- gh itself is only ever used to
// create the repository, never to touch the local git remote).
func resolveRemoteURL(projectName string) (string, error) {
	if initRemote != "" {
		return initRemote, nil
	}
	if initCreateRepo != "" {
		return createGitHubRepo(initCreateRepo, !initRepoPublic, initSSHHost)
	}

	if !prompt.Confirm("GitHubリポジトリは既に作成済みですか？", false) {
		if !ghutil.IsInstalled() {
			fmt.Println("注意: gh (GitHub CLI) が見つからないため、リポジトリの自動作成はスキップします。" +
				"https://cli.github.com/ からインストールすると次回から自動作成できます。")
			return prompt.Ask("GitリモートURL(空欄でスキップ)", ""), nil
		}
		account, ok := ghutil.CurrentAccount()
		if !ok {
			fmt.Println("注意: gh が未認証のため、リポジトリの自動作成はスキップします。" +
				"`gh auth login` を実行してから再度お試しください。")
			return prompt.Ask("GitリモートURL(空欄でスキップ)", ""), nil
		}
		if !prompt.Confirm(fmt.Sprintf("GitHubアカウント \"%s\" で作成します。よろしいですか？", account), true) {
			return "", fmt.Errorf("`gh auth switch` でアカウントを切り替えてから再実行してください")
		}
		name := prompt.Ask("リポジトリ名", scaffold.SanitizeProjectName(projectName))
		visibility := prompt.Ask("公開設定 (public/private)", "private")
		private := !strings.EqualFold(strings.TrimSpace(visibility), "public")
		sshHost := prompt.Ask("SSH Hostエイリアス(空欄 = github.com、例: github.com-work)", "")
		return createGitHubRepo(name, private, sshHost)
	}

	return prompt.Ask("GitリモートURL(空欄でスキップ)", ""), nil
}

func createGitHubRepo(name string, private bool, sshHost string) (string, error) {
	if !ghutil.IsInstalled() {
		return "", fmt.Errorf("gh (GitHub CLI) がインストールされていません。https://cli.github.com/ からインストールしてください")
	}
	visibility := "private"
	if !private {
		visibility = "public"
	}
	fmt.Printf("GitHubリポジトリ %s を作成しています(%s)...\n", name, visibility)
	owner, repo, err := ghutil.CreateRepo(name, private)
	if err != nil {
		return "", fmt.Errorf("GitHubリポジトリの作成に失敗しました: %w", err)
	}
	if sshHost == "" {
		sshHost = "github.com"
	}
	url := fmt.Sprintf("git@%s:%s/%s.git", sshHost, owner, repo)
	fmt.Printf("作成しました: %s\n", url)
	return url, nil
}
