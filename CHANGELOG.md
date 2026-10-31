# Changelog

## [v0.1.0](https://github.com/na3shkw/abs-gpodder-bridge/commits/v0.1.0) - 2026-10-31

最初のリリース。

### New Features 🎉
- gPodder 互換の同期アプリ（主に AntennaPod）と Audiobookshelf（ABS）の間で、ポッドキャストの再生位置を同期する。goPodder の前に置くリバースプロキシとして動き、すべてのリクエストを goPodder へ中継する
  - AntennaPod が再生の記録を送ったとき（POST）は、その位置を ABS にも反映する
  - AntennaPod が同期したとき（GET）は、goPodder と ABS の新しい方の位置にそろえる
  - AntennaPod の記録と ABS のエピソードを、フィードの URL・音声ファイルの URL・GUID で対応づける。そのための索引を `ABS_INDEX_SCHEDULE`（cron 式、または `@every 15m` のような間隔）で定期的に作る
  - ABS がベースパス（v2.18.0 以降の既定は `/audiobookshelf`）の下で動いていても、ABS の RSS フィードの音声 URL を対応づけられる
- 設定は環境変数だけで受け取る。設定に誤りがあると、エラーをログに出して終了コード 1 で止まる
- 環境変数 `TZ`（既定 `UTC`）で、`ABS_INDEX_SCHEDULE` とログの時刻に使うタイムゾーンを決められる。OS にタイムゾーンのデータがなくても動く
- ヘルスチェック用の `GET /healthz`
- POST の本文が 10 MiB を超えたら 413 を返し、リクエストヘッダーを 10 秒以内に読み終えられない接続は切る
- コンテナイメージ `ghcr.io/na3shkw/abs-gpodder-bridge`（linux/amd64）と、ブリッジと goPodder を並べて動かす `compose.yaml`・`.env.example`

### Documentation 📝
- README、仕様書（`docs/spec.md`）、決定の記録（`docs/adr/`）
- 受け入れテストの図の一覧を GitHub Pages（https://na3shkw.github.io/abs-gpodder-bridge/）で公開する

### Other Changes
- 仕様書の要件ごとの受け入れテスト（`acceptance/`）と、その記述から図を作る仕組み
- CI（vet・テスト・受け入れテスト・図が最新かの確認・イメージのビルド）と、tagpr によるリリースの workflow
