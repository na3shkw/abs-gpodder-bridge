package scenario

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/harness"
)

// ---------- Act：クライアントのリクエスト ----------

// ClientPOST はクライアント（alice）としてアクションを送る。
func (s *S) ClientPOST(actions ...Action) *S { return s.ClientPOSTAs(clientUser, actions...) }

// ClientPOSTAs は、パスのユーザー名を user にしてアクションを送る。
func (s *S) ClientPOSTAs(user string, actions ...Action) *S {
	var detail []string
	payload := []map[string]any{}
	for _, a := range actions {
		detail = append(detail, a.label())
		payload = append(payload, a.JSON())
	}
	p := "/api/2/episodes/" + user + ".json"
	s.d.when = append(s.d.when, message{from: Client, to: Bridge, label: "POST " + p, detail: detail})
	body, _ := json.Marshal(payload)
	s.whens = append(s.whens, func(c *runCtx) { s.send(c, http.MethodPost, p, "", nil, body) })
	return s
}

// ClientPOSTRaw は、本文をそのまま送る（JSON でない本文などを送るため）。
func (s *S) ClientPOSTRaw(body string) *S {
	p := "/api/2/episodes/" + clientUser + ".json"
	s.d.when = append(s.d.when, message{from: Client, to: Bridge, label: "POST " + p, detail: []string{"本文: " + body}})
	s.whens = append(s.whens, func(c *runCtx) { s.send(c, http.MethodPost, p, "", nil, []byte(body)) })
	return s
}

// ClientGET はクライアント（alice）として同期する（since=1）。
func (s *S) ClientGET() *S { return s.ClientGETSince(1) }

// ClientGETSince は、since を指定して同期する。
func (s *S) ClientGETSince(since int) *S {
	p := "/api/2/episodes/" + clientUser + ".json"
	q := fmt.Sprintf("since=%d", since)
	s.d.when = append(s.d.when, message{from: Client, to: Bridge, label: "GET " + p + "?" + q})
	s.whens = append(s.whens, func(c *runCtx) { s.send(c, http.MethodGet, p, q, nil, nil) })
	return s
}

// ClientHealthz は GET /healthz を送る。
func (s *S) ClientHealthz() *S {
	s.d.when = append(s.d.when, message{from: Client, to: Bridge, label: "GET /healthz"})
	s.whens = append(s.whens, func(c *runCtx) { s.send(c, http.MethodGet, "/healthz", "", nil, nil) })
	return s
}

// ClientRequest は任意のリクエストを送る。header は追加で付けるヘッダー。
func (s *S) ClientRequest(method, p, query string, header map[string]string, body string) *S {
	label := method + " " + p
	if query != "" {
		label += "?" + query
	}
	var detail []string
	keys := make([]string, 0, len(header))
	for k := range header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		detail = append(detail, k+": "+header[k])
	}
	if body != "" {
		detail = append(detail, "本文: "+body)
	}
	s.d.when = append(s.d.when, message{from: Client, to: Bridge, label: label, detail: detail})
	var b []byte
	if body != "" {
		b = []byte(body)
	}
	s.whens = append(s.whens, func(c *runCtx) { s.send(c, method, p, query, header, b) })
	return s
}

// send はブリッジへリクエストを送り、結果を記録する。
// どのリクエストにも、クライアントの Authorization と Cookie を付ける。
func (s *S) send(c *runCtx, method, p, query string, header map[string]string, body []byte) {
	if c.bridge == nil {
		s.fatal("ブリッジが起動していません（テストの書き方の誤り）")
		return
	}
	u := c.bridge.URL + p
	if query != "" {
		u += "?" + query
	}
	req, _ := http.NewRequest(method, u, bytes.NewReader(body))
	if body == nil {
		req, _ = http.NewRequest(method, u, nil)
	}
	req.Header.Set("Authorization", clientAuth)
	req.Header.Set("Cookie", clientCookie)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range header {
		req.Header.Set(k, v)
	}
	r := clientResult{method: method, path: p, query: query, header: req.Header.Clone(), body: body, start: time.Now()}
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	r.end = time.Now()
	if err != nil {
		r.err = err
	} else {
		r.status = resp.StatusCode
		r.respHeader = resp.Header.Clone()
		r.respBody, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
	}
	c.reqs = append(c.reqs, r)
}

// ---------- Act：ブリッジの起動（設定のテスト用） ----------

// Variant は、起動の設定の違い。
type Variant struct {
	Label string
	Unset []string
	Set   map[string]string
}

// StartVariants は、Arrange でブリッジを起動せず、設定を変えて 1 つずつ起動する。
// 結果は ExpectEveryExitCode / ExpectEveryStarted で確かめる。
func (s *S) StartVariants(vs ...Variant) *S {
	s.manualStart = true
	lines := []string{fmt.Sprintf("設定を変えて起動する（%d 通り）", len(vs))}
	for _, v := range vs {
		lines = append(lines, "・"+v.Label)
	}
	s.d.when = append(s.d.when, stepBox{lane: Bridge, lines: lines})
	s.whens = append(s.whens, func(c *runCtx) {
		for _, v := range vs {
			env := cloneEnv(c.baseEnv)
			for _, k := range v.Unset {
				delete(env, k)
			}
			for k, val := range v.Set {
				env[k] = val
			}
			_, code, ok := harness.TryStart(s.t, env, true)
			c.starts = append(c.starts, startResult{label: v.Label, started: ok, code: code})
		}
	})
	return s
}

// ---------- Act：時間の経過と ABS の変化 ----------

// Wait は d だけ待つ。
func (s *S) Wait(d time.Duration) *S {
	s.d.when = append(s.d.when, step{lane: Bridge, text: fmt.Sprintf("%s 待つ", d)})
	s.whens = append(s.whens, func(*runCtx) { time.Sleep(d) })
	return s
}

// WaitIndexRebuild は、索引がもう一度作られる（batch/get が呼ばれる）まで待つ。
func (s *S) WaitIndexRebuild() *S {
	s.d.when = append(s.d.when, step{lane: Bridge, text: "索引の作り直しを待つ"})
	s.whens = append(s.whens, func(c *runCtx) {
		// 待ち始める前に始まっていた作り直しを数えないよう、2 回分待つ
		n := c.abs.BatchGets()
		if !c.abs.WaitBatchGets(n+2, 10*time.Second) {
			s.fatal("10 秒以内に索引が作り直されませんでした（POST /api/items/batch/get が呼ばれない）")
		}
	})
	return s
}

// WaitIndexBuilt は、最初の索引ができる（batch/get が返る）まで待つ。
func (s *S) WaitIndexBuilt() *S {
	s.d.when = append(s.d.when, step{lane: Bridge, text: "索引ができるのを待つ"})
	s.whens = append(s.whens, func(c *runCtx) {
		if !c.abs.WaitBatchGets(1, 10*time.Second) {
			s.fatal("10 秒以内に索引が作られませんでした（POST /api/items/batch/get が呼ばれない）")
		}
		time.Sleep(300 * time.Millisecond)
	})
	return s
}

// WaitIndexAttempt は、索引を作ろうとする（GET /api/libraries が呼ばれる）まで待つ。
func (s *S) WaitIndexAttempt() *S {
	s.d.when = append(s.d.when, step{lane: Bridge, text: "索引の作り直しの試みを待つ"})
	s.whens = append(s.whens, func(c *runCtx) {
		if !c.abs.WaitRequest("/api/libraries", len(c.abs.Exchanges()), 10*time.Second) {
			s.fatal("10 秒以内に索引を作ろうとしませんでした（GET /api/libraries が呼ばれない）")
		}
		time.Sleep(300 * time.Millisecond)
	})
	return s
}

// ABSAddEpisodeNow は、ABS にエピソードを追加する。
func (s *S) ABSAddEpisodeNow(e Episode) *S {
	s.d.when = append(s.d.when, step{lane: ABS, text: e.Key + " を追加"})
	s.whens = append(s.whens, func(c *runCtx) { c.abs.AddEpisode(toFakeEpisode(e, false)) })
	return s
}

// ABSRemoveEpisodeNow は、ABS からエピソードを削除する。
func (s *S) ABSRemoveEpisodeNow(e Episode) *S {
	s.d.when = append(s.d.when, step{lane: ABS, text: e.Key + " を削除"})
	s.whens = append(s.whens, func(c *runCtx) { c.abs.RemoveEpisode(e.EpisodeID) })
	return s
}
