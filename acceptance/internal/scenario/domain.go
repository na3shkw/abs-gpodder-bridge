package scenario

import (
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// baseTime はシナリオの時刻の基準。T(分) はこの時刻からの経過分で表す。
var baseTime = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

// T は基準時刻から min 分後の時刻を返す。図では「09:05」のように表示する。
func T(min int) time.Time { return baseTime.Add(time.Duration(min) * time.Minute) }

func clock(t time.Time) string {
	t = t.UTC()
	s := t.Format("15:04")
	if t.Second() != 0 || t.Nanosecond() != 0 {
		s = t.Format("15:04:05")
		if t.Nanosecond() != 0 {
			s += fmt.Sprintf(".%03d", t.Nanosecond()/1e6)
		}
	}
	return s
}

// Episode は ABS のポッドキャストエピソードと、クライアントから見たその URL をまとめたもの。
type Episode struct {
	Key       string // 図で使う短い名前（例：ep1）
	LibraryID string
	ItemID    string
	EpisodeID string

	EnclosureURL string  // ABS が持つ元の配信 URL（空なら null）
	GUID         string  // ABS が持つ GUID（空なら null）
	Duration     float64 // 音声ファイルの長さ（秒）
	Path         string  // 音声ファイルのパス

	PodcastURL string // クライアントが購読しているフィードの URL
	ClientURL  string // クライアントが知っている音声 URL（空なら EnclosureURL）

	FeedSlug      string // ABS のフィード経由のとき、フィードの slug
	FeedEpisodeID string // ABS のフィード経由のとき、フィード用 ID
}

func (e Episode) clientURL() string {
	if e.ClientURL != "" {
		return e.ClientURL
	}
	return e.EnclosureURL
}

// NewEpisode は、図で使う名前 key から、ほかの値を決めたエピソードを作る。
func NewEpisode(key string) Episode {
	return Episode{
		Key:          key,
		LibraryID:    "lib-podcast",
		ItemID:       "item-" + key,
		EpisodeID:    "episode-" + key,
		EnclosureURL: "https://cdn.example.com/show/" + key + ".mp3",
		GUID:         "guid-" + key,
		Duration:     600,
		Path:         "/podcasts/show/" + key + ".mp3",
		PodcastURL:   "https://feeds.example.com/show.rss",
	}
}

// ViaABSFeed は、クライアントが ABS のフィード（slug）経由で購読しているときのエピソードを返す。
// クライアントの URL は /feed/{slug}/item/{フィード用 ID}/media.mp3 になる。
func (e Episode) ViaABSFeed(host, slug string) Episode {
	e.FeedSlug = slug
	e.FeedEpisodeID = "fe-" + e.Key
	e.PodcastURL = host + "/feed/" + slug
	e.ClientURL = host + "/feed/" + slug + "/item/" + e.FeedEpisodeID + "/media.mp3"
	return e
}

// ABSHost は、図や例で使う ABS のホスト。
const ABSHost = "https://abs.example.com"

// Action は gPodder のエピソードアクション。
type Action struct {
	ep      Episode
	Podcast string
	Episode string
	GUID    string
	Kind    string
	ts      *time.Time
	started *float64
	pos     *float64
	total   *float64
	raw     map[string]any // そのまま上書きする値（不正な値を作るため）
}

// Play は ep の play アクションを作る。位置なし、時刻なし。
func Play(ep Episode) Action {
	return Action{ep: ep, Podcast: ep.PodcastURL, Episode: ep.clientURL(), Kind: "play"}
}

// Of は ep の、play 以外の種類（download・delete など）のアクションを作る。
func Of(ep Episode, kind string) Action {
	return Action{ep: ep, Podcast: ep.PodcastURL, Episode: ep.clientURL(), Kind: kind}
}

func f(v float64) *float64 { return &v }

func (a Action) Pos(sec float64) Action   { a.pos = f(sec); return a }
func (a Action) Total(sec float64) Action { a.total = f(sec); a.started = f(0); return a }
func (a Action) At(t time.Time) Action    { a.ts = &t; return a }
func (a Action) WithGUID(g string) Action { a.GUID = g; return a }
func (a Action) URL(u string) Action      { a.Episode = u; return a }

// Raw は JSON の項目 k をそのまま v にする（不正な値の記録を作るため）。
func (a Action) Raw(k string, v any) Action {
	m := map[string]any{}
	for kk, vv := range a.raw {
		m[kk] = vv
	}
	m[k] = v
	a.raw = m
	return a
}

// JSON は gPodder API の形に変換する。
func (a Action) JSON() map[string]any {
	m := map[string]any{"podcast": a.Podcast, "episode": a.Episode, "action": a.Kind}
	if a.GUID != "" {
		m["guid"] = a.GUID
	}
	if a.ts != nil {
		m["timestamp"] = a.ts.UTC().Format("2006-01-02T15:04:05")
	}
	if a.started != nil {
		m["started"] = *a.started
	}
	if a.pos != nil {
		m["position"] = *a.pos
	}
	if a.total != nil {
		m["total"] = *a.total
	}
	for k, v := range a.raw {
		m[k] = v
	}
	return m
}

// label は図に書く短い表記（例：ep1 play 200/600 09:02）。
func (a Action) label() string {
	s := a.ep.Key + " " + a.Kind
	if _, ok := a.raw["position"]; !ok && a.pos != nil {
		s += " " + num(*a.pos)
		if a.total != nil {
			s += "/" + num(*a.total)
		}
	}
	if _, ok := a.raw["timestamp"]; !ok {
		if a.ts != nil {
			s += " " + clock(*a.ts)
		} else {
			s += " 時刻なし"
		}
	}
	if a.Episode != a.ep.clientURL() {
		s += " URL=" + compactURL(a.Episode)
	} else if a.ep.FeedEpisodeID != "" {
		s += "（フィード URL）"
	}
	if a.GUID != "" {
		s += " guid=" + a.GUID
	}
	keys := make([]string, 0, len(a.raw))
	for k := range a.raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s += " " + k + "=" + valueString(a.raw[k])
	}
	return s
}

// compactURL は、図に書くために URL を短くする。
func compactURL(u string) string {
	if strings.HasPrefix(u, "https://cdn.example.com/show/") {
		return path.Base(u)
	}
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return strings.TrimPrefix(u, p.Scheme+"://")
	}
	return u
}

// Progress は ABS の進捗。
type Progress struct {
	Ep          Episode
	CurrentTime float64
	Duration    float64
	Finished    bool
	LastUpdate  time.Time
}

func (p Progress) label() string {
	s := fmt.Sprintf("%s 進捗 %s", p.Ep.Key, num(p.CurrentTime))
	if p.Duration > 0 {
		s += "/" + num(p.Duration)
	} else {
		s += "（長さ 0）"
	}
	if p.Finished {
		s += " 完了"
	}
	return s + " " + clock(p.LastUpdate)
}

// KV は順序を保った本文の 1 項目。
type KV struct {
	K string
	V any
}

// Body は順序を保った JSON 本文。図ではこの順に表示する。
type Body []KV

// B は "k1", v1, "k2", v2 ... から Body を作る。
func B(kv ...any) Body {
	var b Body
	for i := 0; i+1 < len(kv); i += 2 {
		b = append(b, KV{K: kv[i].(string), V: kv[i+1]})
	}
	return b
}

func (b Body) String() string {
	s := "{"
	for i, kv := range b {
		if i > 0 {
			s += ", "
		}
		s += kv.K + ": " + valueString(kv.V)
	}
	return s + "}"
}

func valueString(v any) string {
	switch x := v.(type) {
	case float64:
		return num(x)
	case int:
		return strconv.Itoa(x)
	case string:
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case nil:
		return "null"
	}
	return fmt.Sprint(v)
}

func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
