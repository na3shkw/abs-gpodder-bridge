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

ENV PATH=/home/dev/.local/bin:$PATH \
    CLAUDE_CONFIG_DIR=/home/dev/.claude \
    DISABLE_AUTOUPDATER=1 \
    GOCACHE=/home/dev/.cache/go/build \
    GOMODCACHE=/home/dev/.cache/go/mod \
    HIDDEN_TEST_MAILBOX=/mailbox

WORKDIR /workspace
CMD ["claude"]
