package bridge

import (
	"encoding/json"
	"testing"
)

func (d direction) String() string {
	return [...]string{"何もしない", "goPodder → ABS", "ABS → goPodder"}[d]
}

// playState は goPodder 側の再生の状態。total が 0 なら total なし。
func playState(position, total float64, when int64) *gpState {
	return &gpState{
		act:  action{Action: "play", HasPosition: true, Position: position, HasTotal: total != 0, Total: total},
		play: true,
		when: when,
	}
}

func nonPlayState() *gpState { return &gpState{act: action{Action: "download"}} }

// progressAt は ABS 進捗。lastUpdate はミリ秒。
func progressAt(currentTime float64, finished bool, lastUpdateMs float64) *absProgress {
	return &absProgress{CurrentTime: currentTime, Duration: 600, IsFinished: finished, LastUpdate: lastUpdateMs}
}

func TestDecide_更新時刻が新しい方を正とする(t *testing.T) {
	const sec = 1_700_000_000 // goPodder 側の時刻（秒）
	const ms = sec * 1000     // 同じ時刻の ABS 側（ミリ秒）
	tests := []struct {
		name string
		a    *gpState
		p    *absProgress
		want direction
	}{
		{"再生以外の状態で ABS 進捗がなければ何もしない", nonPlayState(), nil, doNothing},
		{"再生以外の状態で ABS 進捗があれば ABS が古くても ABS → goPodder", nonPlayState(), progressAt(10, false, 1000), toGopodder},
		{"ABS 進捗がなければ goPodder → ABS", playState(10, 0, sec), nil, toABS},
		{"goPodder が新しく位置が違えば goPodder → ABS", playState(100, 600, sec+1), progressAt(50, false, ms), toABS},
		{"ABS が新しく位置が違えば ABS → goPodder", playState(100, 600, sec), progressAt(50, false, ms+1000), toGopodder},
		{"時刻が等しく位置が違えば何もしない", playState(100, 600, sec), progressAt(50, false, ms), doNothing},
		{"ABS の時刻はミリ秒を切り捨てて秒で比べる（等しい）", playState(100, 600, sec), progressAt(50, false, ms+999), doNothing},
		{"ABS の時刻はミリ秒を切り捨てて秒で比べる（goPodder が新しい）", playState(100, 600, sec+1), progressAt(50, false, ms+999), toABS},
		{"未完了どうしで位置の差が 1 秒以下なら同じ状態", playState(100, 600, sec+60), progressAt(101, false, ms), doNothing},
		{"未完了どうしで位置の差が 1 秒を超えれば違う状態", playState(100, 600, sec+60), progressAt(101.5, false, ms), toABS},
		{"未完了どうしで ABS が新しくても位置の差が 1 秒以下なら同じ状態", playState(100, 600, sec), progressAt(99, false, ms+60000), doNothing},
		{"両方が完了なら位置が違っても同じ状態", playState(600, 600, sec), progressAt(0, true, ms+60000), doNothing},
		{"goPodder が完了で ABS が未完了なら違う状態", playState(600, 600, sec+60), progressAt(600, false, ms), toABS},
		{"goPodder が未完了で ABS が完了なら同じ位置でも違う状態", playState(600, 0, sec+60), progressAt(600, true, ms), toABS},
		{"total が 0 なら position が大きくても完了ではない", playState(700, 0, sec), progressAt(0, true, ms+60000), toGopodder},
		{"position が total を超えていれば完了", playState(610, 600, sec), progressAt(0, true, ms+60000), doNothing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decide(tt.a, tt.p)

			if got != tt.want {
				t.Errorf("decide = %v, want %v", got, tt.want)
			}
		})
	}
}

func jsonString(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAbsUpdateBody_playアクションの内容でABSを更新する本文を作る(t *testing.T) {
	tests := []struct {
		name string
		a    action
		want string
	}{
		{"total があれば duration を送る", action{Position: 120.5, HasTotal: true, Total: 600},
			`{"currentTime":120.5,"duration":600}`},
		{"position が total に届けば isFinished を送る", action{Position: 600, HasTotal: true, Total: 600},
			`{"currentTime":600,"duration":600,"isFinished":true}`},
		{"position が total を超えても isFinished を送る", action{Position: 650, HasTotal: true, Total: 600},
			`{"currentTime":650,"duration":600,"isFinished":true}`},
		{"total がなければ currentTime だけ", action{Position: 30},
			`{"currentTime":30}`},
		{"total が 0 なら duration も isFinished も送らない", action{Position: 30, HasTotal: true, Total: 0},
			`{"currentTime":30}`},
		{"position 0 も送る", action{Position: 0, HasTotal: true, Total: 600},
			`{"currentTime":0,"duration":600}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonString(t, absUpdateBody(&tt.a))

			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGopodderAction_ABS進捗からgoPodderへ書き込むアクションを作る(t *testing.T) {
	state := action{Podcast: "http://p/feed", Episode: "http://p/1.mp3", Action: "download"}
	withGUID := state
	withGUID.GUID = "g1"
	const lastUpdate = 1704164645123 // 2024-01-02T03:04:05.123Z
	tests := []struct {
		name          string
		state         action
		p             absProgress
		indexDuration float64
		want          string
	}{
		{"ABS の duration を total に使い、小数は切り捨てる", state,
			absProgress{CurrentTime: 120.9, Duration: 600.7, LastUpdate: lastUpdate}, 300,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","action":"play","timestamp":"2024-01-02T03:04:05","started":0,"position":120,"total":600}`},
		{"ABS の duration が 0 なら索引の長さを使う", state,
			absProgress{CurrentTime: 120, Duration: 0, LastUpdate: lastUpdate}, 300.2,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","action":"play","timestamp":"2024-01-02T03:04:05","started":0,"position":120,"total":300}`},
		{"長さが分からなければ total と started を含めない", state,
			absProgress{CurrentTime: 120, LastUpdate: lastUpdate}, 0,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","action":"play","timestamp":"2024-01-02T03:04:05","position":120}`},
		{"完了なら position を total と同じにする", state,
			absProgress{CurrentTime: 0, Duration: 0, IsFinished: true, LastUpdate: lastUpdate}, 300,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","action":"play","timestamp":"2024-01-02T03:04:05","started":0,"position":300,"total":300}`},
		{"完了でも total がなければ currentTime を使う", state,
			absProgress{CurrentTime: 42.5, IsFinished: true, LastUpdate: lastUpdate}, 0,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","action":"play","timestamp":"2024-01-02T03:04:05","position":42}`},
		{"goPodder の状態の guid を写す", withGUID,
			absProgress{CurrentTime: 1, LastUpdate: lastUpdate}, 0,
			`{"podcast":"http://p/feed","episode":"http://p/1.mp3","guid":"g1","action":"play","timestamp":"2024-01-02T03:04:05","position":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonString(t, gopodderAction(&tt.state, &tt.p, tt.indexDuration))

			if got != tt.want {
				t.Errorf("got  %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestGopodderAction_timestampはローカルタイムゾーンにかかわらずUTCで書く(t *testing.T) {
	withLocalTime(t, mustLocation(t, "Asia/Tokyo"))
	p := absProgress{LastUpdate: 1704164645000}

	got := gopodderAction(&action{}, &p, 0)

	if got.Timestamp != "2024-01-02T03:04:05" {
		t.Errorf("Timestamp = %q", got.Timestamp)
	}
}

// actionsFromJSON は goPodder が返す actions を読み取る。
func actionsFromJSON(t *testing.T, s string) []action {
	t.Helper()
	var raws []json.RawMessage
	if err := json.Unmarshal([]byte(s), &raws); err != nil {
		t.Fatal(err)
	}
	var out []action
	for _, raw := range raws {
		out = append(out, mustParseAction(t, string(raw)))
	}
	return out
}

// describe は状態を "episode:action:position" の並びにする。
func describe(states []*gpState) []string {
	var out []string
	for _, st := range states {
		out = append(out, st.act.Episode+":"+st.act.Action+":"+jsonNumber(st.act.Position))
	}
	return out
}

func jsonNumber(f float64) string { b, _ := json.Marshal(f); return string(b) }

func TestEpisodeStates_エピソードごとの状態を求めて古い順に並べる(t *testing.T) {
	tests := []struct {
		name    string
		actions string
		want    []string
	}{
		{"同じ組の play のうち timestamp が最も新しいものを使う", `[
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:02","position":2},
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:03","position":3},
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01","position":1}]`,
			[]string{"e1:play:3"}},
		{"timestamp のない play は使わない", `[
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01","position":1},
			{"podcast":"p","episode":"e1","action":"play","position":9}]`,
			[]string{"e1:play:1"}},
		{"position が負またはない play は使わない", `[
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01","position":-1},
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01"}]`,
			nil},
		{"使える play がなければ再生以外の状態にする", `[
			{"podcast":"p","episode":"e1","action":"play","position":5},
			{"podcast":"p","episode":"e1","action":"download"}]`,
			[]string{"e1:download:0"}},
		{"再生以外の記録が複数あれば配列の最後のものを使う", `[
			{"podcast":"p","episode":"e1","action":"download"},
			{"podcast":"p","episode":"e1","action":"delete"},
			{"podcast":"p","episode":"e1","action":"new"}]`,
			[]string{"e1:new:0"}},
		{"使える play があれば再生以外の記録より優先する", `[
			{"podcast":"p","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01","position":1},
			{"podcast":"p","episode":"e1","action":"delete"}]`,
			[]string{"e1:play:1"}},
		{"podcast が違えば別の組", `[
			{"podcast":"p1","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:01","position":1},
			{"podcast":"p2","episode":"e1","action":"play","timestamp":"2024-01-01T00:00:02","position":2}]`,
			[]string{"e1:play:1", "e1:play:2"}},
		{"再生の状態の時刻が古い順で、再生以外の状態はそれより前", `[
			{"podcast":"p","episode":"late","action":"play","timestamp":"2024-01-01T00:00:09","position":9},
			{"podcast":"p","episode":"early","action":"play","timestamp":"2024-01-01T00:00:01","position":1},
			{"podcast":"p","episode":"dl","action":"download"},
			{"podcast":"p","episode":"mid","action":"play","timestamp":"2024-01-01T09:00:05+09:00","position":5}]`,
			[]string{"dl:download:0", "early:play:1", "mid:play:5", "late:play:9"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := describe(episodeStates(actionsFromJSON(t, tt.actions)))

			if jsonString(t, got) != jsonString(t, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
