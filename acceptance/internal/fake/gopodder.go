package fake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"
)

// Exchange は fake が受け取ったリクエストと、返した応答の記録。
type Exchange struct {
	Request
	Status   int
	RespBody []byte
	RespHdr  http.Header
	Done     time.Time // 応答を返し終えた時刻（まだなら zero）
}

type gpRecord struct {
	raw      map[string]any
	modified int64
	hash     string
}

// Canned は、エピソードアクション以外のパスに返す決まった応答。
type Canned struct {
	Status int
	Header http.Header
	Body   string
}

// GoPodder は goPodder（cbrgm/gopodder）のエピソードアクション API をまねる fake。
//   - (podcast, episode) ごとに記録を 1 件だけ持ち、受け取るたびに上書きする
//   - action・started・position・total が同じなら上書きしない
//   - GET ?since=N は、受け取った時刻（秒）が N 以上の記録を返す
//
// それ以外のパスには Canned の応答（未登録なら 200 と固定の本文）を返す。
type GoPodder struct {
	*httptest.Server

	mu        sync.Mutex
	records   []*gpRecord
	canned    map[string]Canned
	exchanges []Exchange
	rules     rules
}

var episodesPath = regexp.MustCompile(`^/api/2/episodes/[^/]+\.json$`)

func NewGoPodder(t testing.TB) *GoPodder {
	t.Helper()
	g := &GoPodder{canned: map[string]Canned{}}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	return g
}

// Seed は、最初から保存されている記録を入れる（受け取った時刻は大昔とする）。
func (g *GoPodder) Seed(action map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.upsert(action, 1)
}

// SeedDuplicate は、同じ (podcast, episode) でも上書きせずに記録を足す。
// 実物の goPodder では起きないが、ブリッジが複数の記録を正しく扱うかを確かめるために使う。
func (g *GoPodder) SeedDuplicate(action map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.records = append(g.records, &gpRecord{raw: action, modified: 1})
}

// Respond は、エピソードアクション以外のパスへの応答を決める。
func (g *GoPodder) Respond(path string, c Canned) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.canned[path] = c
}

// AddRule は故障を注入する。active=false なら ActivateRules まで効かない。
func (g *GoPodder) AddRule(r Rule, active bool) { g.rules.add(r, active) }
func (g *GoPodder) ActivateRules()              { g.rules.activateAll() }

func (g *GoPodder) Exchanges() []Exchange {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]Exchange(nil), g.exchanges...)
}

func key(a map[string]any) string {
	return fmt.Sprint(a["podcast"]) + "\x00" + fmt.Sprint(a["episode"])
}

func (g *GoPodder) upsert(a map[string]any, modified int64) {
	h := fmt.Sprint(a["action"], "|", a["started"], "|", a["position"], "|", a["total"])
	for _, r := range g.records {
		if key(r.raw) == key(a) {
			if r.hash != h {
				r.raw, r.modified, r.hash = a, modified, h
			}
			return
		}
	}
	g.records = append(g.records, &gpRecord{raw: a, modified: modified, hash: h})
}

func (g *GoPodder) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	// 届いた時点で記録し（応答しない場合も見えるように）、応答を返し終えたら結果を書き足す
	g.mu.Lock()
	idx := len(g.exchanges)
	g.exchanges = append(g.exchanges, Exchange{
		Request: Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Header: r.Header.Clone(), Body: body, Host: r.Host},
	})
	g.mu.Unlock()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	if !g.rules.apply(rec, r) {
		g.handle(rec, r, body)
	}
	g.mu.Lock()
	ex := &g.exchanges[idx]
	ex.Status, ex.RespBody, ex.RespHdr, ex.Done = rec.status, rec.buf.Bytes(), rec.Header().Clone(), time.Now()
	g.mu.Unlock()
}

func (g *GoPodder) handle(w http.ResponseWriter, r *http.Request, body []byte) {
	if !episodesPath.MatchString(r.URL.Path) {
		g.mu.Lock()
		c, ok := g.canned[r.URL.Path]
		g.mu.Unlock()
		if !ok {
			c = Canned{Status: http.StatusOK, Body: `{"from":"gopodder"}`}
		}
		for k, vs := range c.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.WriteHeader(c.Status)
		_, _ = io.WriteString(w, c.Body)
		return
	}
	now := time.Now().Unix()
	switch r.Method {
	case http.MethodGet:
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		g.mu.Lock()
		actions := []map[string]any{}
		for _, rec := range g.records {
			if rec.modified >= since {
				actions = append(actions, rec.raw)
			}
		}
		g.mu.Unlock()
		writeJSON(w, map[string]any{"actions": actions, "timestamp": now})
	case http.MethodPost:
		var actions []map[string]any
		if err := json.Unmarshal(body, &actions); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		g.mu.Lock()
		for _, a := range actions {
			g.upsert(a, now)
		}
		g.mu.Unlock()
		writeJSON(w, map[string]any{"timestamp": now, "update_urls": [][]string{}})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type recorder struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (r *recorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }
func (r *recorder) Write(p []byte) (int, error) {
	r.buf.Write(p)
	return r.ResponseWriter.Write(p)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
