package scenario

import (
	"fmt"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/fake"
)

// ---------- Arrange：ブリッジの設定 ----------

// Env はブリッジの環境変数を追加・上書きする。
func (s *S) Env(key, value string) *S {
	s.env[key] = value
	s.d.addGiven(Bridge, key+"="+value)
	return s
}

// TrailingSlashURLs は GOPODDER_URL と ABS_URL の末尾に / を付ける。
func (s *S) TrailingSlashURLs() *S {
	s.trailingSlash = true
	s.d.addGiven(Bridge, "GOPODDER_URL・ABS_URL の末尾に /")
	return s
}

// DefaultListenPort は LISTEN_PORT を指定せずに起動する（仕様の既定値で待ち受けるはず）。
func (s *S) DefaultListenPort() *S {
	s.defaultPort = true
	s.d.addGiven(Bridge, "LISTEN_PORT なし")
	return s
}

// AdminToken は ABS_ADMIN_TOKEN を設定する。
func (s *S) AdminToken() *S {
	s.admin = true
	s.d.addGiven(Bridge, "ABS_ADMIN_TOKEN あり")
	return s
}

// AdminTokenRejected は ABS_ADMIN_TOKEN を設定するが、ABS は /api/feeds を 403 で断る。
func (s *S) AdminTokenRejected() *S {
	s.admin, s.adminRejected = true, true
	s.d.addGiven(Bridge, "ABS_ADMIN_TOKEN あり")
	s.d.addGiven(ABS, "/api/feeds → 403")
	return s
}

// NoIndexWait は、索引ができるのを待たずに Act に進む。
func (s *S) NoIndexWait() *S { s.noIndexWait = true; return s }

// ---------- Arrange：ABS ----------

func episodeLine(e Episode) string {
	u, g := "URL なし", "GUID なし"
	if e.EnclosureURL != "" {
		u = compactURL(e.EnclosureURL)
	}
	if e.GUID != "" {
		g = e.GUID
	}
	s := fmt.Sprintf("%s: %s / %s", e.Key, u, g)
	if e.Duration != 600 {
		s += fmt.Sprintf(" 長さ %s", num(e.Duration))
	}
	if e.LibraryID != "lib-podcast" {
		s += "（" + e.LibraryID + "）"
	}
	return s
}

// ABSEpisode は ABS にエピソードを置く。
func (s *S) ABSEpisode(eps ...Episode) *S {
	for _, e := range eps {
		s.episodes = append(s.episodes, e)
		s.d.addGiven(ABS, episodeLine(e))
	}
	return s
}

// ABSHiddenEpisode は、ABS_TOKEN のユーザーには見えないエピソードを置く。
func (s *S) ABSHiddenEpisode(eps ...Episode) *S {
	for _, e := range eps {
		s.hidden = append(s.hidden, e)
		s.d.addGiven(ABS, episodeLine(e)+"（見えない）")
	}
	return s
}

// ABSProgress は ABS に進捗を置く。
func (s *S) ABSProgress(ps ...Progress) *S {
	for _, p := range ps {
		s.progress = append(s.progress, p)
		s.d.addGiven(ABS, p.label())
	}
	return s
}

// ABSFeed は、ViaABSFeed で作ったエピソードの RSS フィードを開いておく（/api/feeds に出る）。
func (s *S) ABSFeed(eps ...Episode) *S {
	for _, e := range eps {
		if e.FeedEpisodeID == "" {
			panic("ABSFeed には ViaABSFeed で作ったエピソードを渡す")
		}
		s.feeds = append(s.feeds, e)
		s.d.addGiven(ABS, fmt.Sprintf("フィード %s: %s → %s のファイル", e.FeedSlug, e.FeedEpisodeID, e.Key))
	}
	return s
}

// ABSBookLibrary は本のライブラリと、その中のアイテムを置く。
func (s *S) ABSBookLibrary(libraryID string, itemIDs ...string) *S {
	s.bookLibs[libraryID] = append(s.bookLibs[libraryID], itemIDs...)
	s.d.addGiven(ABS, fmt.Sprintf("本のライブラリ %s（%d アイテム）", libraryID, len(itemIDs)))
	return s
}

// ABSDown は ABS を止めておく。
func (s *S) ABSDown() *S {
	s.absDown = true
	s.d.addGiven(ABS, "停止中")
	return s
}

// ABSFails は、対象のリクエストにステータス status を返す。
func (s *S) ABSFails(t Target, status int) *S {
	s.absRules = append(s.absRules, ruleSpec{rule: fake.Rule{Match: t.match, Status: status}})
	s.d.addGiven(ABS, fmt.Sprintf("%s → %d", t.label, status))
	return s
}

// ABSFailsAfterStartup は、索引を作り終えたあと（Act から）、対象のリクエストにステータス status を返す。
func (s *S) ABSFailsAfterStartup(t Target, status int) *S {
	s.absRules = append(s.absRules, ruleSpec{rule: fake.Rule{Match: t.match, Status: status}, fromAct: true})
	s.d.addGiven(ABS, fmt.Sprintf("起動後の %s → %d", t.label, status))
	return s
}

// ABSHangs は、対象のリクエストに応答しない。
func (s *S) ABSHangs(t Target) *S {
	s.absRules = append(s.absRules, ruleSpec{rule: fake.Rule{Match: t.match, Hang: true}})
	s.d.addGiven(ABS, t.label+" → 応答しない")
	return s
}

// ABSDelays は、対象のリクエストへの応答を d だけ遅らせる。
func (s *S) ABSDelays(t Target, d time.Duration) *S {
	s.absRules = append(s.absRules, ruleSpec{rule: fake.Rule{Match: t.match, Delay: d}})
	s.d.addGiven(ABS, fmt.Sprintf("%s → %s 遅れて応答", t.label, d))
	return s
}

// ---------- Arrange：goPodder ----------

// GoPodderRecord は goPodder に記録を置く。
func (s *S) GoPodderRecord(as ...Action) *S {
	for _, a := range as {
		s.records = append(s.records, a)
		s.d.addGiven(GoPodder, a.label())
	}
	return s
}

// GoPodderDuplicateRecords は、同じ組の記録が複数ある状態にする（実物の goPodder では起きない）。
func (s *S) GoPodderDuplicateRecords(as ...Action) *S {
	for _, a := range as {
		s.duplicates = append(s.duplicates, a)
		s.d.addGiven(GoPodder, a.label()+"（同じ組）")
	}
	return s
}

// GoPodderResponds は、エピソードアクション以外のパスへの goPodder の応答を決める。
func (s *S) GoPodderResponds(p string, status int, header map[string]string, body string) *S {
	h := http.Header{}
	keys := make([]string, 0, len(header))
	for k, v := range header {
		h.Set(k, v)
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s.canned[p] = fake.Canned{Status: status, Header: h, Body: body}
	line := fmt.Sprintf("%s → %d", path.Base(p), status)
	if len(keys) > 0 {
		line += " + " + strings.Join(keys, ", ")
	}
	s.d.addGiven(GoPodder, line)
	return s
}

// GoPodderDown は goPodder を止めておく。
func (s *S) GoPodderDown() *S {
	s.gpDown = true
	s.d.addGiven(GoPodder, "停止中")
	return s
}

// GoPodderFails は、対象のリクエストにステータス status を返す。
func (s *S) GoPodderFails(t Target, status int) *S {
	s.gpRules = append(s.gpRules, ruleSpec{rule: fake.Rule{Match: t.match, Status: status}})
	s.d.addGiven(GoPodder, fmt.Sprintf("%s → %d", t.label, status))
	return s
}

// GoPodderHangs は、対象のリクエストに応答しない。
func (s *S) GoPodderHangs(t Target) *S {
	s.gpRules = append(s.gpRules, ruleSpec{rule: fake.Rule{Match: t.match, Hang: true}})
	s.d.addGiven(GoPodder, t.label+" → 応答しない")
	return s
}
