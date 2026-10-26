# 実装エージェント（Claude Code）用のコンテナ。
# 隠蔽テストは含めない。実装リポジトリ・自分の Claude 設定・mailbox だけを実行時にマウントする。
FROM golang:1.25-bookworm

ARG UID=1000
ARG GID=1000

RUN apt-get update \
 && apt-get install -y --no-install-recommends git ripgrep jq curl ca-certificates less procps \
 && rm -rf /var/lib/apt/lists/*

RUN groupadd -g "${GID}" dev && useradd -m -u "${UID}" -g "${GID}" -s /bin/bash dev

COPY hidden-test /usr/local/bin/hidden-test

USER dev
RUN curl -fsSL https://claude.ai/install.sh | bash

# gopls-lsp プラグイン用。GOBIN はマウントで隠れない場所に置く
# v0.22 以降は Go 1.26 以上が必要なので、Go 1.25 で動く v0.21 系に固定する
RUN GOBIN=/home/dev/.local/bin GOTOOLCHAIN=local go install golang.org/x/tools/gopls@v0.21.1 \
 && go clean -cache -modcache

ENV PATH=/home/dev/.local/bin:$PATH \
    CLAUDE_CONFIG_DIR=/home/dev/.claude \
    DISABLE_AUTOUPDATER=1 \
    GOCACHE=/home/dev/.cache/go/build \
    GOMODCACHE=/home/dev/.cache/go/mod \
    HIDDEN_TEST_MAILBOX=/mailbox

WORKDIR /workspace
CMD ["claude"]
