package bridge

import (
	"encoding/json"
	"testing"
	"time"
)

// withLocalTime は、テストの間だけプロセスのローカルタイムゾーンを loc にする。
// TZ に左右されないことを確かめるために使う。並列には動かさないこと。
func withLocalTime(t *testing.T, loc *time.Location) {
	t.Helper()
	orig := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = orig })
}

func TestParseTimestamp_受け付ける形式はUTCの時刻として読む(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{"タイムゾーン表記なし", "2024-01-02T03:04:05", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"末尾に Z", "2024-01-02T03:04:05Z", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"小数秒", "2024-01-02T03:04:05.678", time.Date(2024, 1, 2, 3, 4, 5, 678e6, time.UTC)},
		{"小数秒と Z", "2024-01-02T03:04:05.5Z", time.Date(2024, 1, 2, 3, 4, 5, 5e8, time.UTC)},
		{"+09:00 の時差は UTC に直す", "2024-01-02T12:04:05+09:00", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"-05:30 の時差は UTC に直す", "2024-01-01T21:34:05-05:30", time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"小数秒と時差", "2024-01-02T12:04:05.25+09:00", time.Date(2024, 1, 2, 3, 4, 5, 25e7, time.UTC)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimestamp(tt.input)

			if err != nil {
				t.Fatalf("parseTimestamp(%q): %v", tt.input, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseTimestamp(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTimestamp_タイムゾーン表記がなければローカルタイムゾーンにかかわらずUTCとみなす(t *testing.T) {
	withLocalTime(t, mustLocation(t, "Asia/Tokyo"))

	got, err := parseTimestamp("2024-01-02T03:04:05")

	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC); !got.Equal(want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestParseTimestamp_時刻として解釈できなければエラー(t *testing.T) {
	for _, input := range []string{
		"",
		"yesterday",
		"2024-01-02",
		"2024-01-02 03:04:05",
		"2024/01/02T03:04:05",
		"2024-13-02T03:04:05",
		"2024-01-02T25:04:05",
		"1704164645",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := parseTimestamp(input); err == nil {
				t.Errorf("parseTimestamp(%q) がエラーにならない", input)
			}
		})
	}
}

func TestFormatTimestamp_UTCの秒までをタイムゾーン表記なしで書く(t *testing.T) {
	tests := []struct {
		unix int64
		want string
	}{
		{0, "1970-01-01T00:00:00"},
		{1704164645, "2024-01-02T03:04:05"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatTimestamp(tt.unix); got != tt.want {
				t.Errorf("formatTimestamp(%d) = %q, want %q", tt.unix, got, tt.want)
			}
		})
	}
}

func TestFormatTimestamp_ローカルタイムゾーンにかかわらずUTCで書く(t *testing.T) {
	withLocalTime(t, mustLocation(t, "Asia/Tokyo"))

	got := formatTimestamp(1704164645)

	if want := "2024-01-02T03:04:05"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func mustParseAction(t *testing.T, s string) action {
	t.Helper()
	a, err := parseAction(json.RawMessage(s))
	if err != nil {
		t.Fatalf("parseAction(%s): %v", s, err)
	}
	return a
}

func TestParseAction_有効なアクションの値を読み取る(t *testing.T) {
	a := mustParseAction(t, `{"podcast":"http://p/feed","episode":"http://p/1.mp3","guid":"g1","action":"play",
		"timestamp":"2024-01-02T12:04:05+09:00","started":0,"position":12.5,"total":600}`)

	want := action{
		Podcast: "http://p/feed", Episode: "http://p/1.mp3", GUID: "g1", Action: "play",
		HasTimestamp: true, Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC).Unix(),
		HasPosition: true, Position: 12.5, HasTotal: true, Total: 600,
	}
	if a != want {
		t.Errorf("got %+v\nwant %+v", a, want)
	}
}

func TestParseAction_任意の値はなくても有効(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"guid・timestamp・数値がない", `{"podcast":"p","episode":"e","action":"download"}`},
		{"null はないものとして扱う", `{"podcast":"p","episode":"e","action":"play","guid":null,"timestamp":null,"position":null,"total":null,"started":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := mustParseAction(t, tt.input)

			if a.GUID != "" || a.HasTimestamp || a.HasPosition || a.HasTotal {
				t.Errorf("値がないのに値があるとされた: %+v", a)
			}
		})
	}
}

func TestParseAction_数値はJSONの数値だけを受け付け小数も受け付ける(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"整数", `{"position":10,"total":20,"started":0}`, true},
		{"小数", `{"position":10.25,"total":20.5,"started":0.5}`, true},
		{"指数表記", `{"position":1e2}`, true},
		{"負の数（形式としては有効）", `{"position":-1}`, true},
		{"数字の文字列の position", `{"position":"10"}`, false},
		{"数字の文字列の total", `{"total":"20"}`, false},
		{"数字の文字列の started", `{"started":"0"}`, false},
		{"真偽値", `{"position":true}`, false},
		{"配列", `{"total":[1]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := `{"podcast":"p","episode":"e","action":"play",` + tt.input[1:]

			_, err := parseAction(json.RawMessage(input))

			if valid := err == nil; valid != tt.valid {
				t.Errorf("parseAction(%s) valid = %v, want %v", input, valid, tt.valid)
			}
		})
	}
}

func TestParseAction_形式に合わないアクションは無効(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"podcast がない", `{"episode":"e","action":"play"}`},
		{"episode がない", `{"podcast":"p","action":"play"}`},
		{"action がない", `{"podcast":"p","episode":"e"}`},
		{"episode が空", `{"podcast":"p","episode":"","action":"play"}`},
		{"episode が文字列でない", `{"podcast":"p","episode":1,"action":"play"}`},
		{"guid が文字列でない", `{"podcast":"p","episode":"e","action":"play","guid":1}`},
		{"timestamp が解釈できない", `{"podcast":"p","episode":"e","action":"play","timestamp":"yesterday"}`},
		{"timestamp が数値", `{"podcast":"p","episode":"e","action":"play","timestamp":1704164645}`},
		{"オブジェクトでない", `["play"]`},
		{"null", `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseAction(json.RawMessage(tt.input)); err == nil {
				t.Errorf("parseAction(%s) が有効とされた", tt.input)
			}
		})
	}
}

func TestIsPlay_actionがplayかを大文字小文字を区別せずに判定する(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"play", true},
		{"PLAY", true},
		{"Play", true},
		{"download", false},
		{"delete", false},
		{"played", false},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			a := action{Action: tt.action}

			if got := a.isPlay(); got != tt.want {
				t.Errorf("isPlay(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}
