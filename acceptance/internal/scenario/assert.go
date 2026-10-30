package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/fake"
)

// ---------- Assert：クライアントへの応答 ----------

// ExpectResponseFromGoPodder は、最後のリクエストへの応答が goPodder の応答そのもの（ステータス・本文）であることを確かめる。
func (s *S) ExpectResponseFromGoPodder() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: "goPodder の応答そのまま", expect: true})
	s.thens = append(s.thens, func(c *runCtx) { s.checkRelayedResponse(c, c.last(), false) })
	return s
}

// ExpectEveryResponseFromGoPodder は、すべてのリクエストへの応答が goPodder の応答そのもの（ステータス・Set-Cookie・本文）であることを確かめる。
func (s *S) ExpectEveryResponseFromGoPodder() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: "どれも goPodder の応答そのまま", detail: []string{"ステータス・Set-Cookie・本文"}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for i := range c.reqs {
			s.checkRelayedResponse(c, &c.reqs[i], true)
		}
	})
	return s
}

func (s *S) checkRelayedResponse(c *runCtx, r *clientResult, cookie bool) {
	if r.err != nil {
		s.fail("%s %s に応答がありません", r.method, r.path)
		return
	}
	var relayed *fake.Exchange
	for _, ex := range c.gp.Exchanges() {
		ex := ex
		if ex.Method == r.method && ex.Path == r.path && ex.Query == r.query && !ex.Done.IsZero() {
			relayed = &ex
		}
	}
	if relayed == nil {
		s.fail("%s %s が goPodder へ中継されていません", r.method, r.path)
		return
	}
	if r.status != relayed.Status || !bytes.Equal(r.respBody, relayed.RespBody) {
		s.fail("%s %s への応答が goPodder の応答と一致しません", r.method, r.path)
	}
	if cookie && relayed.RespHdr.Get("Set-Cookie") != r.respHeader.Get("Set-Cookie") {
		s.fail("%s %s への応答の Set-Cookie が goPodder の応答と一致しません", r.method, r.path)
	}
}

// ExpectStatus は、最後のリクエストへの応答のステータスを確かめる。
func (s *S) ExpectStatus(code int) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: fmt.Sprintf("%d %s", code, http.StatusText(code)), expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		r := c.last()
		if r.err != nil {
			s.fail("応答がありません")
		} else if r.status != code {
			s.fail("ステータスが %d ではありません（%d）", code, r.status)
		}
	})
	return s
}

// ExpectJSONResponse は、最後のリクエストへの応答が 200・JSON・この本文であることを確かめる。
func (s *S) ExpectJSONResponse(body Body) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: "200・application/json", detail: []string{body.String()}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		r := c.last()
		if r.err != nil || r.status != http.StatusOK {
			s.fail("200 が返りません")
			return
		}
		if !strings.HasPrefix(r.respHeader.Get("Content-Type"), "application/json") {
			s.fail("Content-Type が application/json ではありません")
		}
		var m map[string]any
		if json.Unmarshal(r.respBody, &m) != nil || !sameBody(m, body) {
			s.fail("本文が期待と違います")
		}
	})
	return s
}

// ExpectResponseWithin は、最後のリクエストへの応答が d 以内に返ることを確かめる。
func (s *S) ExpectResponseWithin(d time.Duration) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: fmt.Sprintf("%s 以内に応答", d), expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		r := c.last()
		if r.err != nil {
			s.fail("応答がありません")
		} else if el := r.end.Sub(r.start); el > d {
			s.fail("応答までに %s かかりました（%s 以内のはず）", el.Round(100*time.Millisecond), d)
		}
	})
	return s
}

// ---------- Assert：goPodder が受け取ったリクエスト ----------

var hopByHop = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"}

// ExpectRelayedUnchanged は、すべてのリクエストが、メソッド・パス・クエリ・本文・ヘッダーを変えずに goPodder へ届くことを確かめる。
func (s *S) ExpectRelayedUnchanged() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "同じ内容で届く", detail: []string{"メソッド・パス・クエリ・本文・ヘッダー"}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		used := map[int]bool{}
		exs := c.gpAct()
		for _, r := range c.reqs {
			found := -1
			for i, ex := range exs {
				if !used[i] && ex.Method == r.method && ex.Path == r.path && ex.Query == r.query {
					found = i
					break
				}
			}
			if found < 0 {
				s.fail("%s %s?%s が同じメソッド・パス・クエリで goPodder に届いていません", r.method, r.path, r.query)
				continue
			}
			used[found] = true
			ex := exs[found]
			if !bytes.Equal(ex.Body, r.body) {
				s.fail("%s %s の本文が変わっています", r.method, r.path)
			}
			skip := map[string]bool{}
			for _, h := range hopByHop {
				skip[http.CanonicalHeaderKey(h)] = true
			}
			for _, n := range strings.Split(r.header.Get("Connection"), ",") {
				if n = strings.TrimSpace(n); n != "" {
					skip[http.CanonicalHeaderKey(n)] = true
				}
			}
			for k, vs := range r.header {
				if skip[k] {
					continue
				}
				if !reflect.DeepEqual(ex.Header.Values(k), vs) {
					s.fail("%s %s のヘッダー %s が変わっています", r.method, r.path, k)
				}
			}
		}
	})
	return s
}

// ExpectHopByHopRemoved は、hop-by-hop ヘッダーが goPodder に届かず、Host が転送先になることを確かめる。
func (s *S) ExpectHopByHopRemoved() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "hop-by-hop は届かない", detail: []string{"Connection に挙げたもの・Keep-Alive", "Host は転送先"}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		host := strings.TrimPrefix(c.gp.URL, "http://")
		for _, r := range c.reqs {
			for _, ex := range c.gpAct() {
				if ex.Path != r.path {
					continue
				}
				for _, n := range append([]string{"Keep-Alive"}, strings.Split(r.header.Get("Connection"), ",")...) {
					n = strings.TrimSpace(n)
					if n != "" && !strings.EqualFold(n, "close") && !strings.EqualFold(n, "keep-alive") && ex.Header.Get(n) != "" {
						s.fail("hop-by-hop ヘッダー %s が goPodder に届いています", n)
					}
				}
				if ex.Header.Get("Keep-Alive") != "" {
					s.fail("hop-by-hop ヘッダー Keep-Alive が goPodder に届いています")
				}
				if ex.Host != host {
					s.fail("goPodder に届いた Host が転送先（%s）ではありません", host)
				}
			}
		}
	})
	return s
}

// ExpectGoPodderNotReceived は、goPodder がこのパスへのリクエストを受け取らないことを確かめる。
func (s *S) ExpectGoPodderNotReceived(p string) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: p + " は届かない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.gpAct() {
			if ex.Path == p {
				s.fail("%s が goPodder へ中継されました", p)
				return
			}
		}
	})
	return s
}

// ExpectHistoryFetched は、ブリッジが goPodder の記録を since=0 でちょうど 1 回取得することを確かめる。
func (s *S) ExpectHistoryFetched() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "GET episodes?since=0（1 回）", expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		n := 0
		for _, ex := range c.gpAct() {
			if ex.Method == http.MethodGet && isEpisodes(ex.Path) && ex.Query == "since=0" {
				n++
			}
		}
		if n != 1 {
			s.fail("記録の取得（since=0）が %d 回でした（期待は 1 回）", n)
		}
	})
	return s
}

// ExpectBridgeRequestsCarryClientAuth は、goPodder へのリクエストすべてに、クライアントの Authorization と Cookie が付いていることを確かめる。
func (s *S) ExpectBridgeRequestsCarryClientAuth() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "どの要求にもクライアントの認証", detail: []string{"Authorization・Cookie"}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		exs := c.gpAct()
		if len(exs) < 2 {
			s.fail("ブリッジが goPodder へ独自のリクエストを送っていません")
		}
		for _, ex := range exs {
			if ex.Header.Get("Authorization") != clientAuth || !strings.Contains(ex.Header.Get("Cookie"), clientCookie) {
				s.fail("%s %s にクライアントの Authorization・Cookie が付いていません", ex.Method, ex.Path)
			}
		}
	})
	return s
}

// ExpectGoPodderWrite は、ブリッジが goPodder へ書き戻す POST がちょうど 1 回で、中身がこのアクションだけ（順不同）であることを確かめる。
func (s *S) ExpectGoPodderWrite(actions ...Action) *S {
	var detail []string
	for _, a := range actions {
		detail = append(detail, a.label())
	}
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "POST で書き戻す（1 回）", detail: detail, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		var posts []fake.Exchange
		for _, ex := range c.gpAct() {
			if ex.Method == http.MethodPost && isEpisodes(ex.Path) && !c.isClientRequest(ex) {
				posts = append(posts, ex)
			}
		}
		if len(posts) != 1 {
			s.fail("goPodder への書き戻しの POST が %d 回送られました（期待は 1 回）", len(posts))
			return
		}
		var got []map[string]any
		if json.Unmarshal(posts[0].Body, &got) != nil || len(got) != len(actions) {
			s.fail("書き戻したアクションの数が期待と違います")
			return
		}
		used := map[int]bool{}
		for _, a := range actions {
			ok := false
			for i, g := range got {
				if !used[i] && sameAction(g, a.JSON()) {
					used[i], ok = true, true
					break
				}
			}
			if !ok {
				s.fail("書き戻したアクションに、期待する %s の内容のものがありません", a.ep.Key)
			}
		}
	})
	return s
}

// isClientRequest は、goPodder が受け取ったリクエストが、クライアントのリクエストの中継かどうかを返す。
func (c *runCtx) isClientRequest(ex fake.Exchange) bool {
	for _, r := range c.reqs {
		if r.method == ex.Method && r.path == ex.Path && r.query == ex.Query && bytes.Equal(r.body, ex.Body) {
			return true
		}
	}
	return false
}

// ExpectNoGoPodderWrite は、ブリッジが goPodder へ書き戻す POST を送らないことを確かめる。
func (s *S) ExpectNoGoPodderWrite() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: GoPodder, label: "書き戻しは送られない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.gpAct() {
			if ex.Method == http.MethodPost && !c.isClientRequest(ex) {
				s.fail("送られないはずの書き戻しの POST が送られました")
				return
			}
		}
	})
	return s
}

// ExpectClientReceives は、最後のリクエストへの応答にこのアクションが含まれることを確かめる。
func (s *S) ExpectClientReceives(a Action) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: Client, label: "応答に含まれる", detail: []string{a.label()}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		var resp struct {
			Actions []map[string]any `json:"actions"`
		}
		if json.Unmarshal(c.last().respBody, &resp) != nil {
			s.fail("クライアントへの応答を読めません")
			return
		}
		for _, got := range resp.Actions {
			if sameAction(got, a.JSON()) {
				return
			}
		}
		s.fail("クライアントへの応答に %s のアクションが含まれていません", a.ep.Key)
	})
	return s
}

// ---------- Assert：ABS が受け取ったリクエスト ----------

func patchesOf(c *runCtx, ep Episode) []fake.Exchange {
	want := "/api/me/progress/" + ep.ItemID + "/" + ep.EpisodeID
	var out []fake.Exchange
	for _, ex := range c.absAct() {
		if ex.Method == http.MethodPatch && ex.Path == want {
			out = append(out, ex)
		}
	}
	return out
}

// ExpectPATCH は、ep の進捗の PATCH がちょうど 1 回、この本文（ほかの項目なし）で送られることを確かめる。
func (s *S) ExpectPATCH(ep Episode, body Body) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "PATCH " + ep.Key + "（1 回）", detail: []string{body.String()}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		got := patchesOf(c, ep)
		if len(got) != 1 {
			s.fail("%s の進捗の PATCH が %d 回送られました（期待は 1 回）", ep.Key, len(got))
			return
		}
		var m map[string]any
		if json.Unmarshal(got[0].Body, &m) != nil || !sameBody(m, body) {
			s.fail("%s の進捗の PATCH の本文が期待と違います", ep.Key)
		}
	})
	return s
}

// ExpectLastPATCH は、ep の進捗の PATCH のうち、最後のものがこの本文であることを確かめる。
func (s *S) ExpectLastPATCH(ep Episode, body Body) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "最後の PATCH " + ep.Key, detail: []string{body.String()}, expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		got := patchesOf(c, ep)
		if len(got) == 0 {
			s.fail("%s の進捗の PATCH が送られていません", ep.Key)
			return
		}
		var m map[string]any
		if json.Unmarshal(got[len(got)-1].Body, &m) != nil || !sameBody(m, body) {
			s.fail("%s の最後の PATCH の本文が期待と違います", ep.Key)
		}
	})
	return s
}

// ExpectPATCHAttempts は、ep の進捗の PATCH がちょうど n 回送られることを確かめる（本文は問わない）。
func (s *S) ExpectPATCHAttempts(ep Episode, n int) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: fmt.Sprintf("PATCH %s は %d 回だけ（再送しない）", ep.Key, n), expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		if got := len(patchesOf(c, ep)); got != n {
			s.fail("%s の進捗の PATCH が %d 回送られました（期待は %d 回）", ep.Key, got, n)
		}
	})
	return s
}

// ExpectPATCHDoneBeforeResponse は、ep の進捗の PATCH が、クライアントへ応答を返す前に終わっていることを確かめる。
func (s *S) ExpectPATCHDoneBeforeResponse(ep Episode) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "PATCH " + ep.Key + " が応答より先に終わる", expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		got := patchesOf(c, ep)
		if len(got) == 0 {
			s.fail("%s の進捗の PATCH が送られていません", ep.Key)
			return
		}
		if got[0].Done.IsZero() || got[0].Done.After(c.last().end) {
			s.fail("%s の進捗の PATCH が、クライアントへの応答のあとに終わりました", ep.Key)
		}
	})
	return s
}

// ExpectABSAttempted は、Act のあいだに、対象のリクエストが ABS へ送られる（結果は問わない）ことを確かめる。
func (s *S) ExpectABSAttempted(t Target) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: t.label + " を送る", expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.absAct() {
			req, _ := http.NewRequest(ex.Method, "http://x"+ex.Path+"?"+ex.Query, nil)
			if t.match(req) {
				return
			}
		}
		s.fail("%s が ABS へ送られていません", t.label)
	})
	return s
}

// ExpectPATCHCount は、進捗の PATCH が全部で n 回送られることを確かめる。
func (s *S) ExpectPATCHCount(n int) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: fmt.Sprintf("PATCH は全部で %d 回", n), expect: true})
	s.thens = append(s.thens, func(c *runCtx) {
		got := 0
		for _, ex := range c.absAct() {
			if ex.Method == http.MethodPatch {
				got++
			}
		}
		if got != n {
			s.fail("進捗の PATCH が全部で %d 回送られました（期待は %d 回）", got, n)
		}
	})
	return s
}

// ExpectNoPATCH は、進捗の PATCH が 1 回も送られないことを確かめる。
func (s *S) ExpectNoPATCH() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "PATCH は送られない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.absAct() {
			if ex.Method == http.MethodPatch {
				s.fail("送られないはずの進捗の PATCH が送られました")
				return
			}
		}
	})
	return s
}

// ExpectNoPATCHFor は、これらのエピソードの進捗の PATCH が送られないことを確かめる。
func (s *S) ExpectNoPATCHFor(eps ...Episode) *S {
	var keys []string
	for _, e := range eps {
		keys = append(keys, e.Key)
	}
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "PATCH " + strings.Join(keys, "・") + " は送られない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, e := range eps {
			if len(patchesOf(c, e)) > 0 {
				s.fail("送られないはずの %s の進捗の PATCH が送られました", e.Key)
			}
		}
	})
	return s
}

// ExpectNoABSRequests は、Act のあいだ ABS へ 1 つもリクエストが送られないことを確かめる。
func (s *S) ExpectNoABSRequests() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "ABS へは何も送られない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		if exs := c.absAct(); len(exs) > 0 {
			s.fail("送られないはずのリクエストが ABS へ送られました（%s %s）", exs[0].Method, exs[0].Path)
		}
	})
	return s
}

// ExpectTokenUsage は、起動からの ABS へのリクエストすべてで、/api/feeds には管理者のトークン、
// それ以外には ABS_TOKEN が使われることを確かめる。
func (s *S) ExpectTokenUsage() *S {
	s.d.then = append(s.d.then, check{lane: ABS, text: "/api/feeds は管理者、ほかは ABS_TOKEN"})
	s.thens = append(s.thens, func(c *runCtx) {
		feeds := false
		for _, ex := range c.abs.Exchanges() {
			tok := strings.TrimPrefix(ex.Header.Get("Authorization"), "Bearer ")
			if ex.Path == "/api/feeds" {
				feeds = true
				if tok != adminToken {
					s.fail("/api/feeds に管理者のトークンが使われていません")
				}
			} else if tok != userToken {
				s.fail("%s %s に ABS_TOKEN が使われていません", ex.Method, ex.Path)
			}
		}
		if s.admin && !feeds {
			s.fail("/api/feeds が呼ばれていません")
		}
	})
	return s
}

// ExpectIndexBuiltAtStartup は、起動後に索引が作られ、batch/get で Arrange のエピソードのアイテムが取得されたことを確かめる。
func (s *S) ExpectIndexBuiltAtStartup() *S {
	s.d.then = append(s.d.then, check{lane: ABS, text: "起動時に batch/get でアイテムを取得"})
	s.thens = append(s.thens, func(c *runCtx) {
		ids := map[string]bool{}
		for _, ex := range c.abs.Exchanges() {
			if ex.Path == "/api/items/batch/get" {
				var req struct {
					IDs []string `json:"libraryItemIds"`
				}
				_ = json.Unmarshal(ex.Body, &req)
				for _, id := range req.IDs {
					ids[id] = true
				}
			}
		}
		for _, e := range s.episodes {
			if !ids[e.ItemID] {
				s.fail("索引の作成で %s のアイテムが取得されていません", e.Key)
			}
		}
	})
	return s
}

// ExpectNoIndexRebuild は、Act のあいだ索引が作り直されない（batch/get が呼ばれない）ことを確かめる。
func (s *S) ExpectNoIndexRebuild() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "batch/get は呼ばれない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.absAct() {
			if ex.Path == "/api/items/batch/get" {
				s.fail("索引が作り直されました（batch/get が呼ばれた）")
				return
			}
		}
	})
	return s
}

// ExpectNoProgressFetch は、Act のあいだ ABS の進捗を取得しないことを確かめる。
func (s *S) ExpectNoProgressFetch() *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: "GET /api/me/progress は呼ばれない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.absAct() {
			if ex.Path == "/api/me/progress" {
				s.fail("何もしていない間に、ABS の進捗を取得しました")
				return
			}
		}
	})
	return s
}

// ExpectABSNeverRequested は、起動からずっと、ABS がこのパスへのリクエストを受け取らないことを確かめる。
func (s *S) ExpectABSNeverRequested(p string) *S {
	s.d.then = append(s.d.then, message{from: Bridge, to: ABS, label: p + " は呼ばれない", negated: true})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range c.abs.Exchanges() {
			if ex.Path == p {
				s.fail("呼ばれないはずの %s が呼ばれました", p)
				return
			}
		}
	})
	return s
}

// ExpectNoLimitOnItems は、番組一覧（/api/libraries/{id}/items）の取得に limit を付けないことを確かめる。
func (s *S) ExpectNoLimitOnItems() *S {
	s.d.then = append(s.d.then, check{lane: ABS, text: "番組一覧の取得に limit がない"})
	s.thens = append(s.thens, func(c *runCtx) {
		n := 0
		for _, ex := range c.abs.Exchanges() {
			if strings.HasPrefix(ex.Path, "/api/libraries/") && strings.HasSuffix(ex.Path, "/items") {
				n++
				q, _ := url.ParseQuery(ex.Query)
				if _, ok := q["limit"]; ok {
					s.fail("番組一覧の取得に limit が付いています")
				}
			}
		}
		if n == 0 {
			s.fail("番組一覧が取得されていません")
		}
	})
	return s
}

// ExpectCleanPaths は、goPodder と ABS に届いたパスに「//」が含まれないことを確かめる。
func (s *S) ExpectCleanPaths() *S {
	s.d.then = append(s.d.then, check{lane: Bridge, text: "goPodder・ABS へのパスに // がない"})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, ex := range append(c.gp.Exchanges(), c.abs.Exchanges()...) {
			if strings.Contains(ex.Path, "//") {
				s.fail("パスに // が含まれています（%s）", ex.Path)
			}
		}
	})
	return s
}

// ---------- Assert：起動 ----------

// ExpectEveryExitCode は、StartVariants のどの設定でも、ブリッジが終了コード code で終わることを確かめる。
func (s *S) ExpectEveryExitCode(code int) *S {
	s.d.then = append(s.d.then, check{lane: Bridge, text: fmt.Sprintf("どれも終了コード %d で終わる", code)})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, r := range c.starts {
			switch {
			case r.started:
				s.fail("%s: 起動してしまいました（終了コード %d のはず）", r.label, code)
			case r.code != code:
				s.fail("%s: 終了コードが %d ではありません（%d）", r.label, code, r.code)
			}
		}
	})
	return s
}

// ExpectEveryStarted は、StartVariants のどの設定でも、ブリッジが起動して /healthz に応答することを確かめる。
func (s *S) ExpectEveryStarted() *S {
	s.d.then = append(s.d.then, check{lane: Bridge, text: "どれも起動する（/healthz に応答）"})
	s.thens = append(s.thens, func(c *runCtx) {
		for _, r := range c.starts {
			if !r.started {
				s.fail("%s: 起動しませんでした（終了コード %d）", r.label, r.code)
			}
		}
	})
	return s
}

// ---------- 比較 ----------

func sameBody(got map[string]any, want Body) bool {
	if len(got) != len(want) {
		return false
	}
	for _, kv := range want {
		g, ok := got[kv.K]
		if !ok || !sameValue(g, kv.V) {
			return false
		}
	}
	return true
}

func sameAction(got, want map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for k, w := range want {
		g, ok := got[k]
		if !ok || !sameValue(g, w) {
			return false
		}
	}
	return true
}

func sameValue(got, want any) bool {
	gf, gok := toFloat(got)
	wf, wok := toFloat(want)
	if gok && wok {
		return math.Abs(gf-wf) < 1e-9
	}
	return reflect.DeepEqual(got, want)
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}
