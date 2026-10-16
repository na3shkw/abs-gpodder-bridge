# 隠蔽テストハーネス

実装エージェント（Claude Code）からテストを隠したまま、テストを実行して結果だけを返す仕組み。

- 実装リポジトリ：`../abs-gpodder-bridge`（`IMPL_DIR` で変更できる）
- このリポジトリ：テストとハーネスだけを置く。**公開リポジトリへ push しないこと**（コンテナはインターネットにつながるため）

## 構成

```
ホスト
├─ abs-gpodder-bridge-tests/            ← テスト（コンテナには見せない）
├─ ~/.claude/                           ← テスト作成側の履歴（コンテナには見せない）
├─ abs-gpodder-bridge-tests/.state/     ← git 管理外。STATE_DIR で変更できる
│   ├─ claude/       → /home/dev/.claude  実装エージェントの設定・認証情報・セッション履歴（永続）
│   ├─ impl-cache/   → /home/dev/.cache   Go のキャッシュ
│   ├─ mailbox/      → /mailbox           テスト依頼と結果の受け渡し
│   ├─ private/      （非公開）           テストの生ログ
│   └─ build-cache/  （非公開）           実装をビルドするときのキャッシュ
│
├─ watch-mailbox（ホストのプロセス）
│     mailbox/requests/<id>.json を見つけたら run-hidden-tests を実行し、
│     mailbox/results/<id>.{md,json} に整形済みの結果を置く
│
└─ docker
    ├─ 実装エージェントのコンテナ  /workspace = abs-gpodder-bridge
    │     `hidden-test` で依頼を置き、結果を待つ
    ├─ ビルド用のコンテナ（毎回使い捨て）  実装を読み取り専用でマウントして go build
    └─ テスト用のコンテナ（毎回使い捨て）  --network none
          e2e は /root（700）配下、ブリッジは nobody で起動するので、ブリッジからテストは読めない
```

## 使い方

```sh
# 実装エージェントを起動する（watcher も自動で起動する）
harness/bin/impl-shell                  # claude
harness/bin/impl-shell claude --resume  # 前回のセッションを再開
harness/bin/impl-shell bash             # コンテナ内のシェル
harness/bin/impl-shell --rebuild        # イメージを作り直す（Claude Code の更新）

# 隔離できているか確認する（e2e/CANARY の文字列をコンテナ内から探す）
harness/bin/verify-isolation

# ホストで直接テストを実行する（生ログは private/runs/<id>/ に残る）
harness/bin/run-hidden-tests [-run PATTERN]
```

### 認証

リポジトリ直下の `.env`（git 管理外）に、長期トークンを書く。

```sh
claude setup-token   # ホストで実行し、出力されたトークンを控える
# .env に次の 1 行を書き、chmod 600 .env
CLAUDE_CODE_OAUTH_TOKEN=<トークン>
```

- `impl-shell` は `.env` の `CLAUDE_CODE_OAUTH_TOKEN` の行だけを読み、名前だけの `docker run -e CLAUDE_CODE_OAUTH_TOKEN` で渡す（値はコマンドラインに載らない）。`.env` は `source` しない
- `.env` がない場合は、コンテナ内の claude で `/login` する。認証情報は `.state/claude/` に保存される
- 保存場所は `ENV_FILE` で変更できる

コンテナ内の実装エージェントは、次のコマンドでテストを依頼する。

```sh
hidden-test                # 全件
hidden-test -run Healthz   # go test -run のパターンで絞り込み
# 終了コード: 0=全件成功 1=失敗あり 2=ビルド失敗 3=その他
```

## テストを書くときのルール

- 実装者に返るのは「テスト名」「合否」「`harness.Failf` / `harness.Fatalf` のメッセージ」だけ。`t.Log` やブリッジのログは返らない
- メッセージには仕様の節番号と「何が起きなかったか」だけを書く。期待値の具体的な値や入力データは書かない
- テスト名は実装者に見えるので、振る舞いが分かる名前にする（例：`TestHealthz_NotForwardedToGoPodder`）
- e2e は標準ライブラリだけで書く（テスト用のコンテナはネットワークにつながらないため）

## 実装との約束

- `go build ./cmd/bridge` でブリッジのバイナリができること（`CGO_ENABLED=0`、`GOTOOLCHAIN=local`）
- 設定は環境変数だけで受け取ること（仕様 2.1 節）。テストは指定した環境変数以外を渡さない
