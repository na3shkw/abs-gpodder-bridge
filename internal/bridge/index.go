package bridge

import (
	"context"
	"log/slog"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// episodeRef は ABS エピソードを一意に識別する組。
type episodeRef struct {
	LibraryItemID string
	EpisodeID     string
}

// episodeIndex はエピソード索引（6.3 節）。作ったあとは変更しない。
type episodeIndex struct {
	byFeedID map[string][]episodeRef // フィード用 ID の対応表。nil なら対応表なし
	byURL    map[string][]episodeRef // 正規化した元の配信 URL
	byGUID   map[string][]episodeRef
	duration map[episodeRef]float64
}

// match は 6.2 節の手順でアクションのエピソードを照合する。
func (ix *episodeIndex) match(episode, guid string) (episodeRef, bool) {
	try := func(m map[string][]episodeRef, key string) (ref episodeRef, ok, decided bool) {
		refs := m[key]
		switch len(refs) {
		case 0:
			return episodeRef{}, false, false
		case 1:
			return refs[0], true, true
		default:
			return episodeRef{}, false, true // 複数一致したら照合できない
		}
	}
	if ix.byFeedID != nil {
		if id, ok := feedEpisodeID(episode); ok {
			if ref, ok, decided := try(ix.byFeedID, id); decided {
				return ref, ok
			}
		}
	}
	if ref, ok, decided := try(ix.byURL, normalizeURL(episode)); decided {
		return ref, ok
	}
	if guid != "" {
		if ref, ok, _ := try(ix.byGUID, guid); ok {
			return ref, true
		}
	}
	return episodeRef{}, false
}

// feedEpisodeID は /feed/{slug}/item/{フィード用 ID}/... の形の URL からフィード用 ID を取り出す。
func feedEpisodeID(episode string) (string, bool) {
	u, err := url.Parse(episode)
	if err != nil {
		return "", false
	}
	seg := strings.Split(u.Path, "/")
	// "", "feed", slug, "item", id, 以降 1 つ以上
	if len(seg) < 6 || seg[0] != "" || seg[1] != "feed" || seg[2] == "" || seg[3] != "item" || seg[4] == "" {
		return "", false
	}
	return seg[4], true
}

// normalizeURL は 6.2 節の正規化を行う。パスとクエリ文字列は変更しない。
func normalizeURL(s string) string {
	s, _, _ = strings.Cut(s, "#")
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok {
		return s
	}
	scheme = strings.ToLower(scheme)
	end := strings.IndexAny(rest, "/?")
	if end < 0 {
		end = len(rest)
	}
	authority, tail := rest[:end], rest[end:]
	userinfo := ""
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		userinfo, authority = authority[:i+1], authority[i+1:]
	}
	authority = strings.ToLower(authority)
	switch {
	case scheme == "http" && strings.HasSuffix(authority, ":80"):
		authority = strings.TrimSuffix(authority, ":80")
	case scheme == "https" && strings.HasSuffix(authority, ":443"):
		authority = strings.TrimSuffix(authority, ":443")
	}
	return scheme + "://" + userinfo + authority + tail
}

// buildIndex は 6.3 節の手順で ABS から情報を取得し、索引を作る。
// 手順 1〜3 の失敗はエラーを返す。手順 4 だけの失敗は対応表のない索引と feedErr を返す。
func buildIndex(ctx context.Context, abs *absClient) (ix *episodeIndex, feedErr error, err error) {
	libs, err := abs.libraries(ctx)
	if err != nil {
		return nil, nil, err
	}
	var ids []string
	for _, lib := range libs {
		if lib.MediaType != "podcast" {
			continue
		}
		libIDs, err := abs.libraryItemIDs(ctx, lib.ID)
		if err != nil {
			return nil, nil, err
		}
		ids = append(ids, libIDs...)
	}
	items, err := abs.itemsBatch(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	if abs.adminToken == "" {
		return makeIndex(items, nil), nil, nil
	}
	feeds, err := abs.feeds(ctx)
	if err != nil {
		return makeIndex(items, nil), err, nil
	}
	if feeds == nil {
		feeds = []absFeed{} // フィードが 0 件でも対応表はある
	}
	return makeIndex(items, feeds), nil, nil
}

// makeIndex は取得した情報から索引を組み立てる。feeds が nil なら対応表を作らない。
func makeIndex(items []absItem, feeds []absFeed) *episodeIndex {
	ix := &episodeIndex{
		byURL:    map[string][]episodeRef{},
		byGUID:   map[string][]episodeRef{},
		duration: map[episodeRef]float64{},
	}
	byPath := map[string][]episodeRef{}
	for _, item := range items {
		for _, ep := range item.Media.Episodes {
			ref := episodeRef{item.ID, ep.ID}
			if _, dup := ix.duration[ref]; dup {
				continue
			}
			ix.duration[ref] = ep.Duration
			if ep.Enclosure != nil && ep.Enclosure.URL != "" {
				k := normalizeURL(ep.Enclosure.URL)
				ix.byURL[k] = append(ix.byURL[k], ref)
			}
			if ep.GUID != "" {
				ix.byGUID[ep.GUID] = append(ix.byGUID[ep.GUID], ref)
			}
			if ep.AudioFile != nil && ep.AudioFile.Metadata.Path != "" {
				p := ep.AudioFile.Metadata.Path
				byPath[p] = append(byPath[p], ref)
			}
		}
	}
	if feeds == nil {
		return ix
	}
	ix.byFeedID = map[string][]episodeRef{}
	for _, f := range feeds {
		if f.EntityType != "libraryItem" {
			continue
		}
		for _, fe := range f.Episodes {
			for _, ref := range byPath[fe.FullPath] {
				ix.byFeedID[fe.ID] = appendUnique(ix.byFeedID[fe.ID], ref)
			}
		}
	}
	return ix
}

func appendUnique(refs []episodeRef, ref episodeRef) []episodeRef {
	for _, r := range refs {
		if r == ref {
			return refs
		}
	}
	return append(refs, ref)
}

// indexHolder は現在の索引を持ち、ABS_INDEX_SCHEDULE に従って作り直す。
type indexHolder struct {
	cur atomic.Pointer[episodeIndex]
}

func (h *indexHolder) get() *episodeIndex {
	if ix := h.cur.Load(); ix != nil {
		return ix
	}
	return &episodeIndex{}
}

func (h *indexHolder) rebuild(ctx context.Context, abs *absClient, log *slog.Logger) {
	ix, feedErr, err := buildIndex(ctx, abs)
	if err != nil {
		log.Warn("failed to build episode index; keeping the previous index", "error", err)
		return
	}
	if feedErr != nil {
		log.Warn("failed to get ABS feeds; using the index without the feed ID table", "error", feedErr)
	}
	h.cur.Store(ix)
	log.Info("episode index built", "episodes", len(ix.duration), "feedIds", len(ix.byFeedID))
}

// run は起動時に 1 回索引を作り、以後 next が返す時刻に作り直す。
func (h *indexHolder) run(ctx context.Context, next func(time.Time) time.Time, abs *absClient, log *slog.Logger) {
	h.rebuild(ctx, abs, log)
	for {
		at := next(time.Now())
		if at.IsZero() { // 該当する時刻がない
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(at)):
		}
		h.rebuild(ctx, abs, log)
	}
}
