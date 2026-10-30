# abs-gpodder-bridge

gPodder 互換の同期アプリ（主に AntennaPod）と [audiobookshelf](https://www.audiobookshelf.org/)（ABS）の間で、ポッドキャストの再生位置を同期する HTTP サーバー。

たとえば、通勤中に AntennaPod で 20 分まで聴いたエピソードを、家で ABS の Web 画面から続きを再生できる。逆に ABS で聴き進めた位置も、次に AntennaPod が同期したときに AntennaPod へ届く。

## しくみ

ブリッジは、gPodder 互換の同期サーバー [goPodder](https://github.com/cbrgm/gopodder) の前に置くリバースプロキシとして動く。

```
AntennaPod ──HTTP──▶ ブリッジ ──HTTP──▶ goPodder（内部ネットワークのみ）
                       │
                       └──HTTP──▶ ABS REST API
```

- 同期データの保存は goPodder が受け持つ。ブリッジはすべてのリクエストを goPodder へ中継し、ブリッジ自身はデータを保存しない。
- AntennaPod が再生の記録を送ったとき（POST）は、その位置を ABS にも反映する。
- AntennaPod が同期したとき（GET）は、goPodder と ABS の新しい方の位置にそろえる。ABS の方が新しければ goPodder に書き戻し、その応答で AntennaPod に届ける。
- AntennaPod の記録と ABS のエピソードは、フィードの URL・音声ファイルの URL・GUID で対応づける。そのため、ブリッジは ABS の全エピソードの一覧（索引）を定期的に作る。

## 必要なもの

- ABS v2.36.0 以上（`GET /api/me/progress` を使う）
- goPodder v1.4.0 以上
- Docker（Docker を使わない場合は Go 1.25）

> [!WARNING]
> ブリッジが扱えるユーザーは 1 人だけ。goPodder のユーザー名にかかわらず、すべての同期を `ABS_TOKEN` の 1 人の ABS ユーザーとして扱う。複数の人で goPodder を共有していると、全員の再生位置が同じ ABS ユーザーに書き込まれる。

## 動かし方

ブリッジと goPodder を、このリポジトリの [`compose.yaml`](compose.yaml) で並べて動かす。goPodder のポートは外へ公開せず、ブリッジだけを公開する。ブリッジのイメージは `ghcr.io/na3shkw/abs-gpodder-bridge` で公開している。

1. [`.env.example`](.env.example) を `.env` にコピーし、ABS の URL とトークンを書く。ブリッジの設定は `GOPODDER_URL`（`compose.yaml` で決めている）を除いてすべてこのファイルに書く。任意の設定（索引を作る時刻、タイムゾーン、イメージのタグなど）も同じファイルに書ける。

   ```sh
   cp .env.example .env
   ```

2. 起動する。

   ```sh
   docker compose up -d
   ```

3. ブラウザでブリッジ（`http://<ホスト>:8080/`）を開き、goPodder の初期設定をする。ブリッジはすべてのリクエストを goPodder へ中継するので、goPodder の Web 画面もブリッジ経由で使える。
   1. 最初に開くと、goPodder の管理者アカウントを作る画面になる。
   2. 管理者でログインし、同期用のログイン（AntennaPod が使うユーザー名とパスワード）を作る。

4. AntennaPod の同期の設定で「gpodder.net」を選び、サーバーにブリッジ（`http://<ホスト>:8080`）、ユーザー名とパスワードに 3 で作った同期用のログインを指定する。

公開するポートなど、`.env` で変えられないものを変えたいときは、`compose.yaml` は直さずに、同じフォルダに `compose.override.yaml` を置いて上書きする。

### Docker を使わない場合

```sh
go build -o bridge ./cmd/bridge

GOPODDER_URL=http://gopodder:8080 \
ABS_URL=http://abs:80 \
ABS_TOKEN=<ABS の一般ユーザーの API トークン> \
./bridge
```

### 主な設定

設定はすべて環境変数で渡す。全部の変数と、値が不正なときの扱いは[仕様書の 2.1 節](docs/spec.md#21-環境変数)を参照。

| 変数 | 必須 | 既定値 | 内容 |
|---|---|---|---|
| `GOPODDER_URL` | ○ | — | goPodder の URL |
| `ABS_URL` | ○ | — | ABS の URL |
| `ABS_TOKEN` | ○ | — | ABS の一般ユーザーの API トークン |
| `ABS_ADMIN_TOKEN` | — | なし | ABS の管理者の API トークン。ABS の RSS フィードで購読しているエピソードを対応づけるときだけ使う |
| `ABS_INDEX_SCHEDULE` | — | `@every 15m` | 索引を作る時刻（cron 式、または `@every 15m` のような間隔） |
| `LISTEN_PORT` | — | `8080` | 待ち受けるポート |
| `TZ` | — | `UTC` | `ABS_INDEX_SCHEDULE` とログの時刻に使うタイムゾーン（例：`Asia/Tokyo`） |

設定に誤りがあると、エラーをログに出して終了コード 1 で止まる。ヘルスチェックには `GET /healthz` を使える。

## 開発

```sh
docker build -t ghcr.io/na3shkw/abs-gpodder-bridge:dev .   # 手元でイメージを作る（.env で BRIDGE_IMAGE_TAG=dev にすると compose で使える）
go test ./...              # 自分で書くテスト（関数単位・プロセス内の HTTP）
scripts/acceptance         # 受け入れテスト（ブリッジをビルドして起動し、偽の ABS・goPodder を相手に仕様どおりか確かめる）
scripts/diagrams           # 受け入れテストの記述から図を作り直す
scripts/diagrams check     # 図が最新かを確かめる
scripts/serve-diagrams     # 図の一覧を http://127.0.0.1:8766/diagrams/index.html で見る
```

- 受け入れテスト（`acceptance/`）は、仕様書の要件ごとに「前提・操作・期待」の形で書いてあり、テスト 1 件につき 1 枚の図（`acceptance/diagrams/`）がある。
- 開発の約束事は [CLAUDE.md](CLAUDE.md) にまとめてある。

## ドキュメント

- [仕様書](docs/spec.md)：ブリッジがどう動くべきか
- [決定の記録（ADR）](docs/adr/README.md)：仕様やテストの作り方を、なぜそう決めたか
