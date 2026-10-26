package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
)

// gpState は goPodder 側の状態（5.2 節の手順 2、5.1 節の候補）。
type gpState struct {
	act  action
	play bool  // 再生の状態なら true、再生以外の状態なら false
	when int64 // 比較に使う時刻（7.1 節）。再生以外の状態では使わない
}

type direction int

const (
	doNothing direction = iota
	toABS
	toGopodder
)

// decide は 7.2 節の表で処理を決める。p が nil なら ABS 側は存在しない。
func decide(a *gpState, p *absProgress) direction {
	switch {
	case !a.play && p == nil:
		return doNothing
	case !a.play:
		return toGopodder
	case p == nil:
		return toABS
	case sameState(&a.act, p):
		return doNothing
	case a.when > p.lastUpdateSec():
		return toABS
	case p.lastUpdateSec() > a.when:
		return toGopodder
	}
	return doNothing
}

func sameState(a *action, p *absProgress) bool {
	if a.finished() {
		return p.IsFinished
	}
	return !p.IsFinished && math.Abs(a.Position-p.CurrentTime) <= 1
}

// applyPostedActions は 5.1 節の手順 3〜6 を行う。
func (s *Server) applyPostedActions(ctx context.Context, body []byte, received int64) {
	var raws []json.RawMessage
	if err := json.Unmarshal(body, &raws); err != nil {
		s.log.Debug("request body is not a JSON array; skipping ABS update", "error", err)
		return
	}
	ix := s.index.get()
	var order []episodeRef
	cands := map[episodeRef]*gpState{}
	for _, raw := range raws {
		a, err := parseAction(raw)
		if err != nil {
			s.log.Debug("ignoring invalid action", "action", string(raw))
			continue
		}
		if !a.isPlay() || !a.hasPlayablePosition() {
			continue
		}
		ref, ok := ix.match(a.Episode, a.GUID)
		if !ok {
			s.log.Debug("action did not match any ABS episode", "podcast", a.Podcast, "episode", a.Episode, "guid", a.GUID)
			continue
		}
		st := &gpState{act: a, play: true, when: received}
		if a.HasTimestamp {
			st.when = a.Timestamp
		}
		if prev, ok := cands[ref]; !ok {
			order = append(order, ref)
		} else if prev.when > st.when {
			continue
		}
		cands[ref] = st
	}
	if len(order) == 0 {
		return
	}
	progress, err := s.abs.progress(ctx)
	if err != nil {
		s.log.Warn("getting ABS progress failed; skipping ABS update", "error", err)
		return
	}
	for _, ref := range order {
		st := cands[ref]
		if decide(st, progress[ref]) == toABS {
			s.updateABS(ctx, ref, &st.act)
		}
	}
}

// reconcile は 5.2 節の手順 1〜4 を行う。失敗しても戻るだけで、手順 5 は呼び出し側が行う。
func (s *Server) reconcile(r *http.Request) {
	ctx := r.Context()
	actions, err := s.fetchGopodderActions(r)
	if err != nil {
		s.log.Warn("reconcile: getting goPodder actions failed", "error", err)
		return
	}
	states := episodeStates(actions)
	if len(states) == 0 {
		return
	}
	progress, err := s.abs.progress(ctx)
	if err != nil {
		s.log.Warn("reconcile: getting ABS progress failed", "error", err)
		return
	}
	ix := s.index.get()
	var writes []outAction
	for _, st := range states {
		ref, ok := ix.match(st.act.Episode, st.act.GUID)
		if !ok {
			s.log.Debug("reconcile: episode did not match any ABS episode", "podcast", st.act.Podcast, "episode", st.act.Episode, "guid", st.act.GUID)
			continue
		}
		p := progress[ref]
		switch decide(st, p) {
		case toABS:
			s.updateABS(ctx, ref, &st.act)
		case toGopodder:
			writes = append(writes, gopodderAction(&st.act, p, ix.duration[ref]))
		}
	}
	if len(writes) == 0 {
		return
	}
	if err := s.postGopodderActions(r, writes); err != nil {
		s.log.Warn("reconcile: writing actions to goPodder failed", "error", err)
		return
	}
	s.log.Info("reconcile: wrote ABS progress to goPodder", "actions", len(writes))
}

// episodeStates は 5.2 節の手順 2 を行い、状態を時刻の古い順に並べて返す。
func episodeStates(actions []action) []*gpState {
	type key struct{ podcast, episode string }
	var order []key
	seen := map[key]bool{}
	plays := map[key]*gpState{}
	others := map[key]*gpState{}
	for _, a := range actions {
		k := key{a.Podcast, a.Episode}
		if !seen[k] {
			seen[k] = true
			order = append(order, k)
		}
		switch {
		case a.isPlay():
			if !a.hasPlayablePosition() || !a.HasTimestamp {
				continue
			}
			if prev := plays[k]; prev == nil || prev.when <= a.Timestamp {
				plays[k] = &gpState{act: a, play: true, when: a.Timestamp}
			}
		default:
			others[k] = &gpState{act: a}
		}
	}
	var states []*gpState
	for _, k := range order {
		if st := plays[k]; st != nil {
			states = append(states, st)
		} else if st := others[k]; st != nil {
			states = append(states, st)
		}
	}
	// 再生以外の状態は、どの時刻よりも古いものとみなす。
	sort.SliceStable(states, func(i, j int) bool {
		a, b := states[i], states[j]
		if a.play != b.play {
			return !a.play
		}
		return a.play && a.when < b.when
	})
	return states
}

// gopodderAction は 5.4 節のアクションを作る。
func gopodderAction(st *action, p *absProgress, indexDuration float64) outAction {
	out := outAction{
		Podcast:   st.Podcast,
		Episode:   st.Episode,
		GUID:      st.GUID,
		Action:    "play",
		Timestamp: formatTimestamp(p.lastUpdateSec()),
		Position:  int64(p.CurrentTime),
	}
	var total float64
	switch {
	case p.Duration > 0:
		total = p.Duration
	case indexDuration > 0:
		total = indexDuration
	}
	if total > 0 {
		t, started := int64(total), int64(0)
		out.Total, out.Started = &t, &started
		if p.IsFinished {
			out.Position = t
		}
	}
	return out
}

func (s *Server) gopodderRequest(r *http.Request, method, query string, body io.Reader) (*http.Response, error) {
	target := s.cfg.GopodderURL + r.URL.EscapedPath() + query
	req, err := http.NewRequestWithContext(r.Context(), method, target, body)
	if err != nil {
		return nil, err
	}
	for _, h := range []string{"Authorization", "Cookie"} {
		if vs := r.Header.Values(h); len(vs) > 0 {
			req.Header[h] = vs
		}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.gopodder.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, fmt.Errorf("%s %s: status %d", method, r.URL.Path, resp.StatusCode)
	}
	return resp, nil
}

// fetchGopodderActions は 5.2 節の手順 1 を行う。11 章の形式に合わないアクションは除く。
func (s *Server) fetchGopodderActions(r *http.Request) ([]action, error) {
	resp, err := s.gopodderRequest(r, http.MethodGet, "?since=0", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Actions []json.RawMessage `json:"actions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding goPodder actions: %w", err)
	}
	var actions []action
	for _, raw := range body.Actions {
		a, err := parseAction(raw)
		if err != nil {
			s.log.Debug("reconcile: ignoring invalid action", "action", string(raw))
			continue
		}
		actions = append(actions, a)
	}
	return actions, nil
}

func (s *Server) postGopodderActions(r *http.Request, actions []outAction) error {
	b, err := json.Marshal(actions)
	if err != nil {
		return err
	}
	resp, err := s.gopodderRequest(r, http.MethodPost, "", bytes.NewReader(b))
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}
