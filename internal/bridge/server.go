package bridge

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Server はブリッジの HTTP ハンドラー。
type Server struct {
	cfg      *Config
	log      *slog.Logger
	gopodder *http.Client
	abs      *absClient
	index    indexHolder
}

func NewServer(cfg *Config, log *slog.Logger) *Server {
	return &Server{
		cfg: cfg,
		log: log,
		gopodder: &http.Client{
			Timeout: cfg.GopodderTimeout,
			// 応答は手を加えずに返すので、圧縮の展開もリダイレクトの追跡もしない。
			Transport:     &http.Transport{Proxy: nil, DisableCompression: true},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		abs: &absClient{
			baseURL:    cfg.ABSURL,
			token:      cfg.ABSToken,
			adminToken: cfg.ABSAdminToken,
			http:       &http.Client{Timeout: cfg.ABSTimeout},
		},
	}
}

// RunIndex は索引を起動時に作り、以後 ABS_INDEX_SCHEDULE の時刻に作り直す。ctx が終わるまで戻らない。
func (s *Server) RunIndex(ctx context.Context) {
	s.index.run(ctx, s.cfg.NextIndexTime, s.abs, s.log)
}

// RebuildIndex は索引を 1 回作り直す（6.3 節）。失敗したら前の索引を使い続ける。
func (s *Server) RebuildIndex(ctx context.Context) {
	s.index.rebuild(ctx, s.abs, s.log)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"status":"ok"}`)
	case r.Method == http.MethodPost && isEpisodesPath(r.URL.Path):
		s.handlePostEpisodes(w, r)
	case r.Method == http.MethodGet && isEpisodesPath(r.URL.Path):
		s.reconcile(r)
		s.proxy(w, r)
	default:
		s.proxy(w, r)
	}
}

// isEpisodesPath はパスが /api/2/episodes/{user}.json に完全一致するかを返す。
func isEpisodesPath(p string) bool {
	user, ok := strings.CutPrefix(p, "/api/2/episodes/")
	if !ok {
		return false
	}
	user, ok = strings.CutSuffix(user, ".json")
	return ok && user != "" && !strings.Contains(user, "/")
}

// hop-by-hop ヘッダー（RFC 9110 7.6.1）。
var hopHeaders = []string{
	"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate",
	"Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade",
}

func removeHopHeaders(h http.Header) {
	for _, v := range h.Values("Connection") {
		for _, name := range strings.Split(v, ",") {
			if name = strings.TrimSpace(name); name != "" {
				h.Del(name)
			}
		}
	}
	for _, name := range hopHeaders {
		h.Del(name)
	}
}

// forward はクライアントのリクエストを goPodder へ中継し、応答を返す（4 章）。
func (s *Server) forward(r *http.Request, body io.Reader, contentLength int64) (*http.Response, error) {
	target := s.cfg.GopodderURL + r.URL.EscapedPath()
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		target += "?" + r.URL.RawQuery
	}
	if contentLength == 0 {
		body = nil
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		return nil, err
	}
	out.Header = r.Header.Clone()
	removeHopHeaders(out.Header)
	if _, ok := out.Header["User-Agent"]; !ok {
		out.Header.Set("User-Agent", "") // Go の既定の User-Agent を付けない
	}
	out.ContentLength = contentLength
	return s.gopodder.Do(out)
}

func (s *Server) proxy(w http.ResponseWriter, r *http.Request) {
	resp, err := s.forward(r, r.Body, r.ContentLength)
	if err != nil {
		s.badGateway(w, err)
		return
	}
	defer resp.Body.Close()
	writeResponseHeader(w, resp)
	if _, err := io.Copy(w, resp.Body); err != nil {
		s.log.Warn("copying goPodder response failed", "error", err)
	}
}

func (s *Server) badGateway(w http.ResponseWriter, err error) {
	s.log.Error("forwarding to goPodder failed", "error", err)
	w.WriteHeader(http.StatusBadGateway)
}

func writeResponseHeader(w http.ResponseWriter, resp *http.Response) {
	removeHopHeaders(resp.Header)
	for k, vs := range resp.Header {
		w.Header()[k] = vs
	}
	w.WriteHeader(resp.StatusCode)
}

// handlePostEpisodes は 5.1 節の処理を行う。
func (s *Server) handlePostEpisodes(w http.ResponseWriter, r *http.Request) {
	received := time.Now()
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Warn("reading request body failed", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	resp, err := s.forward(r, bytes.NewReader(reqBody), int64(len(reqBody)))
	if err != nil {
		s.badGateway(w, err)
		return
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		s.badGateway(w, err)
		return
	}
	if resp.StatusCode/100 == 2 {
		s.applyPostedActions(r.Context(), reqBody, received.Unix())
	}
	writeResponseHeader(w, resp)
	w.Write(respBody)
}

// absUpdateBody は 5.3 節の PATCH の本文を作る。
func absUpdateBody(a *action) map[string]any {
	body := map[string]any{"currentTime": a.Position}
	if a.HasTotal && a.Total > 0 {
		body["duration"] = a.Total
	}
	if a.finished() {
		body["isFinished"] = true
	}
	return body
}

func (s *Server) updateABS(ctx context.Context, ref episodeRef, a *action) {
	if err := s.abs.updateProgress(ctx, ref, absUpdateBody(a)); err != nil {
		s.log.Warn("updating ABS progress failed", "libraryItemId", ref.LibraryItemID, "episodeId", ref.EpisodeID, "error", err)
		return
	}
	s.log.Info("updated ABS progress", "libraryItemId", ref.LibraryItemID, "episodeId", ref.EpisodeID, "position", a.Position)
}
