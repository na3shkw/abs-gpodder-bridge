package e2e

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// 図: diagrams/PST-01.svg
func TestPOST_ReflectsPositionToABS(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-01", "play の記録の位置を ABS へ反映する（本文はほかの項目なし）").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120)).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-02.svg
func TestPOST_SendsDurationWithoutIsFinished(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-02", "長さがあって聴き終わっていなければ、duration を送り、isFinished は送らない").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120, "duration", 600)).
		Run()
}

// 図: diagrams/PST-03.svg
func TestPOST_SendsIsFinishedWhenPositionReachesTotal(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-03", "位置が長さ以上なら isFinished: true を送る").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(600).Total(600).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 600, "duration", 600, "isFinished", true)).
		Run()
}

// 図: diagrams/PST-04.svg
func TestPOST_OmitsDurationWhenTotalIsZero(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-04", "長さが 0 なら duration を送らない").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).Total(0).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120)).
		Run()
}

// 図: diagrams/PST-05.svg
func TestPOST_SkipsABSWhenGoPodderFails(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-05", "goPodder が 2xx 以外を返したら、ABS へは何も送らない").
		ABSEpisode(ep1).
		GoPodderFails(EpisodesPOST, 500).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectNoABSRequests().
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-06.svg
func TestPOST_IgnoresInvalidActions(t *testing.T) {
	ep1, ep2, ep3, ep4, ep5, ep6 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3"), NewEpisode("ep4"), NewEpisode("ep5"), NewEpisode("ep6")
	New(t, "PST-06", "play 以外・位置が負や数値でない（文字列の数値も）・時刻が読めない記録は無視し、ほかは反映する").
		ABSEpisode(ep1, ep2, ep3, ep4, ep5, ep6).
		ClientPOST(
			Of(ep1, "download").At(T(1)),
			Play(ep2).Pos(-5).At(T(1)),
			Play(ep3).Raw("position", "abc").At(T(1)),
			Play(ep4).Pos(100).Raw("timestamp", "yesterday"),
			Play(ep5).Pos(100).At(T(1)),
			Play(ep6).Raw("position", "120").At(T(1)),
		).
		ExpectNoPATCHFor(ep1, ep2, ep3, ep4, ep6).
		ExpectPATCH(ep5, B("currentTime", 100)).
		Run()
}

// 図: diagrams/PST-07.svg
func TestPOST_SkipsABSWhenBodyIsNotArray(t *testing.T) {
	New(t, "PST-07", "本文が JSON でない・配列でなければ、ABS へは何も送らない").
		ABSEpisode(NewEpisode("ep1")).
		ClientPOSTRaw("{not json").
		ClientPOSTRaw(`{"podcast":"https://feeds.example.com/show.rss"}`).
		ExpectNoABSRequests().
		ExpectEveryResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-08.svg
func TestPOST_IgnoresUnmatchedActions(t *testing.T) {
	ep1, ep9 := NewEpisode("ep1"), NewEpisode("ep9")
	New(t, "PST-08", "照合できない記録は無視し、照合できた記録は反映する").
		ABSEpisode(ep1).
		ClientPOST(
			Play(ep9).Pos(50).At(T(1)),
			Play(ep1).Pos(120).At(T(1)),
		).
		ExpectPATCH(ep1, B("currentTime", 120)).
		ExpectPATCHCount(1).
		Run()
}

// 図: diagrams/PST-09.svg
func TestPOST_KeepsNewestActionPerEpisode(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "PST-09", "同じエピソードの記録が複数あれば時刻が新しい方を、同じ時刻なら配列の後ろを反映する").
		ABSEpisode(ep1, ep2).
		ClientPOST(
			Play(ep1).Pos(200).Total(600).At(T(2)),
			Play(ep1).Pos(100).Total(600).At(T(1)),
			Play(ep2).Pos(300).Total(600).At(T(3)),
			Play(ep2).Pos(400).Total(600).At(T(3)),
		).
		ExpectPATCH(ep1, B("currentTime", 200, "duration", 600)).
		ExpectPATCH(ep2, B("currentTime", 400, "duration", 600)).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-10.svg
func TestPOST_SkipsWhenProgressFetchFails(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-10", "ABS の進捗を取得できなければ反映しない（応答は goPodder のまま）").
		ABSEpisode(ep1).
		ABSFails(ProgressGET, 500).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectABSAttempted(ProgressGET).
		ExpectNoPATCH().
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-11.svg
func TestPOST_ContinuesAfterOnePATCHFails(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "PST-11", "1 件の PATCH が失敗してもほかは反映し、失敗した分は再送しない").
		ABSEpisode(ep1, ep2).
		ABSFails(PATCHOf(ep1), 500).
		ClientPOST(
			Play(ep1).Pos(120).Total(600).At(T(1)),
			Play(ep2).Pos(240).Total(600).At(T(1)),
		).
		ExpectPATCHAttempts(ep1, 1).
		ExpectPATCH(ep2, B("currentTime", 240, "duration", 600)).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-12.svg
func TestPOST_RespondsWhenABSTimesOut(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-12", "ABS が ABS_TIMEOUT_SECONDS 以内に応答しなくても、goPodder の応答を返す").
		Env("ABS_TIMEOUT_SECONDS", "1").
		ABSEpisode(ep1).
		ABSHangs(AnyPATCH).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectPATCHAttempts(ep1, 1).
		ExpectResponseWithin(5 * time.Second).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-13.svg
func TestPOST_FinishesABSUpdateBeforeResponding(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-13", "ABS への反映を終えてから、クライアントへ応答する").
		ABSEpisode(ep1).
		ABSDelays(AnyPATCH, 500*time.Millisecond).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectPATCHDoneBeforeResponse(ep1).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/PST-14.svg
func TestPOST_TreatsMissingTimestampAsReceiptTime(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-14", "時刻のない記録は、受け取った時刻（今）の記録とみなす").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(0)}).
		ClientPOST(Play(ep1).Pos(100).Total(600)).
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/PST-15.svg
func TestPOST_UsesUserTokenForABS(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-15", "ABS へのリクエストには ABS_TOKEN を使う（/api/feeds だけ管理者のトークン）").
		AdminToken().
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120, "duration", 600)).
		ExpectTokenUsage().
		Run()
}

// 図: diagrams/PST-16.svg
func TestPOST_IgnoresUserNameInPath(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "PST-16", "パスのユーザー名にかかわらず、ABS_TOKEN のユーザーの進捗に反映する").
		ABSEpisode(ep1).
		ClientPOSTAs("bob", Play(ep1).Pos(120).Total(600).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120, "duration", 600)).
		Run()
}
