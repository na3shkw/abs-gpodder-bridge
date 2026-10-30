package bridge

import (
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // OS にタイムゾーンのデータがなくても TZ を解釈できるようにする

	"github.com/robfig/cron/v3"
)

// Config は環境変数から読み取った設定（仕様 2.1 節）。
type Config struct {
	ListenPort      int
	GopodderURL     string // 末尾の "/" は取り除いてある
	GopodderTimeout time.Duration
	ABSURL          string // 末尾の "/" は取り除いてある
	ABSToken        string
	ABSAdminToken   string // 空なら未設定
	ABSTimeout      time.Duration
	IndexSchedule   cron.Schedule
	LogLevel        slog.Level
	Location        *time.Location // TZ。cron 式とログの時刻に使う
}

// LoadConfig は getenv から設定を読み取る。値が空の変数は未設定として扱う。
func LoadConfig(getenv func(string) string) (*Config, error) {
	c := &Config{}
	var err error

	if c.ListenPort, err = parsePort(getenv("LISTEN_PORT"), 8080); err != nil {
		return nil, fmt.Errorf("LISTEN_PORT: %w", err)
	}
	if c.GopodderURL, err = parseBaseURL(getenv("GOPODDER_URL")); err != nil {
		return nil, fmt.Errorf("GOPODDER_URL: %w", err)
	}
	if c.GopodderTimeout, err = parseSeconds(getenv("GOPODDER_TIMEOUT_SECONDS"), 30); err != nil {
		return nil, fmt.Errorf("GOPODDER_TIMEOUT_SECONDS: %w", err)
	}
	if c.ABSURL, err = parseBaseURL(getenv("ABS_URL")); err != nil {
		return nil, fmt.Errorf("ABS_URL: %w", err)
	}
	if c.ABSToken = getenv("ABS_TOKEN"); c.ABSToken == "" {
		return nil, fmt.Errorf("ABS_TOKEN: not set")
	}
	c.ABSAdminToken = getenv("ABS_ADMIN_TOKEN")
	if c.ABSTimeout, err = parseSeconds(getenv("ABS_TIMEOUT_SECONDS"), 5); err != nil {
		return nil, fmt.Errorf("ABS_TIMEOUT_SECONDS: %w", err)
	}
	sched := getenv("ABS_INDEX_SCHEDULE")
	if sched == "" {
		sched = "@every 15m"
	}
	if c.IndexSchedule, err = parseSchedule(sched); err != nil {
		return nil, fmt.Errorf("ABS_INDEX_SCHEDULE: %w", err)
	}
	switch lv := getenv("LOG_LEVEL"); lv {
	case "", "info":
		c.LogLevel = slog.LevelInfo
	case "debug":
		c.LogLevel = slog.LevelDebug
	case "warn":
		c.LogLevel = slog.LevelWarn
	case "error":
		c.LogLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("LOG_LEVEL: invalid value %q", lv)
	}
	tz := getenv("TZ")
	if tz == "" {
		tz = "UTC"
	}
	// "Local" は IANA のタイムゾーン名ではないので受け付けない。
	if c.Location, err = time.LoadLocation(tz); err != nil || tz == "Local" {
		return nil, fmt.Errorf("TZ: invalid time zone %q", tz)
	}
	return c, nil
}

// NextIndexTime は now より後で、索引を次に作る時刻を返す。cron 式は TZ の時刻で解釈する。
// 該当する時刻がなければゼロ値を返す。
func (c *Config) NextIndexTime(now time.Time) time.Time {
	return c.IndexSchedule.Next(now.In(c.Location))
}

func parsePort(s string, def int) (int, error) {
	if s == "" {
		return def, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return n, nil
}

func parseSeconds(s string, def float64) (time.Duration, error) {
	v := def
	if s != "" {
		var err error
		v, err = strconv.ParseFloat(s, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
			return 0, fmt.Errorf("invalid number of seconds %q", s)
		}
	}
	// http.Client は 0 以下をタイムアウトなしとみなすので、1 ナノ秒から time.Duration の最大値に収める。
	ns := v * float64(time.Second)
	if ns >= math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return max(time.Duration(ns), time.Nanosecond), nil
}

// parseSchedule は 2.1 節の書き方の cron 式を読み取る。
// robfig/cron が受け付ける書き方のうち、仕様にないもの（タイムゾーンの接頭辞、0 以下の @every）は不正とする。
func parseSchedule(s string) (cron.Schedule, error) {
	if strings.HasPrefix(s, "TZ=") || strings.HasPrefix(s, "CRON_TZ=") {
		return nil, fmt.Errorf("time zone prefix is not allowed; use TZ instead: %q", s)
	}
	sched, err := cron.ParseStandard(s)
	if err != nil {
		return nil, err
	}
	// robfig/cron は 1 秒未満の @every を 1 秒に切り上げるので、0 以下はここで弾く。
	if d, ok := strings.CutPrefix(s, "@every "); ok {
		if v, err := time.ParseDuration(d); err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid interval %q", s)
		}
	}
	return sched, nil
}

func parseBaseURL(s string) (string, error) {
	if s == "" {
		return "", fmt.Errorf("not set")
	}
	u, err := url.Parse(s)
	// 転送先はパスとクエリ文字列を連結して作るので、クエリ文字列やフラグメントを持つ URL は使えない。
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || strings.ContainsAny(s, "?#") {
		return "", fmt.Errorf("invalid URL %q", s)
	}
	return strings.TrimSuffix(s, "/"), nil
}
