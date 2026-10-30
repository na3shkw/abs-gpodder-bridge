package bridge

import (
	"log/slog"
	"maps"
	"math"
	"testing"
	"time"
)

// requiredEnv は必須の変数だけを設定した環境。
var requiredEnv = map[string]string{
	"GOPODDER_URL": "http://gopodder:8080",
	"ABS_URL":      "http://abs:80",
	"ABS_TOKEN":    "user-token",
}

// envWith は requiredEnv に override を重ねた getenv を返す。
func envWith(override map[string]string) func(string) string {
	env := maps.Clone(requiredEnv)
	maps.Copy(env, override)
	return func(k string) string { return env[k] }
}

func mustLoadConfig(t *testing.T, override map[string]string) *Config {
	t.Helper()
	cfg, err := LoadConfig(envWith(override))
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestLoadConfig_必須の変数だけなら任意の変数は既定値になる(t *testing.T) {
	cfg := mustLoadConfig(t, nil)

	if cfg.ListenPort != 8080 {
		t.Errorf("ListenPort = %d, want 8080", cfg.ListenPort)
	}
	if cfg.GopodderTimeout != 30*time.Second {
		t.Errorf("GopodderTimeout = %v, want 30s", cfg.GopodderTimeout)
	}
	if cfg.ABSTimeout != 5*time.Second {
		t.Errorf("ABSTimeout = %v, want 5s", cfg.ABSTimeout)
	}
	if cfg.ABSAdminToken != "" {
		t.Errorf("ABSAdminToken = %q, want empty", cfg.ABSAdminToken)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.Location.String() != "UTC" {
		t.Errorf("Location = %v, want UTC", cfg.Location)
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if got, want := cfg.NextIndexTime(now), now.Add(15*time.Minute); !got.Equal(want) {
		t.Errorf("既定のスケジュールの次回 = %v, want %v（@every 15m）", got, want)
	}
}

func TestLoadConfig_URLの末尾のスラッシュは有無を問わない(t *testing.T) {
	cfg := mustLoadConfig(t, map[string]string{
		"GOPODDER_URL": "http://gopodder:8080/",
		"ABS_URL":      "http://abs/prefix/",
	})

	if cfg.GopodderURL != "http://gopodder:8080" || cfg.ABSURL != "http://abs/prefix" {
		t.Errorf("GopodderURL = %q, ABSURL = %q", cfg.GopodderURL, cfg.ABSURL)
	}
}

func TestLoadConfig_指定した値を読み取る(t *testing.T) {
	cfg := mustLoadConfig(t, map[string]string{
		"LISTEN_PORT":              "9000",
		"GOPODDER_TIMEOUT_SECONDS": "0.5",
		"ABS_TIMEOUT_SECONDS":      "12",
		"ABS_ADMIN_TOKEN":          "admin-token",
		"LOG_LEVEL":                "debug",
		"TZ":                       "Asia/Tokyo",
	})

	if cfg.ListenPort != 9000 || cfg.GopodderTimeout != 500*time.Millisecond || cfg.ABSTimeout != 12*time.Second ||
		cfg.ABSAdminToken != "admin-token" || cfg.LogLevel != slog.LevelDebug || cfg.Location.String() != "Asia/Tokyo" {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoadConfig_LOG_LEVELは4種類を受け付ける(t *testing.T) {
	tests := []struct {
		value string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			cfg := mustLoadConfig(t, map[string]string{"LOG_LEVEL": tt.value})

			if cfg.LogLevel != tt.want {
				t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, tt.want)
			}
		})
	}
}

func TestLoadConfig_範囲の端の値を受け付ける(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"ポートの下限 1", "LISTEN_PORT", "1"},
		{"ポートの上限 65535", "LISTEN_PORT", "65535"},
		{"小数のタイムアウト", "ABS_TIMEOUT_SECONDS", "0.001"},
		{"曜日の名前", "ABS_INDEX_SCHEDULE", "0 9 * * MON-FRI"},
		{"月の名前", "ABS_INDEX_SCHEDULE", "0 0 1 JAN *"},
		{"曜日の 6（土曜日）", "ABS_INDEX_SCHEDULE", "0 0 * * 6"},
		{"@hourly", "ABS_INDEX_SCHEDULE", "@hourly"},
		{"@yearly", "ABS_INDEX_SCHEDULE", "@yearly"},
		{"@every 1s", "ABS_INDEX_SCHEDULE", "@every 1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(envWith(map[string]string{tt.key: tt.value}))

			if err != nil {
				t.Errorf("%s=%q: %v", tt.key, tt.value, err)
			}
		})
	}
}

func TestLoadConfig_極端な秒数でもタイムアウトは正の長さになる(t *testing.T) {
	tests := []struct {
		name, value string
		want        time.Duration
	}{
		{"1 ナノ秒より短い", "1e-12", time.Nanosecond},
		{"time.Duration で表せないほど長い", "1e12", time.Duration(math.MaxInt64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mustLoadConfig(t, map[string]string{"ABS_TIMEOUT_SECONDS": tt.value, "GOPODDER_TIMEOUT_SECONDS": tt.value})

			if cfg.ABSTimeout != tt.want || cfg.GopodderTimeout != tt.want {
				t.Errorf("ABSTimeout = %v, GopodderTimeout = %v, want %v", cfg.ABSTimeout, cfg.GopodderTimeout, tt.want)
			}
		})
	}
}

func TestLoadConfig_空の値は未設定として扱う(t *testing.T) {
	cfg := mustLoadConfig(t, map[string]string{
		"LISTEN_PORT":              "",
		"GOPODDER_TIMEOUT_SECONDS": "",
		"ABS_TIMEOUT_SECONDS":      "",
		"ABS_ADMIN_TOKEN":          "",
		"ABS_INDEX_SCHEDULE":       "",
		"LOG_LEVEL":                "",
		"TZ":                       "",
	})

	if cfg.ListenPort != 8080 || cfg.GopodderTimeout != 30*time.Second || cfg.ABSTimeout != 5*time.Second ||
		cfg.ABSAdminToken != "" || cfg.LogLevel != slog.LevelInfo || cfg.Location.String() != "UTC" {
		t.Errorf("空の値で既定値にならない: %+v", cfg)
	}
}

func TestLoadConfig_不正な値ならエラーになる(t *testing.T) {
	tests := []struct {
		name, key, value string
	}{
		{"GOPODDER_URL が空", "GOPODDER_URL", ""},
		{"ABS_URL が空", "ABS_URL", ""},
		{"ABS_TOKEN が空", "ABS_TOKEN", ""},
		{"URL でない", "GOPODDER_URL", "not a url"},
		{"http でも https でもない", "ABS_URL", "ftp://abs"},
		{"ホストがない", "ABS_URL", "http://"},
		{"ポートが数値でない", "LISTEN_PORT", "abc"},
		{"ポートが小数", "LISTEN_PORT", "80.5"},
		{"ポートが 0", "LISTEN_PORT", "0"},
		{"ポートが 65536", "LISTEN_PORT", "65536"},
		{"タイムアウトが数値でない", "GOPODDER_TIMEOUT_SECONDS", "ten"},
		{"タイムアウトが 0", "ABS_TIMEOUT_SECONDS", "0"},
		{"タイムアウトが負", "ABS_TIMEOUT_SECONDS", "-1"},
		{"タイムアウトが NaN", "ABS_TIMEOUT_SECONDS", "NaN"},
		{"タイムアウトが無限大", "GOPODDER_TIMEOUT_SECONDS", "Inf"},
		{"cron の分が範囲外", "ABS_INDEX_SCHEDULE", "61 * * * *"},
		{"cron のフィールドが 4 つ", "ABS_INDEX_SCHEDULE", "* * * *"},
		{"cron の曜日 7", "ABS_INDEX_SCHEDULE", "0 0 * * 7"},
		{"@every の間隔が解釈できない", "ABS_INDEX_SCHEDULE", "@every soon"},
		{"@every の間隔が負", "ABS_INDEX_SCHEDULE", "@every -5m"},
		{"@every の間隔が 0", "ABS_INDEX_SCHEDULE", "@every 0s"},
		{"cron 式に TZ= が付いている", "ABS_INDEX_SCHEDULE", "TZ=Asia/Tokyo 0 3 * * *"},
		{"cron 式に CRON_TZ= が付いている", "ABS_INDEX_SCHEDULE", "CRON_TZ=Asia/Tokyo 0 3 * * *"},
		{"URL にクエリ文字列がある", "GOPODDER_URL", "http://gopodder:8080?x=1"},
		{"URL が ? で終わる", "GOPODDER_URL", "http://gopodder:8080?"},
		{"URL にフラグメントがある", "ABS_URL", "http://abs#top"},
		{"LOG_LEVEL が未知", "LOG_LEVEL", "verbose"},
		{"LOG_LEVEL が大文字", "LOG_LEVEL", "INFO"},
		{"TZ が存在しない", "TZ", "Asia/Tokio"},
		{"TZ の大文字小文字が違う", "TZ", "asia/tokyo"},
		{"TZ が Local", "TZ", "Local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadConfig(envWith(map[string]string{tt.key: tt.value}))

			if err == nil {
				t.Errorf("%s=%q でエラーにならない", tt.key, tt.value)
			}
		})
	}
}

func TestNextIndexTime_cron式はTZの時刻で解釈する(t *testing.T) {
	tokyo := mustLocation(t, "Asia/Tokyo")
	newYork := mustLocation(t, "America/New_York")
	tests := []struct {
		name     string
		tz       string
		schedule string
		now      time.Time
		want     time.Time
	}{
		{
			name: "日本時間の 7:30（今が 9:00 なら翌日）", tz: "Asia/Tokyo", schedule: "30 7 * * *",
			now:  time.Date(2026, 10, 26, 9, 0, 0, 0, tokyo),
			want: time.Date(2026, 10, 27, 7, 30, 0, 0, tokyo),
		},
		{
			name: "日本時間の 7:30（今が 7:00 なら当日）", tz: "Asia/Tokyo", schedule: "30 7 * * *",
			now:  time.Date(2026, 10, 26, 7, 0, 0, 0, tokyo),
			want: time.Date(2026, 10, 26, 7, 30, 0, 0, tokyo),
		},
		{
			name: "UTC の 7:30", tz: "UTC", schedule: "30 7 * * *",
			now:  time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 10, 26, 7, 30, 0, 0, time.UTC),
		},
		{
			name: "渡した時刻のタイムゾーンには左右されない", tz: "Asia/Tokyo", schedule: "30 7 * * *",
			now:  time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), // 日本時間 9:00
			want: time.Date(2026, 10, 27, 7, 30, 0, 0, tokyo),
		},
		{
			name: "@daily は TZ の 0 時", tz: "Asia/Tokyo", schedule: "@daily",
			now:  time.Date(2026, 10, 26, 12, 0, 0, 0, tokyo),
			want: time.Date(2026, 10, 27, 0, 0, 0, 0, tokyo),
		},
		{
			name: "曜日は TZ の曜日（日本時間の月曜 9:00）", tz: "Asia/Tokyo", schedule: "0 9 * * MON",
			now:  time.Date(2026, 10, 26, 10, 0, 0, 0, tokyo), // 月曜日の 10:00
			want: time.Date(2026, 11, 2, 9, 0, 0, 0, tokyo),
		},
		{
			name: "夏時間の終わりをまたいでも現地の 3:00", tz: "America/New_York", schedule: "0 3 * * *",
			now:  time.Date(2026, 10, 31, 12, 0, 0, 0, newYork),
			want: time.Date(2026, 11, 1, 3, 0, 0, 0, newYork),
		},
		{
			name: "@every は TZ に関係なく今からの間隔", tz: "Asia/Tokyo", schedule: "@every 15m",
			now:  time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 10, 26, 0, 15, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := mustLoadConfig(t, map[string]string{"TZ": tt.tz, "ABS_INDEX_SCHEDULE": tt.schedule})

			got := cfg.NextIndexTime(tt.now)

			if !got.Equal(tt.want) {
				t.Errorf("NextIndexTime(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}
