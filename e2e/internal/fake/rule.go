package fake

import (
	"net/http"
	"sync"
	"time"
)

// Rule は、条件に合うリクエストへの故障の注入。
type Rule struct {
	Match  func(r *http.Request) bool
	Status int           // 0 以外ならこのステータスを返す
	Hang   bool          // 応答しない（相手が切断するまで待つ）
	Delay  time.Duration // 待ってから通常どおり処理する
	active bool
}

type rules struct {
	mu    sync.Mutex
	items []*Rule
}

// Add は規則を加える。active=false の規則は Activate を呼ぶまで効かない。
func (rs *rules) add(r Rule, active bool) *Rule {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r.active = active
	rs.items = append(rs.items, &r)
	return &r
}

func (rs *rules) activateAll() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, r := range rs.items {
		r.active = true
	}
}

// apply は規則を当てはめる。応答を書いた（処理を終えた）なら true を返す。
func (rs *rules) apply(w http.ResponseWriter, r *http.Request) bool {
	rs.mu.Lock()
	var hit *Rule
	for _, it := range rs.items {
		if it.active && it.Match(r) {
			hit = it
			break
		}
	}
	rs.mu.Unlock()
	if hit == nil {
		return false
	}
	if hit.Hang {
		select {
		case <-r.Context().Done():
		case <-time.After(60 * time.Second):
		}
		return true
	}
	if hit.Delay > 0 {
		time.Sleep(hit.Delay)
	}
	if hit.Status != 0 {
		http.Error(w, http.StatusText(hit.Status), hit.Status)
		return true
	}
	return false
}
