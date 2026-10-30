package fake

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// ABSEpisode は fake ABS が持つポッドキャストエピソード。
type ABSEpisode struct {
	LibraryID    string
	ItemID       string
	EpisodeID    string
	EnclosureURL string // 空なら null
	GUID         string // 空なら null
	Duration     float64
	Path         string
	Hidden       bool // ABS_TOKEN のユーザーには一覧に出ない（進捗の更新はできてしまう）
}

// ABSProgress は fake ABS が持つ進捗。LastUpdate はミリ秒。
type ABSProgress struct {
	ItemID      string
	EpisodeID   string
	CurrentTime float64
	Duration    float64
	IsFinished  bool
	LastUpdate  int64
}

// ABSFeedEpisode は開いている RSS フィードの 1 エピソード。
type ABSFeedEpisode struct {
	ID       string // フィード用 ID
	FullPath string
}

// ABSFeed は開いている RSS フィード。
type ABSFeed struct {
	Slug     string
	ItemID   string
	Episodes []ABSFeedEpisode
}

// ABS は audiobookshelf（v2.36 以上）の、ブリッジが使う API をまねる fake。
// PATCH /api/me/progress の振る舞いは ABS の applyProgressUpdate に合わせる。
type ABS struct {
	*httptest.Server

	UserToken  string
	AdminToken string

	mu        sync.Mutex
	libraries map[string]string // id → mediaType
	bookItems map[string][]string
	episodes  []ABSEpisode
	feeds     []ABSFeed
	progress  map[string]*ABSProgress // episodeId → 進捗
	exchanges []Exchange
	batchGets int
	rules     rules
}

func NewABS(t testing.TB, userToken, adminToken string) *ABS {
	t.Helper()
	a := &ABS{
		UserToken:  userToken,
		AdminToken: adminToken,
		libraries:  map[string]string{},
		bookItems:  map[string][]string{},
		progress:   map[string]*ABSProgress{},
	}
	a.Server = httptest.NewServer(http.HandlerFunc(a.serve))
	t.Cleanup(a.Close)
	return a
}

func (a *ABS) AddEpisode(e ABSEpisode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.libraries[e.LibraryID]; !ok {
		a.libraries[e.LibraryID] = "podcast"
	}
	a.episodes = append(a.episodes, e)
}

// RemoveEpisode は一覧からエピソードを消す（進捗の更新も 404 になる）。
func (a *ABS) RemoveEpisode(episodeID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.episodes[:0]
	for _, e := range a.episodes {
		if e.EpisodeID != episodeID {
			out = append(out, e)
		}
	}
	a.episodes = out
}

// AddBookLibrary は本のライブラリと、その中のアイテムを置く。
func (a *ABS) AddBookLibrary(libraryID string, itemIDs ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.libraries[libraryID] = "book"
	a.bookItems[libraryID] = append(a.bookItems[libraryID], itemIDs...)
}

func (a *ABS) AddFeed(f ABSFeed) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.feeds = append(a.feeds, f)
}

func (a *ABS) SetProgress(p ABSProgress) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cp := p
	a.progress[p.EpisodeID] = &cp
}

func (a *ABS) AddRule(r Rule, active bool) { a.rules.add(r, active) }
func (a *ABS) ActivateRules()              { a.rules.activateAll() }

// BatchGets は、これまでに batch/get（索引の作成）が呼ばれた回数を返す。
func (a *ABS) BatchGets() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.batchGets
}

// WaitBatchGets は、batch/get の回数が n 以上になるまで待つ。
func (a *ABS) WaitBatchGets(n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if a.BatchGets() >= n {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// WaitRequest は、path へのリクエストが（since 以降に）届くまで待つ。
func (a *ABS) WaitRequest(path string, since int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ex := a.Exchanges()
		for _, e := range ex[min(since, len(ex)):] {
			if e.Path == path {
				return true
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (a *ABS) Exchanges() []Exchange {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]Exchange(nil), a.exchanges...)
}

func (a *ABS) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	// 届いた時点で記録し（応答しない場合も見えるように）、応答を返し終えたら結果を書き足す
	a.mu.Lock()
	idx := len(a.exchanges)
	a.exchanges = append(a.exchanges, Exchange{
		Request: Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body, Host: r.Host},
	})
	a.mu.Unlock()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	if !a.rules.apply(rec, r) {
		a.handle(rec, r, body)
	}
	a.mu.Lock()
	ex := &a.exchanges[idx]
	ex.Status, ex.RespBody, ex.RespHdr, ex.Done = rec.status, rec.buf.Bytes(), rec.Header().Clone(), time.Now()
	a.mu.Unlock()
}

func (a *ABS) handle(w http.ResponseWriter, r *http.Request, body []byte) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	p := r.URL.Path
	if p == "/api/feeds" {
		if a.AdminToken == "" || token != a.AdminToken {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	} else if token != a.UserToken {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && p == "/api/libraries":
		libs := []map[string]any{}
		for _, id := range sortedKeys(a.libraries) {
			libs = append(libs, map[string]any{"id": id, "name": id, "mediaType": a.libraries[id]})
		}
		writeJSON(w, map[string]any{"libraries": libs})

	case r.Method == http.MethodGet && strings.HasPrefix(p, "/api/libraries/") && strings.HasSuffix(p, "/items"):
		lib := strings.TrimSuffix(strings.TrimPrefix(p, "/api/libraries/"), "/items")
		results := []map[string]any{}
		if a.libraries[lib] == "book" {
			for _, id := range a.bookItems[lib] {
				results = append(results, map[string]any{"id": id, "libraryId": lib, "mediaType": "book", "updatedAt": 0})
			}
		} else {
			seen := map[string]bool{}
			for _, e := range a.episodes {
				if e.LibraryID == lib && !e.Hidden && !seen[e.ItemID] {
					seen[e.ItemID] = true
					results = append(results, map[string]any{"id": e.ItemID, "libraryId": lib, "mediaType": "podcast", "updatedAt": 0})
				}
			}
		}
		writeJSON(w, map[string]any{"results": results, "total": len(results), "limit": 0, "page": 0})

	case r.Method == http.MethodPost && p == "/api/items/batch/get":
		var req struct {
			LibraryItemIDs []string `json:"libraryItemIds"`
		}
		_ = json.Unmarshal(body, &req)
		items := []map[string]any{}
		for _, id := range req.LibraryItemIDs {
			var lib string
			eps := []map[string]any{}
			for _, e := range a.episodes {
				if e.ItemID != id || e.Hidden {
					continue
				}
				lib = e.LibraryID
				var enclosure any
				if e.EnclosureURL != "" {
					enclosure = map[string]any{"url": e.EnclosureURL, "type": "audio/mpeg", "length": "0"}
				}
				var guid any
				if e.GUID != "" {
					guid = e.GUID
				}
				eps = append(eps, map[string]any{
					"libraryItemId": id, "id": e.EpisodeID, "enclosure": enclosure, "guid": guid,
					"duration": e.Duration, "audioFile": map[string]any{"metadata": map[string]any{"path": e.Path}},
				})
			}
			if lib == "" {
				continue
			}
			items = append(items, map[string]any{"id": id, "libraryId": lib, "mediaType": "podcast", "media": map[string]any{"episodes": eps}})
		}
		a.batchGets++
		writeJSON(w, map[string]any{"libraryItems": items})

	case r.Method == http.MethodGet && p == "/api/feeds":
		feeds := []map[string]any{}
		for _, f := range a.feeds {
			eps := []map[string]any{}
			for _, e := range f.Episodes {
				eps = append(eps, map[string]any{"id": e.ID, "fullPath": e.FullPath})
			}
			feeds = append(feeds, map[string]any{"id": "feed-" + f.Slug, "slug": f.Slug, "entityType": "libraryItem", "entityId": f.ItemID, "episodes": eps})
		}
		writeJSON(w, map[string]any{"feeds": feeds, "minified": []any{}})

	case r.Method == http.MethodGet && p == "/api/me/progress":
		list := []map[string]any{}
		for _, id := range sortedKeysP(a.progress) {
			pr := a.progress[id]
			list = append(list, map[string]any{
				"libraryItemId": pr.ItemID, "episodeId": pr.EpisodeID, "mediaItemType": "podcastEpisode",
				"duration": pr.Duration, "currentTime": pr.CurrentTime, "isFinished": pr.IsFinished, "lastUpdate": pr.LastUpdate,
			})
		}
		writeJSON(w, map[string]any{"mediaProgress": list})

	case r.Method == http.MethodPatch && strings.HasPrefix(p, "/api/me/progress/"):
		parts := strings.Split(strings.TrimPrefix(p, "/api/me/progress/"), "/")
		if len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid", http.StatusBadRequest)
			return
		}
		if !a.applyProgress(parts[0], parts[1], payload) {
			http.Error(w, "Episode not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)

	default:
		http.NotFound(w, r)
	}
}

// applyProgress は ABS の進捗の作成・更新の規則をまねる。
func (a *ABS) applyProgress(itemID, episodeID string, payload map[string]any) bool {
	found := false
	for _, e := range a.episodes {
		if e.EpisodeID == episodeID {
			found = true
		}
	}
	if !found {
		return false
	}
	now := time.Now().UnixMilli()
	num := func(k string) (float64, bool) { v, ok := payload[k].(float64); return v, ok }
	pr, ok := a.progress[episodeID]
	if !ok {
		np := &ABSProgress{ItemID: itemID, EpisodeID: episodeID, LastUpdate: now}
		np.Duration, _ = num("duration")
		np.CurrentTime, _ = num("currentTime")
		np.IsFinished, _ = payload["isFinished"].(bool)
		a.progress[episodeID] = np
		return true
	}
	before := *pr
	if fin, ok := payload["isFinished"].(bool); ok {
		if !fin && pr.IsFinished {
			pr.IsFinished = false
			pr.CurrentTime = 0
			delete(payload, "currentTime")
		} else {
			pr.IsFinished = fin
		}
	}
	ctChanged := false
	if v, ok := num("currentTime"); ok {
		ctChanged = v != pr.CurrentTime
		pr.CurrentTime = v
	}
	if v, ok := num("duration"); ok {
		pr.Duration = v
	}
	should := pr.Duration > 0 && pr.Duration-pr.CurrentTime < 10
	if !pr.IsFinished && should {
		pr.IsFinished = true
	} else if pr.IsFinished && ctChanged && !should {
		pr.IsFinished = false
	}
	if *pr != before {
		pr.LastUpdate = now
	}
	return true
}

func sortedKeys(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func sortedKeysP(m map[string]*ABSProgress) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
