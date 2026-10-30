package e2e

import (
	"testing"

	. "github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// 図: diagrams/CFG-01.svg
func TestConfig_ExitsWhenRequiredEnvMissing(t *testing.T) {
	New(t, "CFG-01", "必須の環境変数がない、または値が空なら、終了コード 1 で起動をやめる").
		StartVariants(
			Variant{Label: "GOPODDER_URL なし", Unset: []string{"GOPODDER_URL"}},
			Variant{Label: "ABS_URL なし", Unset: []string{"ABS_URL"}},
			Variant{Label: "ABS_TOKEN なし", Unset: []string{"ABS_TOKEN"}},
			Variant{Label: "ABS_TOKEN=（空）", Set: map[string]string{"ABS_TOKEN": ""}},
		).
		ExpectEveryExitCode(1).
		Run()
}

// 図: diagrams/CFG-02.svg
func TestConfig_ExitsOnInvalidValue(t *testing.T) {
	New(t, "CFG-02", "環境変数の値が不正なら（任意の変数でも）、終了コード 1 で起動をやめる").
		StartVariants(
			Variant{Label: "LISTEN_PORT=abc", Set: map[string]string{"LISTEN_PORT": "abc"}},
			Variant{Label: "GOPODDER_URL=http://[::1", Set: map[string]string{"GOPODDER_URL": "http://[::1"}},
			Variant{Label: "ABS_URL=://no-scheme", Set: map[string]string{"ABS_URL": "://no-scheme"}},
			Variant{Label: "GOPODDER_TIMEOUT_SECONDS=abc", Set: map[string]string{"GOPODDER_TIMEOUT_SECONDS": "abc"}},
			Variant{Label: "ABS_TIMEOUT_SECONDS=abc", Set: map[string]string{"ABS_TIMEOUT_SECONDS": "abc"}},
			Variant{Label: "ABS_INDEX_SCHEDULE=every hour", Set: map[string]string{"ABS_INDEX_SCHEDULE": "every hour"}},
			Variant{Label: "LOG_LEVEL=verbose", Set: map[string]string{"LOG_LEVEL": "verbose"}},
			Variant{Label: "TZ=Asia/Tokio", Set: map[string]string{"TZ": "Asia/Tokio"}},
			Variant{Label: "ABS_INDEX_SCHEDULE=0 7 * * 7", Set: map[string]string{"ABS_INDEX_SCHEDULE": "0 7 * * 7"}},
			Variant{Label: "LISTEN_PORT=0", Set: map[string]string{"LISTEN_PORT": "0"}},
			Variant{Label: "LISTEN_PORT=65536", Set: map[string]string{"LISTEN_PORT": "65536"}},
			Variant{Label: "LISTEN_PORT=8080.5", Set: map[string]string{"LISTEN_PORT": "8080.5"}},
			Variant{Label: "GOPODDER_TIMEOUT_SECONDS=0", Set: map[string]string{"GOPODDER_TIMEOUT_SECONDS": "0"}},
			Variant{Label: "ABS_TIMEOUT_SECONDS=-1", Set: map[string]string{"ABS_TIMEOUT_SECONDS": "-1"}},
		).
		ExpectEveryExitCode(1).
		Run()
}

// 図: diagrams/CFG-03.svg
func TestConfig_AcceptsTrailingSlashInURLs(t *testing.T) {
	New(t, "CFG-03", "URL の末尾の / の有無にかかわらず、同じ先へ送る").
		TrailingSlashURLs().
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", nil, "").
		ExpectRelayedUnchanged().
		ExpectCleanPaths().
		Run()
}

// 図: diagrams/CFG-04.svg
func TestConfig_ListensOnDefaultPort(t *testing.T) {
	New(t, "CFG-04", "LISTEN_PORT を指定しなければ、既定の 8080 で待ち受ける").
		DefaultListenPort().
		ClientHealthz().
		ExpectStatus(200).
		Run()
}

// 図: diagrams/CFG-05.svg
func TestConfig_StartsWithValidValues(t *testing.T) {
	New(t, "CFG-05", "正しい値の環境変数なら（任意の変数は値が空でも）起動する").
		StartVariants(
			Variant{Label: "ABS_INDEX_SCHEDULE=30 7 * * *", Set: map[string]string{"ABS_INDEX_SCHEDULE": "30 7 * * *"}},
			Variant{Label: "ABS_INDEX_SCHEDULE=@every 15m", Set: map[string]string{"ABS_INDEX_SCHEDULE": "@every 15m"}},
			Variant{Label: "ABS_INDEX_SCHEDULE=0 7 * JAN MON", Set: map[string]string{"ABS_INDEX_SCHEDULE": "0 7 * JAN MON"}},
			Variant{Label: "ABS_INDEX_SCHEDULE=@daily", Set: map[string]string{"ABS_INDEX_SCHEDULE": "@daily"}},
			Variant{Label: "LOG_LEVEL=debug", Set: map[string]string{"LOG_LEVEL": "debug"}},
			Variant{Label: "LOG_LEVEL=info", Set: map[string]string{"LOG_LEVEL": "info"}},
			Variant{Label: "LOG_LEVEL=warn", Set: map[string]string{"LOG_LEVEL": "warn"}},
			Variant{Label: "LOG_LEVEL=error", Set: map[string]string{"LOG_LEVEL": "error"}},
			Variant{Label: "TZ=Asia/Tokyo", Set: map[string]string{"TZ": "Asia/Tokyo"}},
			Variant{Label: "ABS_ADMIN_TOKEN・2 つのタイムアウトあり", Set: map[string]string{"ABS_ADMIN_TOKEN": "x", "GOPODDER_TIMEOUT_SECONDS": "10", "ABS_TIMEOUT_SECONDS": "10"}},
			Variant{Label: "GOPODDER_TIMEOUT_SECONDS=0.5", Set: map[string]string{"GOPODDER_TIMEOUT_SECONDS": "0.5"}},
			Variant{Label: "LOG_LEVEL=（空）", Set: map[string]string{"LOG_LEVEL": ""}},
			Variant{Label: "TZ=（空）", Set: map[string]string{"TZ": ""}},
		).
		ExpectEveryStarted().
		Run()
}
