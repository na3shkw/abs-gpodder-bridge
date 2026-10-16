package e2e

// HIDDEN-E2E-CANARY-49c21fdd8cbc85c42ca2662fb9a209b4
// （隔離の確認用。harness/bin/verify-isolation がこの文字列を探す）

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/fake"
	"github.com/na3shkw/abs-gpodder-bridge/e2e/internal/harness"
)

func baseEnv(gopodder, abs *fake.Server) harness.Env {
	return harness.Env{
		"GOPODDER_URL": gopodder.URL,
		"ABS_URL":      abs.URL,
		"ABS_TOKEN":    "test-token",
	}
}

func TestHealthz_ReturnsStatusOK(t *testing.T) {
	gp, abs := fake.New(t), fake.New(t)
	b := harness.Start(t, baseEnv(gp, abs))

	resp, err := http.Get(b.URL + "/healthz")
	if err != nil {
		harness.Fatalf(t, "2.3", "GET /healthz に失敗しました")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		harness.Failf(t, "2.3", "ステータスコードが 200 ではありません（%d）", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" && ct != "application/json; charset=utf-8" {
		harness.Failf(t, "2.3", "Content-Type が application/json ではありません")
	}
	var v map[string]any
	if json.Unmarshal(body, &v) != nil || len(v) != 1 || v["status"] != "ok" {
		harness.Failf(t, "2.3", "本文が仕様どおりではありません")
	}
}

func TestHealthz_NotForwardedToGoPodder(t *testing.T) {
	gp, abs := fake.New(t), fake.New(t)
	b := harness.Start(t, baseEnv(gp, abs))

	resp, err := http.Get(b.URL + "/healthz")
	if err != nil {
		harness.Fatalf(t, "2.3", "GET /healthz に失敗しました")
	}
	resp.Body.Close()

	for _, r := range gp.Requests() {
		if r.Path == "/healthz" {
			harness.Failf(t, "2.3", "/healthz が goPodder へ中継されました")
		}
	}
}
