# mcsync

自宅Forge Minecraftサーバーの**ワールドデータ・設定を複数PC間でgit同期**するための、薄いCLIラッパーです。

- Forgeのダウンロード・インストール、EULA同意、mod自動ダウンロードは全部
  [itzg/docker-minecraft-server](https://github.com/itzg/docker-minecraft-server) というDockerイメージに丸投げします。
- mcsync自身がやるのは「`git`コマンド」と「`docker compose`コマンド」を、決まった順番で毎回タイプする手間を省くことだけです。
- 独自の設定ファイル形式はありません。**`docker-compose.yml`そのものがマニフェスト**(MC/Forgeバージョン・mod一覧を書く場所)です。

## 1. 必要なもの(各PCにインストール)

mcsyncを使う全てのPCに、以下2つが必要です。

### Docker Desktop

Minecraftサーバー本体(Forge、Java、mod)は全てDockerコンテナの中で動きます。ホストPCにJavaやForgeを直接インストールする必要はありません。

1. https://www.docker.com/products/docker-desktop/ からダウンロードしてインストール
2. インストール後、**Docker Desktopアプリを起動しておく**(常駐させる。タスクトレイにクジラのアイコンが出ていればOK)
3. 確認: ターミナルで `docker --version` と `docker compose version` が通ればOK

Docker Desktopを起動していないと`docker`コマンドは「デーモンに繋がらない」エラーになります。mcsyncを使う前は毎回Docker Desktopが起動しているか確認してください。

### Git

ワールドデータ・設定のバージョン管理と、PC間の同期(push/pull)に使います。

1. https://git-scm.com/downloads からインストール
2. 初回のみ、コミット用に名前とメールアドレスを設定しておく:
   ```
   git config --global user.name "あなたの名前"
   git config --global user.email "you@example.com"
   ```

さらに、複数PC間で同期するには**GitHubなどのリモートリポジトリ**が必要です(GitHubで空のリポジトリを1つ作成し、そのURLを`mcsync init`実行時に渡します)。

### Git LFS

mod jarファイル(`data/mods`)を直接git管理するために使います。数十MB単位になりがちなjarを普通のgitでそのまま管理すると、更新するたびに古いバージョンも含めてリポジトリがどんどん肥大化してしまうため、大きいバイナリを差分管理できるGit LFSという拡張を使います。

1. https://git-lfs.com からインストール(またはwinget: `winget install GitHub.GitLFS`)
2. 確認: ターミナルで `git lfs version` が通ればOK

`mcsync init`/`mcsync setup`が、その都度必要な設定(`git lfs install`)を自動でやってくれるので、インストールさえしておけば手動操作は不要です。**インストールを忘れると**、`mcsync setup`で他のPCのプロジェクトをcloneした時に、mod jarが中身の入っていない小さなプレースホルダーファイルのまま(＝実際にはダウンロードされない)になってしまうので注意してください。`mcsync doctor`で確認できます。

**参考:** GitHubのLFSストレージ/帯域は無料枠だと1GBまでです(超えると追加購入が必要)。個人・友人内の利用規模であれば通常は問題になりませんが、大きいmodを大量に入れ替えるような使い方をする場合は、GitHub側の[Git LFSの料金体系](https://docs.github.com/repositories/working-with-files/managing-large-files/about-storage-and-bandwidth-usage)を一度確認しておくと安心です。

## 2. mcsyncのインストール

mcsyncはGo製のシングルバイナリです。このリポジトリ(`mcsync`フォルダ)自体をビルドして使います。

### ビルドする(初回・Go更新時のみ)

Goが必要です(未インストールなら https://go.dev/dl/ から、またはwinget: `winget install GoLang.Go`)。

```
cd c:\Users\skybl\dev\work\mcsync
go build -o mcsync.exe .
```

`mcsync.exe`が生成されます。これを使いたい場所(サーバー用フォルダなど)にコピーするか、PATHが通ったフォルダに置いてください。

以降、ソースコードを変更しない限り再ビルドは不要です。`mcsync.exe`単体をコピーするだけで他のPCでも動きます(そのPCにもDocker/Gitは必要ですが、Goは不要です)。

### PowerShellで `mcsync` とだけ打って実行できるようにする

PowerShellはセキュリティ上、**カレントフォルダのコマンドをそのままの名前では実行しません**。何もしないと、`mcsync.exe`があるフォルダにいても以下のようなエラーになります。

```
mcsync : 用語 'mcsync' は、コマンドレット、関数、スクリプト ファイル、または操作可能な
プログラムの名前として認識されません。
```

対処法は2つあります。

- **その場しのぎ**: 先頭に`.\`を付けて `.\mcsync.exe init` のように実行する
- **恒久対応(推奨)**: `mcsync.exe`があるフォルダをPATH環境変数に登録する。一度登録すれば、以降はどのフォルダからでも`mcsync`とだけ打てば動く

恒久対応は以下のPowerShellコマンドで設定できます(パスは自分の環境に合わせて変更):

```powershell
$mcsyncDir = "C:\Users\skybl\dev\work\mcsync"
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
[Environment]::SetEnvironmentVariable("Path", "$userPath;$mcsyncDir", "User")
```

**注意:** 設定を反映させるには、**今開いているPowerShellウィンドウを一度閉じて、新しく開き直す**必要があります(開いたままのウィンドウにはPATHの変更が反映されません)。

## 3. 全体の流れ(重要な考え方)

1. **1台目のPC**で `mcsync init` → サーバー用のgitリポジトリ(docker-compose.ymlを含む)を作りGitHubにpush
2. **2台目以降のPC**で `mcsync setup <リポジトリURL>` → clone + サーバー起動
3. 遊ぶ前に `mcsync start`(他PCの変更を取り込んでから起動)
4. 遊び終わったら `mcsync stop`(サーバー停止→ワールド・設定をcommit・push)
5. modを追加したくなったら `mcsync mods add <URL>`

**鉄則: 同時に2台以上のPCでサーバーを起動しない。** ワールドデータはgitで同期しますが、2台で同時編集すると上書き事故(マージ不可能)になります。遊び終わったら必ず`mcsync stop`してから別PCに移ってください。

## 4. コマンド一覧

### `mcsync doctor`

git/Dockerが正しくインストールされ、Docker Desktopが起動しているかをチェックします。何か動かない時はまずこれを実行してください。

```
mcsync doctor
```

```
✅ git installed
✅ docker installed
✅ git-lfs installed
❌ docker daemon running
   -> start Docker Desktop and wait for it to finish starting
✅ docker compose available
```

### `mcsync init`

新しいサーバープロジェクトを作ります(**最初の1台目のPCで1回だけ**実行)。

```
mkdir my-server
cd my-server
mcsync.exe init
```

対話形式でいくつか聞かれます(Enterでデフォルト値):

| 質問 | 説明 |
|---|---|
| Project name | サーバーの識別名(フォルダ名がデフォルト) |
| Minecraft version | 例: `1.20.1` |
| Forge version | 空欄でEnterすると、そのMCバージョンのForge公式サイトから**推奨版を自動取得**します |
| Memory | サーバーに割り当てるメモリ。例: `4G` |
| Git remote URL | GitHubなどで先に作っておいた空リポジトリのURL(`https://github.com/you/my-server.git`など)。空欄なら後で手動で`git remote add`してもOK |

実行すると:
- `docker-compose.yml` と `.gitignore` が生成される
- `git init` → 生成した2ファイルをcommit
- remote URLを入力していれば `git remote add origin` → `git push`

対話なしで一気に作りたい場合はフラグでも指定できます:

```
mcsync.exe init --name my-server --mc-version 1.20.1 --memory 4G --remote https://github.com/you/my-server.git
```

初回起動は次のどちらかで:

```
mcsync.exe start
```

初回はForge本体とmodのダウンロードが走るので、数分かかります。進捗はDocker Desktopのログ画面、または `docker compose logs -f mc` で確認できます。

### `mcsync setup <git-url> [フォルダ名]`

**2台目以降のPC**で、既存のサーバープロジェクトを持ってきて起動します。

```
mcsync.exe setup https://github.com/you/my-server.git
```

これだけで:
1. リポジトリをclone(フォルダ名省略時はリポジトリ名を使用)
2. Docker/gitの状態チェック
3. `docker compose up -d` でサーバー起動
4. サーバーが完全に起動する(healthyになる)まで、ログを画面に流しながら待つ

これが設計上の「2コマンドで再開」の1つ目です(1つ目=`setup`、以降は`start`/`stop`の繰り返し)。起動完了の待ち方については`mcsync start`の項も参照してください。

### `mcsync mods add <URLまたはスラッグ>`

modを追加します。ModrinthまたはCurseForgeのページURLをそのまま渡せます。

```
mcsync.exe mods add https://modrinth.com/mod/jei
mcsync.exe mods add https://www.curseforge.com/minecraft/mc-mods/jei
mcsync.exe mods add sophisticated-backpacks
```

内部では`docker-compose.yml`の`environment.MODRINTH_PROJECTS`(または`CURSEFORGE_FILES`)にmodのスラッグを1行追記し、自動でcommit・push(リモート設定済みなら)します。テキスト置換ではなくYAMLパーサーで安全に編集するので、手で書いた他の設定やコメントは壊れません。

反映(実際にmodをダウンロードさせる)には再起動が必要です:

```
mcsync.exe start
```

もしくは `mcsync.exe mods add <URL> --apply` を付けると、追加直後に`docker compose up -d`まで自動実行します。

**注意:** CurseForgeのmodはCurseForge側のAPI仕様により、ホスト側に`CF_API_KEY`という環境変数の設定が別途必要です(itzgイメージのドキュメント参照)。ModrinthのmodにはAPIキーは不要です。迷ったらModrinthを優先してください。

バラのスラッグ(URLでなく`jei`のような文字列)を渡した場合はデフォルトでModrinth扱いになります。CurseForgeのスラッグを渡したい場合は `--type curseforge` を付けてください。

### Modrinth/CurseForgeに無いmod(自作・個人配布のjar)を追加する

ModrinthにもCurseForgeにも無いmod(自作のもの、知人から直接もらったjarなど)は`mods add`では追加できません。その場合は、**`data/mods`フォルダにjarファイルをそのまま置いてください。**

```
my-server/
  docker-compose.yml
  data/
    mods/              <- ここに直接jarを置く
      my-custom-mod.jar
```

`data`フォルダは基本的に.gitignoreの対象(下記参照)ですが、`data/mods`だけは例外的に**git管理対象**になっており、かつ`.gitattributes`により**Git LFS**で追跡されます(`init`で新規作成したプロジェクトには最初から設定済みです)。ファイルを置いたら:

```
mcsync.exe stop
```

を実行すれば(サーバーを止める・止めないに関わらず、変更があれば)自動でcommit・pushされ、他のPCで`mcsync start`すればそのmodも一緒に反映されます。Git LFSのおかげで、jarを更新してもリポジトリの履歴が更新差分だけで済み、素のgitで管理するより肥大化しにくくなっています。

**注意:**
- Git LFSが入っていないPCで`mcsync setup`すると、jarが中身の無いプレースホルダーのままになります(`mcsync doctor`で事前確認してください)。
- `MODRINTH_PROJECTS`/`CURSEFORGE_FILES`経由でダウンロードされるmod(`mcsync mods add`で追加したもの)も同じ`data/mods`フォルダに入りますが、これらは`docker-compose.yml`の記述から再現できるので、二重にgit管理されるとリポジトリが無駄に膨らみます。基本的にはどちらか一方の方法(URL経由 or 手動配置)にmodごとに統一するのがおすすめです。

### `mcsync start`

遊ぶ前に実行します。

```
mcsync.exe start
```

1. `git pull --ff-only`(他PCで保存された最新のワールド・設定を取り込む)
2. (リモート設定済みなら)ローカルのGit LFSキャッシュを掃除(下記参照)
3. `docker compose up -d`(サーバー起動)
4. サーバーが完全に起動する(healthyになる)まで、ログを画面に流しながら待つ

pullでコンフリクト(マージ不能)が起きた場合はエラーで止まります。これは大抵「stopし忘れたまま別PCで遊んでしまった」ケースなので、慌てず手動でgitの状態を確認してください。

#### 起動の進捗表示

`start`実行中はサーバーのログがそのまま画面に流れ、「healthy」(起動完了)と判定されるまで自動で待ちます。ログの出力が20秒以上止まっている間は `... still starting (Xs elapsed)` と経過時間を表示するので、固まっているのか単に時間がかかっているだけなのか区別できます。完了すると

```
Server is up. Connect with localhost in Minecraft's multiplayer screen.
```

と表示されてコマンドが戻ってきます。待たずにバックグラウンドで起動だけさせたい場合は `mcsync.exe start --no-wait` を使ってください。

#### 初回だけ時間がかかる仕組みと、なぜ毎回は再ダウンロードしないか

Forge本体とmodのダウンロード・インストールは、**そのPC・そのプロジェクトフォルダで最初に起動した1回だけ**発生します(初回は5〜20分程度、回線速度による)。ダウンロードしたForge本体・mod jar・ライブラリは`./data`フォルダに保存され、`mcsync stop`(`docker compose down`)しても`data`フォルダ自体は消えないので、**2回目以降の`mcsync start`は数十秒程度で起動します**(このドキュメント作成時の実機テストでは約50秒でした)。

再ダウンロードが発生するのは以下のようなケースだけです:
- 同じプロジェクトを**別の新しいPC**で`mcsync setup`する(そのPCの`data`フォルダはまだ空なので)
- `docker-compose.yml`の`FORGE_VERSION`やmod一覧を変更した(変更差分だけ追加でダウンロードされます)
- `data`フォルダ自体を手動で削除した

**もっと速くする方法について:** 「Forge/modを毎回インストールし直すのではなく、セットアップ済みの状態そのものを配布する」というアイデア(Dockerイメージとして事前ビルドしてどこかに置いておき、各PCはそれをpullするだけにする)も技術的には可能です。ただしこれをやるには、mod構成を変えるたびに独自にイメージをビルドして置き場所(Docker Hub等のコンテナレジストリ)にpushし直す運用が必要になり、「Forge/modの面倒は全部itzgイメージに任せる」というmcsyncの設計方針から外れて管理対象が増えます。個人・友人内での利用では、上記の「初回だけ待てば、あとは各PCとも数十秒」という現状の方が運用はシンプルです。もし本当に必要であれば追加実装できるので、気になる場合は相談してください。

#### Git LFSキャッシュの自動クリーンアップ

`data/mods`のjarを何度も追加・更新していると、`.git/lfs/objects`(ローカルのLFSキャッシュ)に過去バージョンのjarが積み上がり、`.git`フォルダが数GB単位に肥大化することがあります。これを防ぐため、**`mcsync start`と`mcsync stop`は、リモートへのpull/push成功後に自動で`git lfs prune`を実行**します。

```
Cleaning up local Git LFS cache...
prune: 2 local objects, 1 retained, done.
prune: Deleting objects: 100% (1/1), done.
```

`git lfs prune`はgit-lfs公式のメンテナンスコマンドで、**リモートに既にpush済みと確認できたオブジェクトのうち、最近の履歴から参照されていない古いもの**だけを安全に削除します(まだリモートに無い、または直近で使われているオブジェクトは消えません)。そのため毎回自動実行しても安全です。手動で今すぐ掃除したい場合は `git lfs prune` を直接実行しても構いません。

git-lfsが入っていない、またはリモートが未設定のプロジェクトではこのクリーンアップはスキップされます(失敗しても`start`/`stop`自体は止まりません)。

### `mcsync stop`

遊び終わったら実行します。

```
mcsync.exe stop
```

1. `docker compose down`(サーバー停止)
2. ワールド(`data/world`)・設定(`data/config`, `server.properties`, `ops.json`, `whitelist.json`)・mod(`data/mods`)をcommit
3. リモートが設定されていれば`git push`、続けてローカルのGit LFSキャッシュを自動クリーンアップ(上記参照)

**遊び終わったら他のことをする前に必ずこれを実行する**のがルールです。stopし忘れると次に別PCで`start`した時に他PCの変更を取り込めず、データが古いまま上書きされる恐れがあります。

## 5. docker-compose.yml の中身

`init`で生成される内容の例:

```yaml
name: my-server

services:
  mc:
    image: itzg/minecraft-server:latest
    environment:
      EULA: "true"
      TYPE: "FORGE"
      VERSION: "1.20.1"
      FORGE_VERSION: "47.4.10"
      MEMORY: "4G"
      MODRINTH_PROJECTS: |
        jei
        sophisticated-backpacks
    ports:
      - "25565:25565"
    volumes:
      - ./data:/data
    stdin_open: true
    tty: true
```

このファイルこそが「サーバーの設計図」です。MC/Forgeバージョンやmod一覧を変えたい時は、このファイルの内容(または`mcsync mods add`)を変えてcommitすれば、他のPCでも`mcsync start`するだけで同じ状態が再現されます。手でこのファイルを直接編集しても構いませんが、mod一覧の追記は書式ミスを避けるため`mcsync mods add`の利用を推奨します。

サーバーの全データ(ワールド、mod jar、ログなど)は `./data` フォルダ以下にDockerが作ります。このフォルダは.gitignoreで「必要なものだけ追跡」する設定になっています(下記参照)。`data/mods`だけは例外的にGit LFS経由で追跡され、自作・個人配布のmodを直接置く場所として使えます(詳細は上の「Modrinth/CurseForgeに無いmodを追加する」を参照)。

## 6. .gitignore の考え方

```
data/*
!data/world/
!data/world/**
!data/config/
!data/config/**
!data/server.properties
!data/ops.json
!data/whitelist.json
!data/mods/
!data/mods/**
data/libraries/
data/logs/
data/cache/
data/eula.txt
data/*.log
```

- **追跡する(gitで同期する)**: `data/world`(ワールドデータ本体)、`data/config`(mod設定)、`server.properties`、`ops.json`、`whitelist.json`、`data/mods`(mod jar、Git LFS経由) — これらは「プレイヤーの進行状況」や「使っているmod」そのものなので同期が必要
- **追跡しない**: Forge本体・ライブラリ・ログ・`eula.txt` — これらは`docker-compose.yml`の内容とitzgイメージから**毎回自動的に再生成される**ので、gitで運ぶ必要がありません。

## 7. トラブルシューティング

| 症状 | 原因・対処 |
|---|---|
| `docker`系コマンドで「デーモンに繋がらない」エラー | Docker Desktopを起動していない。タスクトレイのクジラアイコンを確認し、起動を待ってから再実行 |
| `mcsync start`が`git pull`でコンフリクトエラー | 別PCで`stop`せずに終了した、または2台で同時に起動していた可能性。`git status`で状態を確認し、必要ならどちらのワールドを残すか手動で判断してマージ |
| `mods add`で「docker-compose.ymlが見つからない」 | プロジェクトのルートフォルダ(docker-compose.ymlがある場所)で実行しているか確認 |
| 初回`start`後、サーバーに繋がらない | Forge本体のダウンロード・インストール中の可能性。`docker compose logs -f mc`でログを見て`Done`が出るまで待つ |
| `data/mods`のjarが数百バイトしかない/サーバーがmodエラーで落ちる | Git LFSが未インストールのままcloneした可能性。`git lfs install`してから`git lfs pull`でLFSの実体を取得し直す(以後は`mcsync doctor`で事前確認を) |
| ルーター越しに友人を招待したい | mcsyncの範囲外です。ルーターのポートフォワーディング(25565/TCP)や、Tailscale等のVPNを別途設定してください |

困ったら、まず `mcsync doctor` を実行してください。

## 8. ソースコード構成(開発者向け)

```
mcsync/
  main.go                        エントリーポイント
  cmd/                            各サブコマンドの実装(cobra)
    root.go                       ルートコマンド定義
    init.go                       `mcsync init`
    setup.go                      `mcsync setup`
    mods.go                       `mcsync mods add`
    start.go                      `mcsync start`
    stop.go                       `mcsync stop`
    doctor.go                     `mcsync doctor`
  internal/
    gitutil/                      gitをサブプロセスとして呼ぶだけのラッパー
    dockerutil/                   docker / docker composeをサブプロセスとして呼ぶだけのラッパー
    forgeversion/                 Forge公式のpromotions_slim.jsonから推奨バージョンを取得
    scaffold/                     `init`時に生成するdocker-compose.yml/.gitignoreのテンプレート
    composeedit/                  既存docker-compose.ymlをyaml.Node経由で安全に差分編集(mods add用)
    modurl/                       Modrinth/CurseForgeのURLからmodスラッグを抽出
    prompt/                       `init`の対話入力ヘルパー
```

設計方針は一貫して「gitとdocker composeの薄いラッパーであること」です。Forgeのインストールやmodのダウンロードロジックはmcsync側には一切存在せず、全てitzg/docker-minecraft-serverイメージに任せています。
