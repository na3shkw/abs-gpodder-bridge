package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// absClient は ABS の REST API（10.1 節）を呼ぶ。
type absClient struct {
	baseURL    string
	token      string
	adminToken string
	http       *http.Client
}

func (c *absClient) do(ctx context.Context, method, path, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: status %d", method, path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decoding response: %w", method, path, err)
	}
	return nil
}

type absLibrary struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
}

func (c *absClient) libraries(ctx context.Context) ([]absLibrary, error) {
	var r struct {
		Libraries []absLibrary `json:"libraries"`
	}
	err := c.do(ctx, http.MethodGet, "/api/libraries", c.token, nil, &r)
	return r.Libraries, err
}

func (c *absClient) libraryItemIDs(ctx context.Context, libraryID string) ([]string, error) {
	var r struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/libraries/"+url.PathEscape(libraryID)+"/items", c.token, nil, &r); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(r.Results))
	for _, it := range r.Results {
		ids = append(ids, it.ID)
	}
	return ids, nil
}

type absEpisode struct {
	ID        string  `json:"id"`
	GUID      string  `json:"guid"`
	Duration  float64 `json:"duration"`
	Enclosure *struct {
		URL string `json:"url"`
	} `json:"enclosure"`
	AudioFile *struct {
		Metadata struct {
			Path string `json:"path"`
		} `json:"metadata"`
	} `json:"audioFile"`
}

type absItem struct {
	ID    string `json:"id"`
	Media struct {
		Episodes []absEpisode `json:"episodes"`
	} `json:"media"`
}

func (c *absClient) itemsBatch(ctx context.Context, ids []string) ([]absItem, error) {
	var r struct {
		LibraryItems []absItem `json:"libraryItems"`
	}
	if ids == nil {
		ids = []string{} // null ではなく [] を送る
	}
	in := map[string][]string{"libraryItemIds": ids}
	err := c.do(ctx, http.MethodPost, "/api/items/batch/get", c.token, in, &r)
	return r.LibraryItems, err
}

type absFeed struct {
	EntityType string `json:"entityType"`
	Episodes   []struct {
		ID       string `json:"id"`
		FullPath string `json:"fullPath"`
	} `json:"episodes"`
}

func (c *absClient) feeds(ctx context.Context) ([]absFeed, error) {
	var r struct {
		Feeds []absFeed `json:"feeds"`
	}
	err := c.do(ctx, http.MethodGet, "/api/feeds", c.adminToken, nil, &r)
	return r.Feeds, err
}

// absProgress は ABS 進捗（episodeId を持つ mediaProgress）。
type absProgress struct {
	LibraryItemID string  `json:"libraryItemId"`
	EpisodeID     string  `json:"episodeId"`
	CurrentTime   float64 `json:"currentTime"`
	Duration      float64 `json:"duration"`
	IsFinished    bool    `json:"isFinished"`
	LastUpdate    float64 `json:"lastUpdate"` // ミリ秒の UNIX 時刻
}

// lastUpdateSec は比較に使う秒単位の時刻（11.1 節）。
func (p *absProgress) lastUpdateSec() int64 { return int64(p.LastUpdate) / 1000 }

func (c *absClient) progress(ctx context.Context) (map[episodeRef]*absProgress, error) {
	var r struct {
		MediaProgress []*absProgress `json:"mediaProgress"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/me/progress", c.token, nil, &r); err != nil {
		return nil, fmt.Errorf("%w (ABS may be older than v2.36.0)", err)
	}
	m := make(map[episodeRef]*absProgress)
	for _, p := range r.MediaProgress {
		if p != nil && p.EpisodeID != "" {
			m[episodeRef{p.LibraryItemID, p.EpisodeID}] = p
		}
	}
	return m, nil
}

func (c *absClient) updateProgress(ctx context.Context, ref episodeRef, body map[string]any) error {
	path := "/api/me/progress/" + url.PathEscape(ref.LibraryItemID) + "/" + url.PathEscape(ref.EpisodeID)
	return c.do(ctx, http.MethodPatch, path, c.token, body, nil)
}
