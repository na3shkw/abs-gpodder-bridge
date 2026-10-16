# shellcheck shell=bash
# 隠蔽テストハーネスの共通設定。各スクリプトから source される。
# 値を変えたい場合は、環境変数で上書きするか harness/config.local.sh（git 管理外）に書く。

HARNESS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TESTS_REPO="$(cd "$HARNESS_DIR/.." && pwd)"

if [[ -f "$HARNESS_DIR/config.local.sh" ]]; then
  # shellcheck source=/dev/null
  source "$HARNESS_DIR/config.local.sh"
fi

# 実装リポジトリ（実装エージェントのコンテナに /workspace としてマウントする）
IMPL_DIR="${IMPL_DIR:-$(cd "$TESTS_REPO/.." && pwd)/abs-gpodder-bridge}"

# ホスト側の状態ディレクトリ（git 管理外）。
# このリポジトリ自体はコンテナにマウントしないので、ここに置いてもテストは見えない。
STATE_DIR="${STATE_DIR:-$TESTS_REPO/.state}"
# コンテナに見せるもの
IMPL_CLAUDE_DIR="$STATE_DIR/claude"        # 実装エージェントの Claude Code 設定・セッション履歴
IMPL_CACHE_DIR="$STATE_DIR/impl-cache"     # 実装エージェントの Go キャッシュなど
MAILBOX_DIR="$STATE_DIR/mailbox"           # テスト依頼と結果の受け渡し
# コンテナに見せないもの
PRIVATE_DIR="$STATE_DIR/private"           # 生ログ（テスト内容を含むため非公開）
BUILD_CACHE_DIR="$STATE_DIR/build-cache"   # 実装ビルド用の Go キャッシュ

# 実装エージェントの認証トークン（CLAUDE_CODE_OAUTH_TOKEN）を書くファイル（git 管理外）
ENV_FILE="${ENV_FILE:-$TESTS_REPO/.env}"

IMPL_IMAGE="${IMPL_IMAGE:-abs-gpodder-bridge-impl:latest}"
GO_IMAGE="${GO_IMAGE:-golang:1.25-bookworm}"
E2E_GOCACHE_VOLUME="${E2E_GOCACHE_VOLUME:-abs-gpodder-bridge-e2e-gocache}"

# ブリッジのビルド対象パッケージ（実装との約束）
BRIDGE_PKG="${BRIDGE_PKG:-./cmd/bridge}"
# go test 全体のタイムアウト
E2E_TIMEOUT="${E2E_TIMEOUT:-10m}"

ensure_state_dirs() {
  mkdir -p "$IMPL_CLAUDE_DIR" "$IMPL_CACHE_DIR" "$PRIVATE_DIR" "$BUILD_CACHE_DIR" \
    "$MAILBOX_DIR/requests" "$MAILBOX_DIR/processing" "$MAILBOX_DIR/results"
}
