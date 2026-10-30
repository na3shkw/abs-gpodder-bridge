# abs-gpodder-bridge インターフェース仕様

## 1. 目的と構成

abs-gpodder-bridge（以下「ブリッジ」）は、gPodder 互換の同期クライアント（主に AntennaPod）と audiobookshelf（以下「ABS」）の間で、ポッドキャストエピソードの再生状況を同期する HTTP サーバーである。

ブリッジは goPodder の前段に置くリバースプロキシとして動作する。

```
AntennaPod ──HTTP──▶ ブリッジ ──HTTP──▶ goPodder（内部ネットワークのみ）
                       │
                       └──HTTP──▶ ABS REST API
```

- gPodder API の実装と、すべての同期データの保存は goPodder が担う。
- ブリッジは、すべてのリクエストを goPodder へ中継する。そのうえで、エピソードアクションの POST と GET の 2 つにだけ処理を差し込み、ABS との同期を行う（5 章）。
- ブリッジ自身は永続データを持たない。エピソード索引（6.3 節）はメモリ上にだけ持ち、再起動で失われてよい。
- ブリッジは、ABS の再生状況（進捗）を定期的に見に行かず、ABS からの通知も受けない。ABS での進捗の変化は、クライアントが次に同期（5.2 節の GET）したときにクライアントへ届く。例外として、照合に使うエピソード索引だけは、`ABS_INDEX_SCHEDULE` で指定した時刻に ABS から取得して作り直す（6.3 節）。

### 用語

| 用語 | 意味 |
|---|---|
| クライアント | ブリッジに接続する gPodder 互換アプリ（AntennaPod など） |
| アクション | gPodder API のエピソードアクション（`POST/GET /api/2/episodes/{user}.json` の要素） |
| play アクション | `action` が `play` のアクション（大文字小文字を区別しない） |
| ABS エピソード | ABS 上のポッドキャストエピソード。`libraryItemId` と `episodeId` の組で一意に識別する |
| ABS 進捗 | ABS の `mediaProgress` のうち、`episodeId` を持つもの |
| 照合 | アクションのエピソードを ABS エピソードに対応づけること（6 章） |
| フィード用 ID | ABS の RSS フィードが、エピソードごとに割り当てる ID。ABS エピソードの `episodeId` とは別の値（6.2 節） |

## 2. 設定（環境変数・ポート）

### 2.1 環境変数

| 変数名 | 必須 | 既定値 | 説明 |
|---|---|---|---|
| `LISTEN_PORT` | — | `8080` | ブリッジが待ち受ける TCP ポート |
| `GOPODDER_URL` | ○ | — | goPodder のベース URL（例：`http://gopodder:8080`）。末尾の `/` は有無を問わない |
| `GOPODDER_TIMEOUT_SECONDS` | — | `30` | goPodder への 1 リクエストあたりのタイムアウト（秒）。クライアントのリクエストの中継と、5 章でブリッジが独自に送るリクエストの両方に使う |
| `ABS_URL` | ○ | — | ABS のベース URL（例：`http://abs:80`）。末尾の `/` は有無を問わない |
| `ABS_TOKEN` | ○ | — | ABS の一般ユーザーの API トークン。`Authorization: Bearer` で送る（3.2 節） |
| `ABS_ADMIN_TOKEN` | — | なし | ABS の管理者の API トークン。`GET /api/feeds` だけに使う（3.2 節、6.2 節）。未設定なら照合の手順 1 を行わない |
| `ABS_TIMEOUT_SECONDS` | — | `5` | ABS への 1 リクエストあたりのタイムアウト（秒） |
| `ABS_INDEX_SCHEDULE` | — | `@every 15m` | エピソード索引（6.3 節）を作る時刻。cron 式（分 時 日 月 曜日の 5 フィールド）、または `@every {間隔}`（例：`@every 15m`、`@every 1s`）で書く。曜日は `0`〜`6`（`0` が日曜日、`7` は不正）。月と曜日は名前（`JAN`、`MON` など）でも書ける。`@hourly`・`@daily`・`@weekly`・`@monthly`・`@yearly` も受け付ける |
| `LOG_LEVEL` | — | `info` | `debug` / `info` / `warn` / `error` のいずれか |
| `TZ` | — | `UTC` | ブリッジが動作するタイムゾーン。IANA のタイムゾーン名（例：`Asia/Tokyo`）で書く。`ABS_INDEX_SCHEDULE` の cron 式と、ログに出す時刻に使う |

- 必須の変数が未設定の場合、またはいずれかの変数（必須・任意を問わない）の値が不正（数値でない、URL として解釈できない、cron 式として解釈できない、`LOG_LEVEL` が上記以外、`TZ` がタイムゾーン名として解釈できない、など）の場合、ブリッジはエラーをログに出力し、終了コード 1 で起動を中止する。
- `TZ` は gPodder のアクションの `timestamp` に影響しない。`timestamp` は `TZ` にかかわらず UTC として読み書きする（11.1 節）。
- タイムゾーンのデータは、実行環境の OS にタイムゾーンのデータがなくても解釈できるよう、ブリッジ自身が持つ。
- ログは標準出力に出す。ログの書式は規定しない。

> 決めた経緯：[ADR 009](adr/009-index-schedule.md)、[ADR 010](adr/010-timezone.md)、[ADR 011](adr/011-config-values.md)、[ADR 012](adr/012-no-log-assertions.md)

### 2.2 ポートと公開範囲

- 外部（クライアント）に公開するのは、ブリッジの `LISTEN_PORT` だけとする。
- goPodder は compose 内部のネットワークだけに置き、ホストへポートを公開しない。
- 通信は HTTP のみとする。HTTPS は扱わない（9 章）。

### 2.3 ヘルスチェック

ブリッジは、自身のヘルスチェック用に次のエンドポイントを持つ。このエンドポイントは goPodder へ中継しない。

- `GET /healthz`
  - 常に `200 OK`、`Content-Type: application/json`、本文 `{"status":"ok"}` を返す。
  - goPodder や ABS の状態、索引の有無は確認しない。

## 3. 認証

### 3.1 クライアント ⇔ ブリッジ ⇔ goPodder

- ブリッジは独自の認証を持たない。クライアントの認証はすべて goPodder が行う。
- ブリッジは、リクエストの `Authorization` ヘッダーと `Cookie` ヘッダーをそのまま goPodder へ転送する。goPodder の応答の `Set-Cookie` ヘッダーも、そのままクライアントへ返す。
- 5 章でブリッジが独自に goPodder へ送るリクエストには、元のクライアントリクエストの `Authorization` ヘッダーと `Cookie` ヘッダーをそのまま付ける。

### 3.2 ブリッジ ⇔ ABS

- ブリッジは、`GET /api/feeds` 以外の ABS へのすべてのリクエストに `Authorization: Bearer {ABS_TOKEN}` を付ける。
- `GET /api/feeds` には `Authorization: Bearer {ABS_ADMIN_TOKEN}` を付ける。`ABS_ADMIN_TOKEN` は、ほかのどのリクエストにも使わない。
  - ABS の API トークンは持ち主のユーザーの全権限を持つ。管理者のトークンの使い道を 1 つに限るのはそのためである。
  - `ABS_TOKEN` のユーザーが管理者であれば、`ABS_ADMIN_TOKEN` に同じ値を設定してよい。
- 本バージョンは単一ユーザーを前提とする。gPodder 側のユーザー名（パスの `{user}`）にかかわらず、すべての同期を `ABS_TOKEN` のユーザーの進捗として扱う。
- 索引（6.3 節）は `ABS_TOKEN` のユーザーがアクセスできるライブラリとアイテムだけから作る。ブリッジは、索引にない ABS エピソードの進捗を更新しない。

> 決めた経緯：[ADR 001](adr/001-feed-url-mapping.md)

## 4. 中継するエンドポイント

`GET /healthz` 以外のすべてのリクエストを、goPodder へ中継する。

- メソッド、パス、クエリ文字列、本文は変更せずに転送する。
- 転送先は、`GOPODDER_URL` にリクエストのパスとクエリ文字列を連結した URL とする。
- `Host` ヘッダーは転送先に合わせて書き換える。それ以外のヘッダーは、HTTP の hop-by-hop ヘッダー（`Connection`、`Keep-Alive`、`Transfer-Encoding` など）を除いて、そのまま転送する。
- goPodder の応答のステータスコード、ヘッダー（hop-by-hop ヘッダーを除く）、本文は、変更せずにクライアントへ返す。
- 5 章の差し込みを行うエンドポイントでも、クライアントへ返す応答は goPodder の応答そのものとする。ブリッジが応答の内容を書き換えることはない。

## 5. 差し込みを行うエンドポイント

ここでの `{user}` は任意のユーザー名を表す。パスは `/api/2/episodes/{user}.json` に完全一致するものだけを対象とする。

### 5.1 POST /api/2/episodes/{user}.json

クライアントがアクションを送信したときに、play アクションを ABS へ反映する。

処理の順序は次のとおり。

1. リクエストを 4 章のとおり goPodder へ中継し、応答を受け取る。
2. goPodder の応答が 2xx 以外なら、ABS への反映は行わない。goPodder の応答をそのまま返して終了する。
3. 2xx なら、リクエスト本文のアクションのうち、次の条件をすべて満たすものを反映の候補とする。
   - play アクションである。
   - `position` が 0 以上の数値である。
   - 11 章の形式として有効である（`timestamp` はなくてもよい）。
   - 6 章の照合で ABS エピソードが見つかる。
4. 同じ ABS エピソードに複数の候補がある場合は、`timestamp` が最も新しい 1 件だけを残す。`timestamp` が同じものが複数ある場合は、本文の配列で後ろにあるものを残す。比較に使う時刻は 7.1 節のとおり。
5. 候補が 1 件以上残っていれば、ABS の進捗一覧を取得する（`GET /api/me/progress`。10 章）。取得に失敗した場合は、ABS への反映を行わずに手順 7 へ進む。
6. 残った各候補について、7 章のルールで ABS へ反映するかを判定し、反映するものは 5.3 節のとおり ABS を更新する。
7. goPodder の応答をそのままクライアントへ返す。

- ABS への反映は、クライアントへ応答を返す前に完了させる。
- 手順 3〜6 で ABS への通信が失敗しても、クライアントへの応答は手順 1 の goPodder の応答のままとする（8 章）。
- 本文が JSON として解釈できない場合、または配列でない場合は、ABS への反映を行わずに goPodder の応答を返す。
- 照合できなかったアクションのために索引を作り直すことはしない（6.3 節）。照合できなかったアクションも goPodder には保存されているので、索引が更新されたあとの 5.2 節のリコンサイルで ABS へ反映される。

> 決めた経緯：[ADR 002](adr/002-progress-endpoint.md)、[ADR 004](adr/004-compare-on-post.md)

### 5.2 GET /api/2/episodes/{user}.json

クライアントがアクションを取得する前に、goPodder と ABS の再生状況を双方向に突き合わせ（以下「リコンサイル」）、差分を解消する。

処理の順序は次のとおり。

1. **goPodder の記録を取得する**：goPodder へ `GET /api/2/episodes/{user}.json?since=0` を送る（認証ヘッダーは 3.1 節のとおり）。2xx 以外が返った場合は、手順 2〜4 を省略して手順 5 へ進む。
2. **エピソードごとの状態を求める**：手順 1 の応答の `actions` を、`(podcast, episode)` の組ごとにまとめ、組ごとに次のどちらかを「goPodder の状態」とする。
   - 次の条件をすべて満たす play アクションがあれば、そのうち `timestamp` が最も新しいもの（**再生の状態**）。
     - `position` が 0 以上の数値である。
     - `timestamp` があり、11 章の形式として有効である。`timestamp` がない play アクションはリコンサイルの対象にしない。
   - 上記の play アクションがなく、play 以外の種類のアクション（`download`、`delete` など、種類を問わない）があれば、そのうちの 1 件（**再生以外の状態**）。このエピソードは、クライアントが知っているエピソードとして扱う。
   - 上記の条件を満たさない play アクション（`timestamp` がないものなど）は、どちらの状態にも使わない。
3. **ABS の進捗を取得する**：ABS の進捗一覧を取得する（`GET /api/me/progress`。10 章）。失敗した場合は、手順 4 を省略して手順 5 へ進む。
4. **突き合わせる**：手順 2 の各組を 6 章で照合し、ABS エピソードが見つかったものについて、7 章のルールで次のどちらか、または何もしないかを決める。
   - **goPodder → ABS**：5.3 節のとおり ABS を更新する。再生の状態の組だけが対象になる。
   - **ABS → goPodder**：5.4 節のアクションを、goPodder へ `POST /api/2/episodes/{user}.json` で送る。この方向のアクションが複数ある場合は、1 回の POST にまとめて送る。
   - 組は、goPodder の状態の時刻（7.1 節）が古い順に処理する。同じ ABS エピソードに複数の組が照合された場合も組ごとに処理し、まとめない。古い順に処理することで、同じ ABS エピソードを複数回更新したときに最後に残るのは最も新しい状態になる。
5. **元のリクエストを中継する**：クライアントのリクエストを 4 章のとおり goPodder へ中継し、その応答をそのまま返す。

- 手順 1〜4 のどこで失敗しても、手順 5 は必ず実行する（8 章）。
- goPodder に一度もアクションが記録されていないエピソードは、ABS に進捗があってもリコンサイルの対象にしない（9 章）。

> 決めた経緯：[ADR 005](adr/005-invalid-actions.md)、[ADR 006](adr/006-duplicate-and-ambiguous.md)、[ADR 008](adr/008-write-back-non-play.md)

### 5.3 ABS を更新する内容

ABS エピソード `(libraryItemId, episodeId)` を、play アクション `a` の内容で更新するときは、次の本文で `PATCH /api/me/progress/{libraryItemId}/{episodeId}` を送る。

| ABS のフィールド | 値 |
|---|---|
| `currentTime` | `a.position`（秒） |
| `duration` | `a.total` が正の数値の場合のみ含める。値は `a.total`（秒） |
| `isFinished` | `a.total` が正の数値、かつ `a.position >= a.total` の場合のみ含める。値は `true` |

- `isFinished` は `false` を送らない。ABS は、完了済みの進捗に `isFinished: false` を受け取ると、`currentTime` を 0 に戻し、送られた `currentTime` を無視する。`isFinished` を含めずに `currentTime` を送れば、ABS は完了を自動で外し、送られた位置を保つ。
- `lastUpdate` は送らない。ABS 進捗の `lastUpdate` は、ABS が更新した時刻になる。
- ABS は、既存の進捗を更新したときに残り時間（`duration − currentTime`）が 10 秒未満なら、自動で完了（`isFinished: true`）にする。ブリッジはこの振る舞いを抑えない。

> 決めた経緯：[ADR 003](adr/003-abs-progress-update.md)

### 5.4 goPodder へ書き込むアクションの内容

ABS 進捗 `p` を goPodder へ書き込むときは、次のアクションを作る。ここでの「goPodder の状態」は、5.2 節の手順 2 で照合に使った組の状態（再生の状態、または再生以外の状態）を指す。

| フィールド | 値 |
|---|---|
| `podcast` | goPodder の状態の `podcast`（クライアントが知っている URL をそのまま使う） |
| `episode` | 同じく `episode` |
| `guid` | goPodder の状態に `guid` があれば同じ値。なければ含めない |
| `action` | `"play"` |
| `timestamp` | `p.lastUpdate` を 11.1 節の形式に変換した値 |
| `started` | `0`。`total` を含めない場合は含めない |
| `position` | `p.isFinished` が真で、かつ `total` を含める場合は `total` と同じ値。それ以外は `p.currentTime` の小数点以下を切り捨てた整数 |
| `total` | 次の順に決め、小数点以下を切り捨てた整数とする。① `p.duration` が正の数値ならその値 ② そうでなく、索引（6.3 節）にある ABS エピソードの長さが正の数値ならその値 ③ どちらでもなければ、`total` を含めない |

`device` フィールドは含めない。

- ABS の Web 画面で、再生していないエピソードに完了の印を付けると、ABS 進捗の `duration` と `currentTime` は 0 になる。そのため ② で索引の長さを使う。
- AntennaPod は、受け取ったアクションの `total` を使わず、`position` と自分が持つエピソードの長さを比べて再生済みかを判断する。完了をクライアントへ伝える手段は、`position` を長さと同じにすることである。

> 決めた経緯：[ADR 007](adr/007-write-back-finished.md)

## 6. エピソードの照合ルール

### 6.1 照合の入力

照合は、アクションの次の値を使う。

- `episode`：音声ファイルの URL（必須）
- `guid`：エピソードの GUID（任意）

### 6.2 照合の手順

次の順に試し、最初に一致した ABS エピソードを照合結果とする。どれにも一致しなければ「照合できない」とする。

1. **ABS のフィード URL**：`episode` の URL のパスが `/feed/{slug}/item/{フィード用 ID}/...` の形で、そのフィード用 ID が索引の対応表（6.3 節）にある場合、対応する ABS エピソード。URL のスキーム、ホスト、ポートは比較しない。索引に対応表がない場合（`ABS_ADMIN_TOKEN` が未設定、または `GET /api/feeds` に失敗した場合）は、この手順を飛ばす。
2. **元の配信 URL**：`episode` が、ABS エピソードの元の配信 URL（ABS がフィードから取り込んだときの音声ファイルの URL）と一致する場合、その ABS エピソード。
3. **GUID**：`guid` があり、それが ABS エピソードの GUID と一致する場合、その ABS エピソード。

- 元の配信 URL や GUID を持たない ABS エピソード（値が `null`）は、その手順では一致しない。
- いずれかの手順で、同じキーに複数の ABS エピソードが一致した場合は、そのアクションを「照合できない」とし、次の手順は試さない。

URL の比較は、次の正規化をしたうえで文字列の完全一致で行う。

- スキームとホストは小文字にする。
- スキームの既定ポート（http の 80、https の 443）は取り除く。
- フラグメント（`#` 以降）は取り除く。
- パスとクエリ文字列は変更しない。

補足：ABS の RSS フィードでは、各エピソードの `<guid>` は音声 URL と同じ値になる。ABS のフィードで購読しているクライアントのアクションの `guid` は、元の GUID とは一致しない。

> 決めた経緯：[ADR 001](adr/001-feed-url-mapping.md)、[ADR 006](adr/006-duplicate-and-ambiguous.md)

### 6.3 エピソード索引

照合のため、ブリッジは ABS の全ポッドキャストライブラリから、次の情報を持つ索引を作る。

- ABS エピソードごとに：
  - `libraryItemId` と `episodeId`
  - 元の配信 URL（`media.episodes[].enclosure.url`。ないこともある）
  - GUID（`media.episodes[].guid`。ないこともある）
  - 音声ファイルのパス（`media.episodes[].audioFile.metadata.path`）
  - 長さ（`media.episodes[].duration`、秒）
- フィード用 ID の対応表：フィード用 ID → `(libraryItemId, episodeId)`

**作り方**：

1. `GET /api/libraries` で、`mediaType` が `podcast` のライブラリを得る。
2. 各ライブラリについて `GET /api/libraries/{libraryId}/items` で、アイテムの ID を得る。`limit` は付けない（全件が返る）。
3. `POST /api/items/batch/get` に、手順 2 のアイテムの ID を `{"libraryItemIds": [...]}` として送り、エピソードの情報を得る。1 回で送っても、複数回に分けて送ってもよい。
4. `ABS_ADMIN_TOKEN` が設定されていれば、`GET /api/feeds` で開いているフィードの一覧を得る。`entityType` が `libraryItem` のフィードの `episodes[]` について、`fullPath` が手順 3 のエピソードの音声ファイルのパスと完全に一致するものを対応づけ、`episodes[].id`（フィード用 ID）から ABS エピソードへの対応表を作る。手順 3 のエピソード（`ABS_TOKEN` のユーザーの索引）にないものは対応づけない。

**作る時刻**：

- ブリッジの起動時に 1 回作る。起動時の作成が終わるまでは、索引が空として扱う（すべて照合できない）。
- 以後、`ABS_INDEX_SCHEDULE` で指定した時刻に作り直す。毎回、全体を作り直す。
- 照合できないアクションがあっても、作り直さない。
- 作り直しに失敗した場合（手順 1〜3 のいずれかが失敗した場合）は、前の索引を使い続け、`warn` でログに出す。手順 4 だけが失敗した場合は、対応表のない索引を使い、`warn` でログに出す。
- 索引を作ってから次に作るまでの間に ABS に追加されたエピソードは、照合できない。ABS の RSS フィードのエピソード一覧は、クライアントがフィードを更新したときに更新されるので、`ABS_INDEX_SCHEDULE` は、ABS にエピソードが追加される時間帯の少し後にするとよい。

> 決めた経緯：[ADR 009](adr/009-index-schedule.md)

## 7. 競合の解決ルール

goPodder と ABS の状態が食い違う場合は、**更新時刻が新しい方を正とする**。

### 7.1 比較に使う時刻

- goPodder 側：
  - 5.1 節の POST：play アクションの `timestamp`。`timestamp` がないアクションは、ブリッジがそのリクエストを受け取った時刻とみなす。
  - 5.2 節の GET：再生の状態の `timestamp`。再生以外の状態は、どの時刻よりも古いものとみなす。
- ABS 側：ABS 進捗の `lastUpdate`（ミリ秒の UNIX 時刻）。ABS エピソードに進捗がない場合は「ABS 側は存在しない」とする。

### 7.2 判定

goPodder 側の状態 `a` と、ABS 側の進捗 `p` を比べ、次の表で処理を決める。

| 条件 | 処理 |
|---|---|
| `a` が再生以外の状態で、`p` が存在しない | 何もしない |
| `a` が再生以外の状態で、`p` が存在する | ABS → goPodder（5.2 節の GET の場合のみ） |
| `p` が存在しない | goPodder → ABS |
| `a` の時刻 > `p` の時刻、かつ「同じ状態」でない | goPodder → ABS |
| `p` の時刻 > `a` の時刻、かつ「同じ状態」でない | ABS → goPodder（5.2 節の GET の場合のみ。5.1 節の POST では何もしない） |
| 上記以外（「同じ状態」である、または時刻が等しい） | 何もしない |

表は上の行から順に当てはめる。

「同じ状態」とは、`a` が再生の状態であり、次のどちらかを満たすことをいう。

- `a.total` が正の数値かつ `a.position >= a.total`（`a` が完了）であり、`p.isFinished` が `true`
- `a` が完了ではなく、`p.isFinished` が `false` で、`a.position` と `p.currentTime` の差の絶対値が 1 秒以下

この判定により、ブリッジが ABS に反映した内容がリコンサイルで goPodder へ書き戻される、といった往復は起きない。

## 8. エラー時の振る舞い

| 状況 | 振る舞い |
|---|---|
| goPodder に接続できない、またはタイムアウトした（クライアントの元のリクエストの中継時） | `502 Bad Gateway` を返す |
| 5.1 節で、ABS の進捗の取得、照合、更新のいずれかに失敗した | goPodder の応答をそのまま返す。失敗は `warn` でログに出す |
| 5.2 節の手順 1〜4 のいずれかで失敗した | 失敗した手順以降のリコンサイルを省略し、手順 5 の中継は必ず行う。失敗は `warn` でログに出す |
| ABS の進捗の取得（`GET /api/me/progress`）が 404 などで失敗した | 上の 2 行のとおり扱う。ABS のバージョンが古い（v2.36.0 未満）可能性があることを `warn` のログに含める |
| 索引の作成に失敗した | 6.3 節のとおり、前の索引を使い続ける。`warn` でログに出す |
| 照合できないアクションがあった | そのアクションを無視する。`debug` でログに出す |
| ABS の 1 件の更新だけが失敗した | 他の候補の処理は続ける |

- ABS への失敗した更新を、ブリッジ自身が再試行することはない。取りこぼした内容は、次の 5.2 節のリコンサイルで解消される。
- ABS へのリクエストは `ABS_TIMEOUT_SECONDS`、goPodder へのリクエストは `GOPODDER_TIMEOUT_SECONDS` でタイムアウトさせる。

## 9. 対象外（やらないこと）

本バージョンでは、次のことを扱わない。

- HTTPS（TLS の終端）
- 複数ユーザー。gPodder のユーザーと ABS のユーザーの対応づけ
- play 以外のアクション（`download`、`delete`、`new` など）を ABS へ反映すること
- 購読（サブスクリプション）の変更を ABS へ反映すること。ABS へのポッドキャストの追加・削除
- goPodder に一度もアクションが記録されていないエピソードの ABS 進捗を、クライアントへ届けること（10.3 節の拡張として追記予定）
- オーディオブックの進捗
- ブリッジ独自の認証
- ブリッジ自身による同期データの永続化
- 大量のエピソード履歴に対する性能の最適化
- ABS の RSS フィードと元の配信元のフィードの間で購読を切り替えた場合など、同じ ABS エピソードが複数の `(podcast, episode)` の組で記録されている場合に、それらを 1 つにまとめること（5.2 節の手順 4 のとおり、組ごとに処理する）

## 10. 参照：ブリッジが使う外部の API と前提

### 10.1 ABS の API

ABS は v2.36.0 以上を必要とする（`GET /api/me/progress` のため）。v2.33.2 と v2.37.1 のソースで、この章の内容を確認した。

| 用途 | エンドポイント | トークン |
|---|---|---|
| ポッドキャストライブラリの一覧 | `GET /api/libraries`（`mediaType` が `podcast` のもの） | `ABS_TOKEN` |
| ライブラリ内のアイテム一覧 | `GET /api/libraries/{libraryId}/items`（`limit` なし。応答の `results[].id`） | `ABS_TOKEN` |
| アイテムのエピソード一覧 | `POST /api/items/batch/get`（本文 `{"libraryItemIds": [...]}`。応答の `libraryItems[].media.episodes[]`） | `ABS_TOKEN` |
| 開いている RSS フィードの一覧 | `GET /api/feeds`（応答の `feeds[]`。管理者専用） | `ABS_ADMIN_TOKEN` |
| 進捗の一覧 | `GET /api/me/progress`（応答の `mediaProgress` のうち `episodeId` を持つもの） | `ABS_TOKEN` |
| 進捗の更新 | `PATCH /api/me/progress/{libraryItemId}/{episodeId}` | `ABS_TOKEN` |

確認した ABS の振る舞い：

- RSS フィードのエピソードの音声 URL は `/feed/{slug}/item/{フィード用 ID}/media.{拡張子}` の形になる。フィード用 ID は、音声ファイルのパスごとに割り当てられ、フィードを開いている間は変わらない。フィードを閉じて開き直すと、slug とフィード用 ID が変わる。
- `GET /api/feeds` の `feeds[].entityId` はアイテムの ID、`feeds[].episodes[].id` はフィード用 ID、`feeds[].episodes[].fullPath` は音声ファイルのパスである。`fullPath` は、エピソードの `audioFile.metadata.path` と同じ値になる。
- ABS 進捗の `lastUpdate` はミリ秒の UNIX 時刻である。
- 進捗の更新では、値が変わった場合に `lastUpdate` が更新時刻になる。値が変わらない場合は更新されない。
- 進捗の更新は、エピソードを `episodeId` だけで探し、ライブラリへのアクセス権を確認しない（3.2 節で索引にないエピソードを更新しないとしているのはこのため）。
- 5.3 節に書いた、`isFinished: false` で位置が 0 に戻る振る舞いと、残り 10 秒未満で自動的に完了になる振る舞い。

### 10.2 goPodder の振る舞い

goPodder（cbrgm/gopodder、`21e868b`）のソースで、次のことを確認した。

- `GET /api/2/episodes/{user}.json?since=...` は、goPodder がアクションを受け取った時刻（アクションの `timestamp` ではない）が `since` 以上のものを返す。ブリッジが 5.2 節で書き込んだアクションは、直後に中継する元の GET の応答に含まれる。
- アクションは `(ユーザー, podcast, episode)` ごとに 1 件だけ保存され、新しく受け取ったアクションで、種類や `timestamp` に関係なく上書きされる。そのため、play のあとに `download` や `delete` を受け取ると、play の記録は残らない（5.2 節の手順 2 で再生以外の状態を扱うのはこのため）。
- `action`、`started`、`position`、`total` が同じアクションは、`timestamp` が違っても保存し直されない。
- `timestamp` がない、または解釈できないアクションは、`timestamp` なしで保存される。返す `timestamp` は UTC の `YYYY-MM-DDTHH:MM:SS` 形式にそろえられる。
- 受け取ってから一定期間（既定 90 日）経ったアクションは削除される。

### 10.3 追記する予定の拡張

**対象とする使い方**：ブラウザ（ABS の Web UI）で聴き始めて途中で止めたエピソードの続きを、AntennaPod で再生する。

現在の仕様では、goPodder に一度もアクションが記録されていないエピソードは 5.2 節のリコンサイルの対象外であり、この使い方に対応できない（9 章）。ただし、クライアントがそのエピソードをダウンロードしていれば、goPodder に `download` のアクションが残るので、5.2 節の再生以外の状態として対応できる。

**拡張の方針**：goPodder に記録がない ABS 進捗について、ブリッジがクライアントの知っている `podcast` と `episode` の URL を組み立て、5.4 節のアクションとして goPodder へ書き込む。

- `podcast`：その ABS エピソードが属するポッドキャストの、ABS の RSS フィードの URL（goPodder の購読一覧にあるもの）
- `episode`：その RSS フィードが配信している、そのエピソードの音声 URL（`/feed/{slug}/item/{フィード用 ID}/media.{拡張子}`）

クライアントは、受け取ったアクションの `guid` または `episode` が自分の持つエピソードと一致した場合にだけ再生位置を反映する。そのため、組み立てる URL はクライアントが RSS フィードから得た URL と一致していなければならない（ホスト部分は、クライアントがフィードを取得したときのホストになる）。

**前提**：

- クライアントが、元の配信元ではなく ABS の RSS フィード経由で購読していること

**進め方**：URL のホスト部分の決め方などを確認してから、この拡張を 5.2 節・5.4 節・6 章・9 章に正式な仕様として追記する。追記するまでは、この拡張を実装・テストの対象にしない。

## 11. データ形式

### 11.1 時刻

- gPodder のアクションの `timestamp` は、UTC の `YYYY-MM-DDTHH:MM:SS` 形式（タイムゾーン表記なし）として扱う。環境変数 `TZ`（2.1 節）には影響されない。末尾に `Z` が付いたもの、小数秒を含むもの、`+09:00` のような時差が付いたもの（UTC に直して扱う）も受け付ける。
- 時刻として解釈できない `timestamp` を持つアクションは、無効なアクションとして無視する。
- ブリッジが goPodder へ書き込む `timestamp` は、UTC の `YYYY-MM-DDTHH:MM:SS` 形式（小数秒なし、タイムゾーン表記なし）とする。
- 時刻の比較は秒単位で行う（小数秒、ABS の `lastUpdate` のミリ秒は切り捨てる）。

> 決めた経緯：[ADR 005](adr/005-invalid-actions.md)、[ADR 010](adr/010-timezone.md)

### 11.2 数値

- `position`、`total`、`started` は秒単位の数値として扱う。整数でなくても受け付ける。
- 数値として解釈できない値を持つアクションは、無効なアクションとして無視する。

> 決めた経緯：[ADR 005](adr/005-invalid-actions.md)
