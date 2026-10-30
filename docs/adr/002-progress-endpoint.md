# 002. 進捗の取得に `GET /api/me/progress` を使う

- 状態：採用
- 日付：2026-10-17
- 関連：[仕様](../spec.md) 5.1 節・5.2 節・10.1 節（元の論点 B11）

## 状況

ブリッジは、ABS のユーザーの進捗を取得して goPodder の記録と比べる。最初の仕様では `GET /api/me`（ユーザー情報の全体）の中の `mediaProgress` を使うことにしていた。

ABS v2.36.0 で、進捗だけを返す `GET /api/me/progress` が追加された。応答は `{ "mediaProgress": [...] }` で、要素の形は `GET /api/me` の `mediaProgress` と同じ（`server/controllers/MeController.js` の `getAllMediaProgress`）。v2.36.0 より前の ABS では、このパスはどのルートにも一致せず 404 になる見込み（ルートの定義から判断。実機では未確認）。

## 選択肢

- (a) `GET /api/me` を使い続ける（全バージョンで動く）
- (b1) `GET /api/me/progress` を使い、ABS v2.36.0 以上を必須にする
- (b2) まず `GET /api/me/progress` を呼び、404 なら `GET /api/me` に切り替える
- (b3) `GET /status` で ABS のバージョンを調べて使い分ける

得られる効果は、応答が少し小さくなることと、進捗だけを取るという意図がはっきりすることくらいで、当初は (a) を推奨した。

## 決定

(b1) を採る。

- 進捗の取得には `GET /api/me/progress` を使い、ABS v2.36.0 以上を必須とする。
- 404 などで失敗したら「進捗の取得に失敗」として扱う。GET では突き合わせを省き、POST では ABS へ反映しない（[004](004-compare-on-post.md)）。warn ログで、ABS が古い可能性を示す。

## 結果

- 利用する ABS は v2.37.1 で、個人で使う前提なので、古い ABS への対応は要らない。仕様もテストも 1 通りで済む。
- 古い ABS で動かすと、同期は行われず中継だけになる。
- 古い版にも対応したくなったら、(b2) を足せばよい。読み取りの処理は共通なので、追加は小さい。
