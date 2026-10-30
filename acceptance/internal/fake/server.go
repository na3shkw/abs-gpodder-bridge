// Package fake は、ブリッジの接続先（goPodder・ABS）の代わりになる HTTP サーバーを提供する。
package fake

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Request は fake サーバーが受け取ったリクエストの記録。
type Request struct {
	Method string
	Path   string
	Query  string
	Header http.Header
	Body   []byte
	Host   string
}

// Server は受け取ったリクエストを記録し、Handle で登録した応答を返す。
// 登録のないリクエストには 404 を返す。
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	requests []Request
	routes   map[string]http.HandlerFunc // "METHOD /path"
	fallback http.HandlerFunc
}

// New は fake サーバーを起動する。終了はテストの Cleanup で行う。
func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{routes: map[string]http.HandlerFunc{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// Handle は "METHOD /path" に一致するリクエストへの応答を登録する。
func (s *Server) Handle(methodAndPath string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[methodAndPath] = h
}

// Fallback は、どの登録にも一致しないリクエストへの応答を登録する。
func (s *Server) Fallback(h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback = h
}

// Requests は受け取ったリクエストの記録を、受け取った順に返す。
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.requests = append(s.requests, Request{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Header: r.Header.Clone(),
		Body:   body,
	})
	h := s.routes[r.Method+" "+r.URL.Path]
	if h == nil {
		h = s.fallback
	}
	s.mu.Unlock()
	if h == nil {
		http.NotFound(w, r)
		return
	}
	h(w, r)
}
