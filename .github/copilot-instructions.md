# instructions

指示が曖昧な場合は編集せずに曖昧な箇所を指摘してください。

## ドキュメント

* `README.md` と `docs/*.md` は英語で記載し、markdown の書式警告は修正してください
* `README.md` または `docs/*.md` を更新したら、`skills/gh-copilot-attendant/SKILL.md` も更新してください
* コマンドの追加・削除や引数・オプションの変更時は、`docs/usage.md` を更新してください
* コマンドの追加・削除・引数変更時は、`README.md` も必ず更新してください

### README.md

* プロジェクトの概要、Installation、主要サブコマンドの簡潔な使用例、詳細ドキュメントへのリンクを記載します
* completion の詳細をヘルプに書かず、Installation の次のセクションに簡単な説明と詳細への案内を記載する
* 詳細な Usage は `docs/usage.md`、開発手順は `docs/development.md` に記載します

### docs/usage.md

* 各コマンドの Usage を記載します
* サブコマンドごとにグルーピングして記載します
* サブコマンドを持っているサブコマンドの説明は不要です
* コマンドヘルプの項目は項目のタイトルでソートして記述します
* コマンドの説明は下記コメントブロックの内容のように記述してください

<!--
#### コマンドの機能

```sh
コマンド列
```

コマンドの Long の説明
-->

* コマンドの Usage では、省略可能な引数やオプションを `[]`、選択肢を `|` で示し、デフォルト値や繰り返し可否も明記してください
* Usage例・説明文は日本語・英語の混在を避け、統一した言語で記載してください
* 実行例のコマンド列は実際に動作する形式で記載してください
* コマンドの説明文（Long）は簡潔かつ具体的に、何ができるか・どんなオプションがあるかを明記してください
* サブコマンドのグルーピング順は、`README.md` と `docs/usage.md` で一貫性を持たせてください
* `docs/usage.md` のコマンド説明は、実装と乖離しないよう定期的に見直してください

### docs/development.md

* ローカルチェックや開発用ツールの更新手順を記載します

## コーディング規約

* fmt.Errorf: error strings should not end with punctuation or newlines (ST1005) go-staticcheck
* gh extension 用開発の共通パッケージは github.com/srz-zumix/go-gh-extension/pkg/<path/to/dir> で import

### ソースコード全般

* ディレクトリ・ファイル構成は以下の責務分割に従うこと
  * cmd/: CLIコマンド定義・引数/フラグ処理・cobra.Command生成のみを担当し、ビジネスロジックは持たない
    * cmd/直下に主要コマンドを配置
    * cmd/のサブディレクトリに、各コマンドのサブコマンドを配置
    * サブディレクトリ内にもさらに細分化されたコマンドを配置する場合がある
  * go-gh-extension/pkg: 共通パッケージ群
    * actions/: GitHub Actions関連のユーティリティ
    * cmdflags/: コマンドラインフラグの共通処理
    * gh/: GitHub APIラッパー・ビジネスロジック層。API呼び出しはgh/client/配下で行い、gh/直下はラッパー・ユーティリティ関数のみ
      * gh/配下のラッパー関数は必ずctx context.Context, g *GitHubClientを先頭引数に取り、repository.Repository型等を利用する
    * gh/client/: go-github等の外部APIクライアント呼び出し専用。APIレスポンスの整形やエラーラップは行わない
    * logger/: ロギング関連
    * parser/: 入力値のパース・バリデーション等
    * render/: 表示用の整形・出力処理（テーブル/JSON/hovercard等）
  * config/: 設定ファイル関連
  * version/: バージョン情報管理
* importはローカルパッケージをgithub.com/srz-zumix/go-gh-extension/pkg/<path>で記述し、cmd/から直接github/client/やgo-githubをimportしない
* コマンド追加時はcmd/配下にcobra.Commandを返すNew<Cmd名>Cmd関数を新設し、親コマンドで登録する
* コメントは英語で記載し、関数・構造体・パッケージの責務が明確になるよう記述する
* エラーはcmd/では操作内容を含めてラップし、gh/client/ではラップせずそのまま返す
* テストコードは*_test.goで実装し、各責務ごとに配置する
  * cmd/ 配下にはテストを書きません。フラグのパースや排他制御は cobra/pflag が担保しているため、二重にテストしない
  * cmd/ にテストを書きたくなった場合は責務分割のシグナルです。検証したいロジックを pkg/ もしくは internal/ に切り出し、そちらでテストする
* テストデータには実データ（ユーザー名・ID・ハッシュ・組織名・メールアドレス等）を使用せず、ユーザー提供の情報や実際の API レスポンスも転用しない。検証に必要な形式・制約を満たす架空の値を使用する
* コード重複は避け、共通処理は関数化・ユーティリティ化する
* Lint/Formatter（go fmt, go vet, staticcheck等）を通してからコミットする
* 依存関係の循環(import cycle)が発生しないよう注意する

### パッケージ詳細

#### cmd

* 新しいコマンドを作成する場合は他のコマンドの実装を参照し、書き方など踏襲してください
* オプションは基本的に変数で受け取ります
* RunE で処理を実装します
* Args で引数の検証をします
* gh/*.go のラッパー関数を呼び出し、cmd package では github package を import しなくても良い設計にします
* エラーの場合はどういう操作をしようとしてエラーになったかメッセージに含めるようにしてください
* cmd/root.go は変更してはいけません
* cmd/**/*.go のサブコマンドは cobra.Command を return する関数を定義し、その中でコマンド実装してください
  * コマンドの登録は親コマンドの .go ファイルで行います
  * 関数名は New<コマンド名>Cmd としてください。例えば list コマンドであれば NewListCmd 、add コマンドであれば NewAddCmd となります
* owner もしくは repo オプションはオプショナルです。文字列が空の場合の処理は不要です
* cmd package のテストは書きません。オプションの検証はライブラリ側で担保されているものとして扱います
  * フラグ値の妥当性判定や API 呼び出し前の事前検証など、ライブラリが担保しない判断は RunE に置かず pkg/ もしくは internal/ の関数に切り出してください
* サブディレクトリ構成例:
  * cmd/member/add.go, cmd/member/list.go, cmd/member/role.go など
  * cmd/org/role/list.go などの多階層構成も可

#### go-gh-extension/pkg/gh

* gh/client/*.go では API 呼び出しのエラーはフォーマットせずそのまま返します
* gh/*.go には github/client/*.go の関数のラッパーを記述します
  * owner/repo などの string は使わず repository.Repository 型を引数に取ります
  * ラッパー関数は ctx context.Context を第一、 g *GitHubClient を第二引数に取ります
