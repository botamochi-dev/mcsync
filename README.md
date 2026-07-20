# mcsync

自宅Forge Minecraftサーバーの**ワールドデータ・設定を複数PC間でgit同期**するための、薄いCLIラッパーです。

- Forgeサーバーは**Dockerを使わず、ホストPC上に直接**構築・実行します。Java・Forgeのダウンロード/インストール/EULA同意はmcsyncが自動でやります。
- mcsync自身がやるのは「Java/Forgeの用意」「サーバープロセスの起動・停止」「`git`コマンドを決まった順番で毎回タイプする手間を省くこと」です。
- 設定ファイルは**`mcsync.yml`**(MC/Forgeバージョン・メモリ量を書く場所)だけです。

## 必要環境

各PCに以下が必要です。**Docker・Java・Forgeは不要**(mcsyncが自動で用意します)。

- **[Git](https://git-scm.com/downloads)**: ワールド・設定のバージョン管理とPC間同期に使用。初回のみ`git config --global user.name/user.email`を設定してください。複数PCで同期するにはGitHub等のリモートリポジトリも必要です。
- **[Git LFS](https://git-lfs.com)**(`winget install GitHub.GitLFS`): `data/mods`に置くmod jarを差分管理するために使用。未インストールだと他PCでmod jarが空のプレースホルダーになるので注意(`mcsync doctor`で確認可)。

Java・Forgeは`mcsync start`/`setup`実行時に自動でダウンロード・インストールされます(初回のみ時間がかかります)。

### mcsync自体のインストール

Go製のシングルバイナリです。ビルドして使います(Go: https://go.dev/dl/ または `winget install GoLang.Go`)。

```
git clone <このリポジトリのURL>
cd mcsync
go build -o mcsync.exe .
```

生成された`mcsync.exe`をPATHの通ったフォルダに置けば、以後どこからでも`mcsync`とだけ打って実行できます(PowerShellでは先頭に`.\`を付けて`.\mcsync.exe`と打つか、フォルダをPATH環境変数に追加してください)。

## サーバーを新規作成する

**最初の1台目のPCで1回だけ**実行します。

```
mkdir my-server
cd my-server
mcsync init
```

対話形式でプロジェクト名・Minecraftバージョン・Forgeバージョン(空欄で推奨版を自動取得)・メモリ量を聞かれたあと、Gitリモートの設定方法を選べます:

- **既にGitHubリポジトリを作成済み**: URLをそのまま入力
- **まだ作成していない**: `gh`(GitHub CLI、要`gh auth login`)を使って新規リポジトリを自動作成できます。リポジトリ名・公開設定(public/private、デフォルトprivate)を聞かれたあと、GitHub上にリポジトリを作成し、そのままリモートに追加します

実行すると`mcsync.yml`と`.gitignore`が生成され、gitリポジトリが初期化・commit・pushされます。

対話なしでフラグ指定も可能です:

```
mcsync init --name my-server --mc-version 1.20.1 --memory 4G --remote https://github.com/you/my-server.git
mcsync init --name my-server --mc-version 1.20.1 --memory 4G --create-repo my-server --repo-public
```

複数のGitHubアカウントをSSHの`~/.ssh/config`のHostエイリアス(例: `github.com-work`)で使い分けている場合は、`--ssh-host github.com-work`を指定すると`git@github.com-work:owner/repo.git`の形でリモートを追加します(対話モードでも同様に聞かれます)。

作成後、`mcsync start`で初回起動します(Java/Forgeのダウンロードが走るため数分かかります)。

## サーバーの起動・停止

```
mcsync start   # git pull → 起動 → 起動完了まで待機
mcsync stop    # 停止(ワールド保存)→ commit → push
```

- `start`は他PCの変更をpullしてから起動し、起動完了(ワールドロード終了)までログを表示しながら待ちます。バックグラウンドで起動だけさせたい場合は`mcsync start --no-wait`。
- `stop`はサーバーを安全に停止させたあと、ワールド・設定・modの変更を自動でcommit・push します。**遊び終わったら必ず実行してください。**
- **同時に2台以上のPCでサーバーを起動しないでください。** 上書き事故の原因になります。`start`は簡易ロック機構で他PCの起動中を検知して警告しますが、保険程度と考え、運用ルールとしても徹底してください。

## 別のPCで開発を始める

**2台目以降のPC**で、既存のサーバープロジェクトをcloneしてそのまま起動します。

```
mcsync setup https://github.com/you/my-server.git
```

clone→Java/Forgeの用意→起動→起動完了待ちまで自動で行われます。以降はそのフォルダで`mcsync start`/`stop`を使います。

## その他のよく使うコマンド

- **mod追加**: `data/mods`フォルダに自分でダウンロードしたjarを置くだけです。`mcsync stop`時に自動でcommit・pushされます(Modrinth/CurseForgeからの自動ダウンロード機能はありません)。
- **`mcsync status`**: サーバーの起動状態・メモリ使用量、gitの同期状態、ディスク使用量を一目で確認できます。困ったときはまずこれ。
- **`mcsync restore [commit]`**: ワールドを過去のsave状態に戻します。引数なしで直近のsave履歴を一覧表示、commitハッシュを指定すると復元(確認プロンプトあり、`-y`で省略可)。
- **`mcsync autosave`**: サーバーを止めずに定期的にセーブ・pushします(デフォルト15分おき、`--interval`で変更、Ctrl+Cで停止)。
- **`mcsync doctor`**: Git/Git LFS(とオプションのgh)が正しく入っているかチェックします。

## コマンド一覧

| コマンド | 説明 |
|---|---|
| `mcsync init [--create-repo <名前> --repo-public --ssh-host <host>]` | 新規サーバープロジェクトを作成(任意でGitHubリポジトリも自動作成) |
| `mcsync setup <git-url> [フォルダ名]` | 既存プロジェクトをclone・起動 |
| `mcsync start [--no-wait] [--force]` | git pull後にサーバー起動 |
| `mcsync stop` | サーバー停止・保存・push |
| `mcsync status` | 状態確認 |
| `mcsync restore [commit] [-y]` | ワールドを過去の状態に復元 |
| `mcsync autosave [--interval 5m]` | 止めずに定期セーブ |
| `mcsync doctor` | 環境(Git/Git LFS/gh)チェック |

## ファイル構成

新規作成したプロジェクトフォルダの中身:

```
my-server/
  mcsync.yml       サーバー設定(MC/Forgeバージョン、メモリ量)
  .gitignore
  data/            サーバーの全データ(ワールド・mod・Forge本体など)
    world/          ワールドデータ(git管理)
    config/         mod設定(git管理)
    mods/           modのjarファイル(git管理、Git LFS)
    server.properties, ops.json, whitelist.json (git管理)
  .mcsync/         このPCだけのローカル状態(PID・ログ等、git管理外)
```

`data`配下でgit管理されるのは上記のみで、Forge本体・ライブラリ・実行スクリプト・ログなどは`mcsync.yml`から毎回自動的に再生成されるため同期されません。

mcsync自体のソースコード構成:

```
mcsync/
  main.go             エントリーポイント
  cmd/                各サブコマンドの実装
  internal/
    gitutil/           git呼び出し
    ghutil/             gh(GitHub CLI)呼び出し(リポジトリ自動作成用)
    javautil/          Javaランタイムの自動取得
    forgeinstall/       Forgeサーバーの自動インストール
    serverproc/         サーバープロセスの起動・停止管理
    forgeversion/       Forge推奨バージョンの取得
    scaffold/           mcsync.yml/.gitignoreのテンプレート
    manifest/           mcsync.ymlの読み込み
    prompt/             対話入力ヘルパー
```
