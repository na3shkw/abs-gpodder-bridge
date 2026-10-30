package acceptance

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/scenario"
)

// 図: diagrams/HLT-01.svg
func TestHealthz_ReturnsStatusOK(t *testing.T) {
	New(t, "HLT-01", "GET /healthz に 200・JSON・{\"status\":\"ok\"} を返す").
		ClientHealthz().
		ExpectJSONResponse(B("status", "ok")).
		Run()
}

// 図: diagrams/HLT-02.svg
func TestHealthz_NotForwardedToGoPodder(t *testing.T) {
	New(t, "HLT-02", "GET /healthz は goPodder へ中継しない").
		ClientHealthz().
		ExpectGoPodderNotReceived("/healthz").
		Run()
}

// 図: diagrams/HLT-03.svg
func TestHealthz_OKWhileDependenciesDown(t *testing.T) {
	New(t, "HLT-03", "goPodder と ABS が止まっていても、GET /healthz に 200 を返す").
		GoPodderDown().
		ABSDown().
		ClientHealthz().
		ExpectStatus(200).
		Run()
}

// 図: diagrams/PRX-01.svg
func TestProxy_ForwardsMethodPathQueryBody(t *testing.T) {
	New(t, "PRX-01", "メソッド・パス・クエリ・本文を変えずに goPodder へ中継する").
		ClientRequest("GET", "/api/2/subscriptions/alice/phone.json", "since=10", nil, "").
		ClientRequest("POST", "/api/2/subscriptions/alice/phone.json", "", nil, `{"add":["https://feeds.example.com/a.rss"]}`).
		ClientRequest("PUT", "/subscriptions/alice/phone.txt", "", nil, "https://feeds.example.com/a.rss").
		ClientRequest("DELETE", "/api/2/devices/alice/phone.json", "", nil, "").
		ExpectRelayedUnchanged().
		Run()
}

// 図: diagrams/PRX-02.svg
func TestProxy_ForwardsHeaders(t *testing.T) {
	New(t, "PRX-02", "Authorization・Cookie・任意のヘッダーを変えずに中継する").
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", map[string]string{
			"Authorization": "Basic YWxpY2U6c2VjcmV0",
			"Cookie":        "sessionid=abc123",
			"X-Custom":      "value-1",
		}, "").
		ExpectRelayedUnchanged().
		Run()
}

// 図: diagrams/PRX-03.svg
func TestProxy_DropsHopByHopHeaders(t *testing.T) {
	New(t, "PRX-03", "hop-by-hop ヘッダーは中継せず、Host は転送先にする").
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", map[string]string{
			"Connection": "X-Hop",
			"X-Hop":      "secret",
			"Keep-Alive": "timeout=5",
		}, "").
		ExpectHopByHopRemoved().
		Run()
}

// 図: diagrams/PRX-04.svg
func TestProxy_ReturnsGoPodderResponseAsIs(t *testing.T) {
	cookie := map[string]string{"Set-Cookie": "sessionid=xyz; Path=/"}
	New(t, "PRX-04", "goPodder の応答のステータス・Set-Cookie・本文を変えずに返す").
		GoPodderResponds("/api/2/auth/alice/login.json", 200, cookie, `{"ok":true}`).
		GoPodderResponds("/api/2/subscriptions/alice.json", 401, nil, "Unauthorized").
		GoPodderResponds("/api/2/devices/alice.json", 404, nil, "Not Found").
		GoPodderResponds("/api/2/updates/alice/phone.json", 500, nil, "internal error").
		ClientRequest("POST", "/api/2/auth/alice/login.json", "", nil, "").
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", nil, "").
		ClientRequest("GET", "/api/2/devices/alice.json", "", nil, "").
		ClientRequest("GET", "/api/2/updates/alice/phone.json", "", nil, "").
		ExpectEveryResponseFromGoPodder().
		Run()
}

// 図: diagrams/PRX-05.svg
func TestProxy_OnlyExactEpisodesPathIsIntercepted(t *testing.T) {
	New(t, "PRX-05", "差し込みの対象でないリクエスト（パスが違う・PUT）は中継だけを行う").
		ClientRequest("GET", "/api/2/episodes/alice.json/extra", "", nil, "").
		ClientRequest("PUT", "/api/2/episodes/alice.json", "", nil, "[]").
		ExpectEveryResponseFromGoPodder().
		ExpectNoABSRequests().
		Run()
}

// 図: diagrams/PRX-06.svg
func TestProxy_Returns502WhenGoPodderDown(t *testing.T) {
	New(t, "PRX-06", "goPodder に接続できなければ 502 を返す").
		GoPodderDown().
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", nil, "").
		ExpectStatus(502).
		Run()
}

// 図: diagrams/PRX-07.svg
func TestProxy_Returns502WhenGoPodderTimesOut(t *testing.T) {
	New(t, "PRX-07", "goPodder が GOPODDER_TIMEOUT_SECONDS 以内に応答しなければ 502 を返す").
		Env("GOPODDER_TIMEOUT_SECONDS", "1").
		GoPodderHangs(AllRequests).
		ClientRequest("GET", "/api/2/subscriptions/alice.json", "", nil, "").
		ExpectStatus(502).
		ExpectResponseWithin(5 * time.Second).
		Run()
}

// 図: diagrams/PRX-08.svg
func TestProxy_ClosesConnectionWhenHeaderIsNotFinished(t *testing.T) {
	New(t, "PRX-08", "リクエストヘッダーを 10 秒以内に送り終えなければ、中継せずに接続を切る").
		ClientStallsHeader().
		ExpectConnectionClosedAfter(10 * time.Second).
		ExpectGoPodderNotReceived("/api/2/subscriptions/alice.json").
		Run()
}
