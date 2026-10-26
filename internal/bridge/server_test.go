package bridge_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"abs-gpodder-bridge/internal/bridge"
)

// ---- 偽の ABS ----

const (
	userToken  = "user-token"
	adminToken = "admin-token"
)

type progress struct {
	CurrentTime float64 `json:"currentTime"`
	Duration    float64 `json:"duration"`
	IsFinished  bool    `json:"isFinished"`
	LastUpdate  int64   `json:"lastUpdate"`
}

// fakeABS は 1 つのポッドキャストライブラリ（アイテム li1）を持つ ABS。
// 進捗は episodeId ごとに保存し、PATCH で書き換える。
type fakeABS struct {
	mu            sync.Mutex
	episodes      string // li1 の media.episodes（JSON）
	feeds         string // GET /api/feeds の feeds（JSON）
	progress      map[string]progress
	now           int64 // PATCH で lastUpdate に入れる時刻（ミリ秒）
	failLibraries bool
	failProgress  bool
}

func newFakeABS(episodes string) *fakeABS {
	return &fakeABS{episodes: episodes, feeds: "[]", progress: map[string]progress{}, now: 1893456000000}
}

func (f *fakeABS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := userToken
	if r.URL.Path == "/api/feeds" {
		want = adminToken
	}
	if r.Header.Get("Authorization") != "Bearer "+want {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/libraries" && !f.failLibraries:
		io.WriteString(w, `{"libraries":[{"id":"lib1","mediaType":"podcast"},{"id":"lib2","mediaType":"book"}]}`)
	case r.Method == "GET" && r.URL.Path == "/api/libraries/lib1/items":
		io.WriteString(w, `{"results":[{"id":"li1"}]}`)
	case r.Method == "POST" && r.URL.Path == "/api/items/batch/get":
		io.WriteString(w, `{"libraryItems":[{"id":"li1","media":{"episodes":`+f.episodes+`}}]}`)
	case r.Method == "GET" && r.URL.Path == "/api/feeds":
		io.WriteString(w, `{"feeds":`+f.feeds+`}`)
	case r.Method == "GET" && r.URL.Path == "/api/me/progress" && !f.failProgress:
		var list []map[string]any
		for id, p := range f.progress {
			list = append(list, map[string]any{"libraryItemId": "li1", "episodeId": id,
				"currentTime": p.CurrentTime, "duration": p.Duration, "isFinished": p.IsFinished, "lastUpdate": p.LastUpdate})
		}
		json.NewEncoder(w).Encode(map[string]any{"mediaProgress": list})
	case r.Method == "PATCH" && strings.HasPrefix(r.URL.Path, "/api/me/progress/li1/"):
		var body struct {
			CurrentTime float64  `json:"currentTime"`
			Duration    *float64 `json:"duration"`
			IsFinished  bool     `json:"isFinished"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		id := strings.TrimPrefix(r.URL.Path, "/api/me/progress/li1/")
		p := f.progress[id]
		p.CurrentTime, p.IsFinished, p.LastUpdate = body.CurrentTime, body.IsFinished, f.now
		if body.Duration != nil {
			p.Duration = *body.Duration
		}
		f.progress[id] = p
		io.WriteString(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeABS) set(fn func(*fakeABS)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeABS) progressOf(episodeID string) (progress, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.progress[episodeID]
	return p, ok
}

// ---- 偽の goPodder ----

// fakeGopodder は (podcast, episode) ごとにアクションを 1 件だけ保存する goPodder。
// エピソードアクション以外のパスへのリクエストは、受け取った内容を lastRequest に残す。
type fakeGopodder struct {
	mu          sync.Mutex
	keys        []string
	actions     map[string]map[string]any
	postStatus  int // 0 なら 200
	lastRequest *receivedRequest
}

type receivedRequest struct {
	Method, URI, Host, Body string
	Header                  http.Header
}

func newFakeGopodder() *fakeGopodder { return &fakeGopodder{actions: map[string]map[string]any{}} }

func (g *fakeGopodder) store(actions ...map[string]any) {
	for _, a := range actions {
		k := a["podcast"].(string) + " " + a["episode"].(string)
		if _, ok := g.actions[k]; !ok {
			g.keys = append(g.keys, k)
		}
		g.actions[k] = a
	}
}

func (g *fakeGopodder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	g.mu.Lock()
	defer g.mu.Unlock()
	if !strings.HasPrefix(r.URL.Path, "/api/2/episodes/") {
		g.lastRequest = &receivedRequest{r.Method, r.RequestURI, r.Host, string(body), r.Header.Clone()}
		w.Header().Set("Set-Cookie", "sessionid=from-gopodder")
		w.Header().Set("X-Gopodder", "yes")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, "gopodder-body")
		return
	}
	if r.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case "POST":
		if g.postStatus != 0 {
			w.WriteHeader(g.postStatus)
			return
		}
		var actions []map[string]any
		json.Unmarshal(body, &actions)
		g.store(actions...)
		io.WriteString(w, `{"timestamp":1,"update_urls":[]}`)
	case "GET":
		list := []map[string]any{}
		for _, k := range g.keys {
			list = append(list, g.actions[k])
		}
		json.NewEncoder(w).Encode(map[string]any{"actions": list, "timestamp": 1})
	}
}

func (g *fakeGopodder) actionOf(podcast, episode string) map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.actions[podcast+" "+episode]
}

// ---- ブリッジ ----

// startBridge はブリッジを起動し、索引を 1 回作ってから返す。
func startBridge(t *testing.T, abs *fakeABS, gp *fakeGopodder, env map[string]string) *httptest.Server {
	t.Helper()
	absSrv := httptest.NewServer(abs)
	t.Cleanup(absSrv.Close)
	gpSrv := httptest.NewServer(gp)
	t.Cleanup(gpSrv.Close)
	vars := map[string]string{
		"GOPODDER_URL":    gpSrv.URL,
		"ABS_URL":         absSrv.URL,
		"ABS_TOKEN":       userToken,
		"ABS_ADMIN_TOKEN": adminToken,
	}
	maps.Copy(vars, env)
	cfg, err := bridge.LoadConfig(func(k string) string { return vars[k] })
	if err != nil {
		t.Fatal(err)
	}
	srv := bridge.NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RebuildIndex(context.Background())
	b := httptest.NewServer(srv)
	t.Cleanup(b.Close)
	return b
}

// send はクライアントとしてブリッジへリクエストを送り、応答のステータスと本文を返す。
func send(t *testing.T, b *httptest.Server, method, path, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, b.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	return resp, string(respBody)
}

const episodes = `[
  {"id":"ep1","guid":"g1","duration":600,"enclosure":{"url":"http://example.com/1.mp3"},"audioFile":{"metadata":{"path":"/p/1.mp3"}}},
  {"id":"ep2","guid":"g2","duration":300,"enclosure":{"url":"http://example.com/2.mp3"},"audioFile":{"metadata":{"path":"/p/2.mp3"}}}
]`

// ---- テスト ----

func TestHealthz_goPodderへ中継せずにokを返す(t *testing.T) {
	gp := newFakeGopodder()
	b := startBridge(t, newFakeABS(episodes), gp, nil)

	resp, body := send(t, b, "GET", "/healthz", "")

	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/json" || body != `{"status":"ok"}` {
		t.Errorf("got %d %q %s", resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
	if gp.lastRequest != nil {
		t.Errorf("goPodder へ中継された: %+v", gp.lastRequest)
	}
}

func TestProxy_リクエストを変えずにgoPodderへ中継する(t *testing.T) {
	gp := newFakeGopodder()
	b := startBridge(t, newFakeABS(episodes), gp, nil)
	req, _ := http.NewRequest("PUT", b.URL+"/api/2/devices/user/phone.json?a=1&b=%2F", strings.NewReader(`{"caption":"x"}`))
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	req.Header.Set("Cookie", "sessionid=abc")
	req.Header.Set("X-Custom", "kept")
	req.Header.Set("Connection", "X-Hop")
	req.Header.Set("X-Hop", "dropped")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	got := gp.lastRequest
	if got == nil {
		t.Fatal("goPodder へ中継されない")
	}
	if got.Method != "PUT" || got.URI != "/api/2/devices/user/phone.json?a=1&b=%2F" || got.Body != `{"caption":"x"}` {
		t.Errorf("メソッド・パス・クエリ・本文が変わった: %+v", got)
	}
	if got.Header.Get("Authorization") != "Basic dXNlcjpwYXNz" || got.Header.Get("Cookie") != "sessionid=abc" || got.Header.Get("X-Custom") != "kept" {
		t.Errorf("ヘッダーが転送されない: %v", got.Header)
	}
	if got.Header.Get("X-Hop") != "" {
		t.Errorf("Connection で指定された hop-by-hop ヘッダーが転送された: %v", got.Header)
	}
	if got.Host == strings.TrimPrefix(b.URL, "http://") {
		t.Errorf("Host が転送先に合わせて書き換えられていない: %s", got.Host)
	}
}

func TestProxy_goPodderの応答を変えずに返す(t *testing.T) {
	b := startBridge(t, newFakeABS(episodes), newFakeGopodder(), nil)

	resp, body := send(t, b, "GET", "/api/2/subscriptions/user.json", "")

	if resp.StatusCode != http.StatusAccepted || body != "gopodder-body" ||
		resp.Header.Get("Set-Cookie") != "sessionid=from-gopodder" || resp.Header.Get("X-Gopodder") != "yes" {
		t.Errorf("got %d %v %q", resp.StatusCode, resp.Header, body)
	}
}

func TestProxy_goPodderにつながらなければ502を返す(t *testing.T) {
	b := startBridge(t, newFakeABS(episodes), newFakeGopodder(), map[string]string{"GOPODDER_URL": "http://127.0.0.1:1"})

	resp, _ := send(t, b, "GET", "/api/2/subscriptions/user.json", "")

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
}

func TestPostEpisodes_playアクションをABSの進捗に反映しgoPodderの応答を返す(t *testing.T) {
	abs := newFakeABS(episodes)
	gp := newFakeGopodder()
	b := startBridge(t, abs, gp, nil)

	resp, body := send(t, b, "POST", "/api/2/episodes/anyone.json", `[
	  {"podcast":"p","episode":"http://example.com/1.mp3","action":"play","timestamp":"2024-01-01T00:00:00","position":120,"total":600},
	  {"podcast":"p","episode":"http://example.com/2.mp3","action":"play","timestamp":"2024-01-01T00:00:00","position":300,"total":300}
	]`)

	if resp.StatusCode != 200 || body != `{"timestamp":1,"update_urls":[]}` {
		t.Errorf("goPodder の応答がそのまま返らない: %d %s", resp.StatusCode, body)
	}
	if p, _ := abs.progressOf("ep1"); p.CurrentTime != 120 || p.Duration != 600 || p.IsFinished {
		t.Errorf("ep1 の進捗 = %+v", p)
	}
	if p, _ := abs.progressOf("ep2"); p.CurrentTime != 300 || !p.IsFinished {
		t.Errorf("ep2 の進捗 = %+v", p)
	}
	if gp.actionOf("p", "http://example.com/1.mp3") == nil {
		t.Error("goPodder にアクションが保存されない")
	}
}

func TestPostEpisodes_goPodderが2xx以外を返したらABSを更新しない(t *testing.T) {
	abs := newFakeABS(episodes)
	gp := newFakeGopodder()
	gp.postStatus = http.StatusInternalServerError
	b := startBridge(t, abs, gp, nil)

	resp, _ := send(t, b, "POST", "/api/2/episodes/user.json",
		`[{"podcast":"p","episode":"http://example.com/1.mp3","action":"play","position":120}]`)

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if p, ok := abs.progressOf("ep1"); ok {
		t.Errorf("ABS が更新された: %+v", p)
	}
}

func TestPostEpisodes_ABSの進捗の方が新しければ更新しない(t *testing.T) {
	abs := newFakeABS(episodes)
	abs.progress["ep1"] = progress{CurrentTime: 50, Duration: 600, LastUpdate: 1704067201000} // 2024-01-01T00:00:01Z
	b := startBridge(t, abs, newFakeGopodder(), nil)

	send(t, b, "POST", "/api/2/episodes/user.json",
		`[{"podcast":"p","episode":"http://example.com/1.mp3","action":"play","timestamp":"2024-01-01T00:00:00Z","position":120}]`)

	if p, _ := abs.progressOf("ep1"); p.CurrentTime != 50 {
		t.Errorf("ABS が更新された: %+v", p)
	}
}

func TestPostEpisodes_本文が配列でなければABSを更新せずgoPodderの応答を返す(t *testing.T) {
	abs := newFakeABS(episodes)
	b := startBridge(t, abs, newFakeGopodder(), nil)

	resp, body := send(t, b, "POST", "/api/2/episodes/user.json",
		`{"podcast":"p","episode":"http://example.com/1.mp3","action":"play","position":120}`)

	if resp.StatusCode != 200 || body != `{"timestamp":1,"update_urls":[]}` {
		t.Errorf("got %d %s", resp.StatusCode, body)
	}
	if p, ok := abs.progressOf("ep1"); ok {
		t.Errorf("ABS が更新された: %+v", p)
	}
}

func TestGetEpisodes_ABSの新しい進捗をgoPodderへ書き戻し同じ応答でクライアントに届ける(t *testing.T) {
	abs := newFakeABS(episodes)
	abs.progress["ep1"] = progress{CurrentTime: 250.8, Duration: 600, LastUpdate: 1704164645123} // 2024-01-02T03:04:05.123Z
	gp := newFakeGopodder()
	gp.store(map[string]any{"podcast": "p", "episode": "http://example.com/1.mp3", "guid": "g-client", "action": "download"})
	b := startBridge(t, abs, gp, map[string]string{"TZ": "Asia/Tokyo"})

	resp, body := send(t, b, "GET", "/api/2/episodes/user.json?since=100", "")

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got struct {
		Actions []map[string]any `json:"actions"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil || len(got.Actions) != 1 {
		t.Fatalf("応答 %s", body)
	}
	want := map[string]any{"podcast": "p", "episode": "http://example.com/1.mp3", "guid": "g-client", "action": "play",
		"timestamp": "2024-01-02T03:04:05", "started": 0.0, "position": 250.0, "total": 600.0}
	if !maps.Equal(got.Actions[0], want) {
		t.Errorf("クライアントに届いたアクション\n got  %v\n want %v", got.Actions[0], want)
	}
}

func TestGetEpisodes_goPodderの新しい再生状態をABSに反映する(t *testing.T) {
	abs := newFakeABS(episodes)
	abs.progress["ep2"] = progress{CurrentTime: 10, Duration: 300, LastUpdate: 1704067200000} // 2024-01-01T00:00:00Z
	gp := newFakeGopodder()
	// ep2 は GUID で照合され、goPodder の方が 1 秒新しい。ep1 は ABS に進捗がない。
	gp.store(map[string]any{"podcast": "p", "episode": "http://unknown/2.mp3", "guid": "g2",
		"action": "play", "timestamp": "2024-01-01T09:00:01+09:00", "position": 150, "total": 300})
	gp.store(map[string]any{"podcast": "p", "episode": "http://example.com/1.mp3",
		"action": "play", "timestamp": "2024-01-01T00:00:00", "position": 42})
	b := startBridge(t, abs, gp, nil)

	send(t, b, "GET", "/api/2/episodes/user.json", "")

	if p, _ := abs.progressOf("ep2"); p.CurrentTime != 150 || p.Duration != 300 {
		t.Errorf("ep2 の進捗 = %+v", p)
	}
	if p, ok := abs.progressOf("ep1"); !ok || p.CurrentTime != 42 {
		t.Errorf("ep1 の進捗 = %+v (%v)", p, ok)
	}
}

func TestGetEpisodes_ABSの進捗が取れなくても元のリクエストを中継する(t *testing.T) {
	abs := newFakeABS(episodes)
	abs.failProgress = true
	gp := newFakeGopodder()
	gp.store(map[string]any{"podcast": "p", "episode": "http://example.com/1.mp3",
		"action": "play", "timestamp": "2024-01-01T00:00:00", "position": 42})
	b := startBridge(t, abs, gp, nil)

	resp, body := send(t, b, "GET", "/api/2/episodes/user.json", "")

	if resp.StatusCode != 200 || !strings.Contains(body, `"position":42`) {
		t.Errorf("got %d %s", resp.StatusCode, body)
	}
}

func TestIndex_ABS_ADMIN_TOKENがなければフィードURLで照合しない(t *testing.T) {
	tests := []struct {
		name       string
		adminToken string
		wantSynced bool
	}{
		{"ABS_ADMIN_TOKEN があればフィード URL で照合する", adminToken, true},
		{"ABS_ADMIN_TOKEN がなければフィード URL で照合しない", "", false},
		{"GET /api/feeds が拒否されればフィード URL で照合しない", "wrong-token", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs := newFakeABS(episodes)
			abs.feeds = `[{"entityType":"libraryItem","entityId":"li1","episodes":[{"id":"feed-ep1","fullPath":"/p/1.mp3"}]}]`
			b := startBridge(t, abs, newFakeGopodder(), map[string]string{"ABS_ADMIN_TOKEN": tt.adminToken})

			send(t, b, "POST", "/api/2/episodes/user.json",
				`[{"podcast":"p","episode":"http://abs:13378/feed/show/item/feed-ep1/media.mp3","action":"play","position":77}]`)

			p, synced := abs.progressOf("ep1")
			if synced != tt.wantSynced || (synced && p.CurrentTime != 77) {
				t.Errorf("ep1 の進捗 = %+v (%v), want synced=%v", p, synced, tt.wantSynced)
			}
		})
	}
}

func TestIndex_作り直しに失敗したら前の索引を使い続ける(t *testing.T) {
	abs := newFakeABS(episodes)
	absSrv := httptest.NewServer(abs)
	t.Cleanup(absSrv.Close)
	gpSrv := httptest.NewServer(newFakeGopodder())
	t.Cleanup(gpSrv.Close)
	vars := map[string]string{"GOPODDER_URL": gpSrv.URL, "ABS_URL": absSrv.URL, "ABS_TOKEN": userToken}
	cfg, err := bridge.LoadConfig(func(k string) string { return vars[k] })
	if err != nil {
		t.Fatal(err)
	}
	srv := bridge.NewServer(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.RebuildIndex(context.Background())
	abs.set(func(f *fakeABS) { f.failLibraries = true })
	srv.RebuildIndex(context.Background())
	b := httptest.NewServer(srv)
	t.Cleanup(b.Close)

	send(t, b, "POST", "/api/2/episodes/user.json",
		`[{"podcast":"p","episode":"http://example.com/1.mp3","action":"play","position":33}]`)

	if p, ok := abs.progressOf("ep1"); !ok || p.CurrentTime != 33 {
		t.Errorf("前の索引で照合されない: %+v (%v)", p, ok)
	}
}
