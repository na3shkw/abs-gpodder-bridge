package bridge

import (
	"encoding/json"
	"testing"
)

// ABS の POST /api/items/batch/get の libraryItems と同じ形。
const testItems = `[
  {"id":"li1","media":{"episodes":[
    {"id":"ep1","guid":"g1","duration":600,"enclosure":{"url":"HTTP://Example.COM:80/a/Ep1.mp3?x=Y"},"audioFile":{"metadata":{"path":"/p/1.mp3"}}},
    {"id":"ep2","guid":"g2","duration":300,"enclosure":{"url":"https://cdn.example.com:443/a/2.mp3"},"audioFile":{"metadata":{"path":"/p/2.mp3"}}},
    {"id":"ep3","guid":null,"duration":100,"enclosure":null,"audioFile":{"metadata":{"path":"/p/3.mp3"}}}
  ]}},
  {"id":"li2","media":{"episodes":[
    {"id":"ep4","guid":"gdup","enclosure":{"url":"http://dup.example.com/same.mp3"}},
    {"id":"ep5","guid":"g5","enclosure":{"url":"http://dup.example.com/same.mp3"}},
    {"id":"ep6","guid":"gdup","enclosure":{"url":"http://example.com/6.mp3"}},
    {"id":"ep7","guid":"g7","enclosure":{"url":"http://h/feed/s/item/f1/media.mp3"}}
  ]}}
]`

// ABS の GET /api/feeds の feeds と同じ形。
const testFeeds = `[
  {"entityType":"libraryItem","entityId":"li1","episodes":[
    {"id":"f1","fullPath":"/p/1.mp3"},
    {"id":"f3","fullPath":"/p/3.mp3"},
    {"id":"fx","fullPath":"/not/in/index.mp3"}
  ]},
  {"entityType":"collection","entityId":"c1","episodes":[
    {"id":"fc","fullPath":"/p/2.mp3"}
  ]}
]`

func indexFromJSON(t *testing.T, items, feeds string) *episodeIndex {
	t.Helper()
	var it []absItem
	if err := json.Unmarshal([]byte(items), &it); err != nil {
		t.Fatal(err)
	}
	var fd []absFeed
	if feeds != "" {
		if err := json.Unmarshal([]byte(feeds), &fd); err != nil {
			t.Fatal(err)
		}
	}
	return makeIndex(it, fd)
}

type matchCase struct {
	name, episode, guid string
	want                string // "libraryItemId/episodeId"。照合できなければ空
}

func runMatchCases(t *testing.T, ix *episodeIndex, tests []matchCase) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ix.match(tt.episode, tt.guid)

			got := ""
			if ok {
				got = ref.LibraryItemID + "/" + ref.EpisodeID
			}
			if got != tt.want {
				t.Errorf("match(%q, %q) = %q, want %q", tt.episode, tt.guid, got, tt.want)
			}
		})
	}
}

func TestMatch_フィード用IDと元の配信URLとGUIDの順に照合する(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		{"ABS のフィード URL のフィード用 ID で照合する", "http://abs.local:13378/feed/my-show/item/f3/media.mp3", "", "li1/ep3"},
		{"フィード URL はスキーム・ホスト・ポートを比べない", "https://Other:8443/feed/x/item/f3/media.m4a", "", "li1/ep3"},
		{"フィード用 ID が元の配信 URL より先に一致する", "http://h/feed/s/item/f1/media.mp3", "", "li1/ep1"},
		{"フィード URL の形でも対応表になければ元の配信 URL を試す", "http://h/feed/s/item/unknown/media.mp3", "g2", "li1/ep2"},
		{"元の配信 URL で照合する", "http://example.com/6.mp3", "", "li2/ep6"},
		{"元の配信 URL より GUID が後", "http://example.com/6.mp3", "g1", "li2/ep6"},
		{"元の配信 URL で一致しなければ GUID で照合する", "http://unknown/x.mp3", "g2", "li1/ep2"},
		{"どれにも一致しなければ照合できない", "http://unknown/x.mp3", "unknown", ""},
	})
}

func TestMatch_URLはスキームとホストの大文字小文字と既定ポートとフラグメントを無視して比べる(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		{"ホストの大文字小文字と http の既定ポート", "http://example.com/a/Ep1.mp3?x=Y", "", "li1/ep1"},
		{"スキームの大文字小文字", "Http://EXAMPLE.com:80/a/Ep1.mp3?x=Y", "", "li1/ep1"},
		{"フラグメントを取り除く", "http://example.com/a/Ep1.mp3?x=Y#t=10", "", "li1/ep1"},
		{"https の既定ポート", "https://CDN.example.com/a/2.mp3", "", "li1/ep2"},
		{"パスの大文字小文字は区別する", "http://example.com/a/ep1.mp3?x=Y", "", ""},
		{"クエリの大文字小文字は区別する", "http://example.com/a/Ep1.mp3?x=y", "", ""},
		{"クエリがなければ別の URL", "http://example.com/a/Ep1.mp3", "", ""},
		{"既定でないポートは取り除かない", "http://example.com:8080/a/Ep1.mp3?x=Y", "", ""},
		{"https の既定ポートは http では既定でない", "http://cdn.example.com:443/a/2.mp3", "", ""},
		{"スキームが違えば別の URL", "https://example.com/6.mp3", "", ""},
	})
}

func TestMatch_同じキーに複数のABSエピソードが一致したら照合できず次の手順も試さない(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		{"元の配信 URL が複数一致したら GUID を試さない", "http://dup.example.com/same.mp3", "g5", ""},
		{"GUID が複数一致したら照合できない", "http://unknown/x.mp3", "gdup", ""},
	})
}

func TestMatch_値のないABSエピソードはその手順で一致しない(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		// ep3 は guid も enclosure も null
		{"guid のないアクションは guid が null のエピソードに一致しない", "http://unknown/x.mp3", "", ""},
		{"空の episode は enclosure が null のエピソードに一致しない", "", "", ""},
	})
}

func TestMatch_フィード用IDの対応表は条件に合うフィードだけから作る(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		{"音声ファイルのパスが索引にないフィードのエピソードは対応づけない", "http://h/feed/s/item/fx/media.mp3", "", ""},
		{"entityType が libraryItem でないフィードは使わない", "http://h/feed/s/item/fc/media.mp3", "", ""},
	})
}

func TestMatch_フィード用IDの対応表がなければフィードURLの手順を飛ばす(t *testing.T) {
	ix := indexFromJSON(t, testItems, "") // ABS_ADMIN_TOKEN 未設定、または GET /api/feeds の失敗

	runMatchCases(t, ix, []matchCase{
		{"フィード URL でも照合しない", "http://h/feed/s/item/f3/media.mp3", "", ""},
		{"元の配信 URL として一致すればその ABS エピソード", "http://h/feed/s/item/f1/media.mp3", "", "li2/ep7"},
		{"GUID の手順は行う", "http://h/feed/s/item/f3/media.mp3", "g1", "li1/ep1"},
	})
}

func TestMatch_フィードURLの形でないパスはフィード用IDとして扱わない(t *testing.T) {
	ix := indexFromJSON(t, testItems, testFeeds)

	runMatchCases(t, ix, []matchCase{
		{"item の後にファイル名がない", "http://h/feed/s/item/f3", "", ""},
		{"先頭が /feed でない", "http://h/prefix/feed/s/item/f3/media.mp3", "", ""},
		{"item の位置が違う", "http://h/feed/item/f3/media.mp3", "", ""},
	})
}

func TestIndexHolder_索引を作る前は何も照合しない(t *testing.T) {
	var h indexHolder

	_, ok := h.get().match("http://example.com/a/Ep1.mp3?x=Y", "g1")

	if ok {
		t.Error("索引を作る前に照合できてしまった")
	}
}
