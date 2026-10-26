package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// action は gPodder のエピソードアクションのうち、ブリッジが使う値。
type action struct {
	Podcast      string
	Episode      string
	GUID         string // なければ空
	Action       string
	HasTimestamp bool
	Timestamp    int64 // UNIX 時刻（秒）
	HasPosition  bool
	Position     float64
	HasTotal     bool
	Total        float64
}

func (a *action) isPlay() bool { return strings.EqualFold(a.Action, "play") }

// hasPlayablePosition は position が 0 以上の数値であるかを返す（5.1 節、5.2 節）。
func (a *action) hasPlayablePosition() bool { return a.HasPosition && a.Position >= 0 }

// finished は a.total が正の数値かつ a.position >= a.total であるかを返す（5.3 節、7.2 節）。
func (a *action) finished() bool { return a.HasTotal && a.Total > 0 && a.Position >= a.Total }

var errInvalidAction = errors.New("invalid action")

// parseAction は 1 件のアクションを読み取る。11 章の形式に合わないものはエラーにする。
// null の値は、その値がないものとして扱う。
func parseAction(raw json.RawMessage) (action, error) {
	var m map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil || m == nil {
		return action{}, errInvalidAction
	}
	var a action
	var ok bool
	if a.Podcast, ok = m["podcast"].(string); !ok || a.Podcast == "" {
		return action{}, errInvalidAction
	}
	if a.Episode, ok = m["episode"].(string); !ok || a.Episode == "" {
		return action{}, errInvalidAction
	}
	if a.Action, ok = m["action"].(string); !ok || a.Action == "" {
		return action{}, errInvalidAction
	}
	if v := m["guid"]; v != nil {
		if a.GUID, ok = v.(string); !ok {
			return action{}, errInvalidAction
		}
	}
	if v := m["timestamp"]; v != nil {
		s, ok := v.(string)
		if !ok {
			return action{}, errInvalidAction
		}
		t, err := parseTimestamp(s)
		if err != nil {
			return action{}, errInvalidAction
		}
		a.HasTimestamp, a.Timestamp = true, t.Unix()
	}
	var err error
	if a.HasPosition, a.Position, err = number(m["position"]); err != nil {
		return action{}, err
	}
	if a.HasTotal, a.Total, err = number(m["total"]); err != nil {
		return action{}, err
	}
	if _, _, err = number(m["started"]); err != nil {
		return action{}, err
	}
	return a, nil
}

func number(v any) (bool, float64, error) {
	if v == nil {
		return false, 0, nil
	}
	n, ok := v.(json.Number)
	if !ok {
		return false, 0, errInvalidAction
	}
	f, err := n.Float64()
	if err != nil {
		return false, 0, errInvalidAction
	}
	return true, f, nil
}

// parseTimestamp は 11.1 節の形式の時刻を読み取る。
// タイムゾーン表記のないものは UTC とみなす。小数秒は time.Parse が受け付ける。
func parseTimestamp(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02T15:04:05", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// formatTimestamp は goPodder へ書き込む形式（11.1 節）にする。
func formatTimestamp(unixSec int64) string {
	return time.Unix(unixSec, 0).UTC().Format("2006-01-02T15:04:05")
}

// outAction は goPodder へ書き込むアクション（5.4 節）。
type outAction struct {
	Podcast   string `json:"podcast"`
	Episode   string `json:"episode"`
	GUID      string `json:"guid,omitempty"`
	Action    string `json:"action"`
	Timestamp string `json:"timestamp"`
	Started   *int64 `json:"started,omitempty"`
	Position  int64  `json:"position"`
	Total     *int64 `json:"total,omitempty"`
}
