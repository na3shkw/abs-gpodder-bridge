// Package scenario は、e2e テストを Arrange・Act・Assert で書くための DSL。
//
// 同じ記述から、テストの実行（Run モード）と、テスト 1 件につき 1 枚の図の生成
// （write / check モード）の両方を行う。図とテストがずれないよう、Assert はすべて
// この DSL で書き、t.Error などを直接呼ばないこと。
//
// DSL の各メソッドは、「図に描く内容」と「実行する・確かめる処理」を必ず組で持つ。
package scenario

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/fake"
	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/harness"
)

const (
	userToken     = "abs-user-token"
	adminToken    = "abs-admin-token"
	clientUser    = "alice"
	clientAuth    = "Basic YWxpY2U6c2VjcmV0" // alice:secret
	clientCookie  = "sessionid=abc123"
	defaultSched  = "@every 1h"
	indexWaitTime = 10 * time.Second
)

// S は 1 件のテストシナリオ。
type S struct {
	t     *testing.T
	id    string
	title string
	d     diagram

	// Arrange
	env           map[string]string
	trailingSlash bool
	defaultPort   bool
	admin         bool
	adminRejected bool
	noIndexWait   bool
	gpDown        bool
	absDown       bool
	episodes      []Episode
	hidden        []Episode
	progress      []Progress
	records       []Action
	duplicates    []Action
	feeds         []Episode
	bookLibs      map[string][]string
	canned        map[string]fake.Canned
	gpRules       []ruleSpec
	absRules      []ruleSpec
	manualStart   bool

	whens     []func(*runCtx)
	thens     []func(*runCtx)
	finalized bool
}

type ruleSpec struct {
	rule    fake.Rule
	fromAct bool
}

type runCtx struct {
	s       *S
	gp      *fake.GoPodder
	abs     *fake.ABS
	bridge  *harness.Bridge
	baseEnv harness.Env
	gpMark  int
	absMark int
	reqs    []clientResult
	starts  []startResult
}

type clientResult struct {
	method, path, query string
	header              http.Header
	body                []byte
	status              int
	respHeader          http.Header
	respBody            []byte
	err                 error
	start, end          time.Time
}

type startResult struct {
	label   string
	started bool
	code    int
}

// New はシナリオを始める。id はテスト設計の ID（例：PST-09）、title は何を確かめるか。
func New(t *testing.T, id, title string) *S {
	t.Helper()
	s := &S{t: t, id: id, title: title, env: map[string]string{}, bookLibs: map[string][]string{}, canned: map[string]fake.Canned{}}
	t.Cleanup(func() {
		if !s.finalized && !t.Failed() {
			t.Errorf("scenario %s: Run を呼んでいません", id)
		}
	})
	return s
}

// Run はシナリオを実行する。図の生成モードでは実行せず、図を登録するだけ。
func (s *S) Run() {
	s.t.Helper()
	s.finalized = true
	if len(s.episodes) > 0 && len(s.progress) == 0 && !s.absDown {
		s.d.addGiven(ABS, "進捗なし")
	}
	if len(s.records) == 0 && len(s.duplicates) == 0 && !s.gpDown {
		s.d.addGiven(GoPodder, "記録なし")
	}
	if mode() != modeRun {
		register(s)
		return
	}
	c := s.arrange()
	if s.t.Failed() {
		return
	}
	c.gpMark = len(c.gp.Exchanges())
	c.absMark = len(c.abs.Exchanges())
	c.gp.ActivateRules()
	c.abs.ActivateRules()
	for _, w := range s.whens {
		w(c)
		if s.t.Failed() {
			return
		}
	}
	for _, th := range s.thens {
		th(c)
	}
}

// arrange は fake を用意してブリッジを起動し、索引ができるまで待つ。
func (s *S) arrange() *runCtx {
	c := &runCtx{s: s, gp: fake.NewGoPodder(s.t)}
	absAdmin := ""
	if s.admin {
		absAdmin = adminToken
		if s.adminRejected {
			absAdmin = "another-admin-token"
		}
	}
	c.abs = fake.NewABS(s.t, userToken, absAdmin)
	for _, e := range s.episodes {
		c.abs.AddEpisode(toFakeEpisode(e, false))
	}
	for _, e := range s.hidden {
		c.abs.AddEpisode(toFakeEpisode(e, true))
	}
	for _, p := range s.progress {
		c.abs.SetProgress(fake.ABSProgress{ItemID: p.Ep.ItemID, EpisodeID: p.Ep.EpisodeID, CurrentTime: p.CurrentTime,
			Duration: p.Duration, IsFinished: p.Finished, LastUpdate: p.LastUpdate.UnixMilli()})
	}
	bySlug := map[string]*fake.ABSFeed{}
	var slugs []string
	for _, e := range s.feeds {
		f, ok := bySlug[e.FeedSlug]
		if !ok {
			f = &fake.ABSFeed{Slug: e.FeedSlug, ItemID: e.ItemID}
			bySlug[e.FeedSlug] = f
			slugs = append(slugs, e.FeedSlug)
		}
		f.Episodes = append(f.Episodes, fake.ABSFeedEpisode{ID: e.FeedEpisodeID, FullPath: e.Path})
	}
	for _, sl := range slugs {
		c.abs.AddFeed(*bySlug[sl])
	}
	for lib, items := range s.bookLibs {
		c.abs.AddBookLibrary(lib, items...)
	}
	for _, a := range s.records {
		c.gp.Seed(a.JSON())
	}
	for _, a := range s.duplicates {
		c.gp.SeedDuplicate(a.JSON())
	}
	for p, cn := range s.canned {
		c.gp.Respond(p, cn)
	}
	for _, r := range s.gpRules {
		c.gp.AddRule(r.rule, !r.fromAct)
	}
	for _, r := range s.absRules {
		c.abs.AddRule(r.rule, !r.fromAct)
	}
	if s.gpDown {
		c.gp.Close()
	}
	if s.absDown {
		c.abs.Close()
	}

	gpURL, absURL := c.gp.URL, c.abs.URL
	if s.trailingSlash {
		gpURL += "/"
		absURL += "/"
	}
	c.baseEnv = harness.Env{
		"GOPODDER_URL":       gpURL,
		"ABS_URL":            absURL,
		"ABS_TOKEN":          userToken,
		"ABS_INDEX_SCHEDULE": defaultSched,
	}
	if s.admin {
		c.baseEnv["ABS_ADMIN_TOKEN"] = adminToken
	}
	for k, v := range s.env {
		c.baseEnv[k] = v
	}
	if s.manualStart {
		return c
	}
	b, code, ok := harness.TryStart(s.t, cloneEnv(c.baseEnv), !s.defaultPort)
	if !ok {
		if code >= 0 {
			s.fatal("ブリッジが起動直後に終了しました（終了コード %d）", code)
		} else {
			s.fatal("10 秒以内に GET /healthz が応答しませんでした")
		}
		return c
	}
	c.bridge = b
	if !s.noIndexWait && !s.absDown {
		if !c.abs.WaitBatchGets(1, indexWaitTime) {
			s.fatal("起動後 10 秒以内に索引が作られませんでした（POST /api/items/batch/get が呼ばれない）")
		}
	}
	return c
}

func toFakeEpisode(e Episode, hidden bool) fake.ABSEpisode {
	return fake.ABSEpisode{LibraryID: e.LibraryID, ItemID: e.ItemID, EpisodeID: e.EpisodeID,
		EnclosureURL: e.EnclosureURL, GUID: e.GUID, Duration: e.Duration, Path: e.Path, Hidden: hidden}
}

func cloneEnv(e harness.Env) harness.Env {
	c := harness.Env{}
	for k, v := range e {
		c[k] = v
	}
	return c
}

func (c *runCtx) gpAct() []fake.Exchange  { return c.gp.Exchanges()[c.gpMark:] }
func (c *runCtx) absAct() []fake.Exchange { return c.abs.Exchanges()[c.absMark:] }

func (c *runCtx) last() *clientResult {
	if len(c.reqs) == 0 {
		c.s.fatal("クライアントのリクエストがありません（テストの書き方の誤り）")
		return &clientResult{}
	}
	return &c.reqs[len(c.reqs)-1]
}

func (s *S) fail(format string, args ...any) {
	s.t.Helper()
	s.t.Errorf("REPORT: [%s] %s", s.id, fmt.Sprintf(format, args...))
}

func (s *S) fatal(format string, args ...any) {
	s.t.Helper()
	s.t.Fatalf("REPORT: [%s] %s", s.id, fmt.Sprintf(format, args...))
}

// ---------- 故障を注入する対象 ----------

// Target は、故障を注入するリクエストの種類。
type Target struct {
	label string
	match func(r *http.Request) bool
}

func isEpisodes(p string) bool {
	return strings.HasPrefix(p, "/api/2/episodes/") && strings.HasSuffix(p, ".json") && strings.Count(p, "/") == 4
}

var (
	// AllRequests はすべてのリクエスト。
	AllRequests = Target{"すべての要求", func(*http.Request) bool { return true }}
	// History はブリッジが goPodder の記録を取得する GET（since=0）。
	History = Target{"記録の取得（since=0）", func(r *http.Request) bool {
		return r.Method == http.MethodGet && isEpisodes(r.URL.Path) && r.URL.Query().Get("since") == "0"
	}}
	// EpisodesPOST は goPodder への POST /api/2/episodes/{user}.json。
	EpisodesPOST = Target{"POST episodes", func(r *http.Request) bool { return r.Method == http.MethodPost && isEpisodes(r.URL.Path) }}
	// ProgressGET は ABS の GET /api/me/progress。
	ProgressGET = Target{"GET /api/me/progress", func(r *http.Request) bool { return r.Method == http.MethodGet && r.URL.Path == "/api/me/progress" }}
	// AnyPATCH は ABS の進捗の PATCH。
	AnyPATCH = Target{"PATCH progress", func(r *http.Request) bool { return r.Method == http.MethodPatch }}
	// BatchGet は ABS の POST /api/items/batch/get。
	BatchGet = Target{"POST batch/get", func(r *http.Request) bool { return r.URL.Path == "/api/items/batch/get" }}
	// Libraries は ABS の GET /api/libraries。
	Libraries = Target{"GET /api/libraries", func(r *http.Request) bool { return r.URL.Path == "/api/libraries" }}
	// Feeds は ABS の GET /api/feeds。
	Feeds = Target{"GET /api/feeds", func(r *http.Request) bool { return r.URL.Path == "/api/feeds" }}
)

// PATCHOf は ep の進捗の PATCH。
func PATCHOf(ep Episode) Target {
	p := "/api/me/progress/" + ep.ItemID + "/" + ep.EpisodeID
	return Target{"PATCH " + ep.Key, func(r *http.Request) bool { return r.Method == http.MethodPatch && r.URL.Path == p }}
}
