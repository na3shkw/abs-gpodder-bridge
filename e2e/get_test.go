package e2e

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// 図: diagrams/GET-01.svg
func TestGET_ReflectsNewerGoPodderStateToABS(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-01", "goPodder の記録の方が新しければ、ABS へ反映する").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 100, Duration: 600, LastUpdate: T(1)}).
		GoPodderRecord(Play(ep1).Pos(400).Total(600).At(T(10))).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 400, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/GET-02.svg
func TestGET_WritesNewerABSProgressBackToGoPodder(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-02", "ABS の進捗の方が新しければ、goPodder へ play として書き戻す").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300.7, Duration: 600, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCH().
		ExpectGoPodderWrite(Play(ep1).Pos(300).Total(600).At(T(10))).
		Run()
}

// 図: diagrams/GET-03.svg
func TestGET_WritesBackInOnePOST(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-03", "書き戻すエピソードが複数あれば、1 回の POST にまとめる").
		ABSEpisode(ep1, ep2).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)},
			Progress{Ep: ep2, CurrentTime: 450, Duration: 600, LastUpdate: T(11)},
		).
		GoPodderRecord(
			Play(ep1).Pos(50).Total(600).At(T(1)),
			Play(ep2).Pos(60).Total(600).At(T(2)),
		).
		ClientGET().
		ExpectGoPodderWrite(
			Play(ep1).Pos(300).Total(600).At(T(10)),
			Play(ep2).Pos(450).Total(600).At(T(11)),
		).
		Run()
}

// 図: diagrams/GET-04.svg
func TestGET_ClientReceivesWrittenBackAction(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-04", "書き戻した記録が、続けて中継する元の GET の応答に入る").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectClientReceives(Play(ep1).Pos(300).Total(600).At(T(10))).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-05.svg
func TestGET_FinishedWithoutDurationUsesEpisodeLength(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-05", "ABS で聴き終わっていて進捗に長さがなければ、エピソードの長さを total と位置にする").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 0, Duration: 0, Finished: true, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(600).Total(600).At(T(10))).
		Run()
}

// 図: diagrams/GET-06.svg
func TestGET_UnfinishedUsesCurrentTimeAndEpisodeLength(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-06", "聴き終わっていなければ、位置は currentTime の切り捨て、total は進捗 → 索引の順で取る").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 250.9, Duration: 0, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(250).Total(600).At(T(10))).
		Run()
}

// 図: diagrams/GET-07.svg
func TestGET_OmitsTotalWhenNoLengthKnown(t *testing.T) {
	ep1 := NewEpisode("ep1")
	ep1.Duration = 0
	New(t, "GET-07", "進捗にも索引にも長さがなければ、total と started を書かない").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 250.9, Duration: 0, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(100).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(250).At(T(10))).
		Run()
}

// 図: diagrams/GET-08.svg
func TestGET_CopiesGUIDFromRecord(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-08", "記録に guid があれば同じ値を、なければ含めずに書き戻す（device は含めない）").
		ABSEpisode(ep1, ep2).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)},
			Progress{Ep: ep2, CurrentTime: 300, Duration: 600, LastUpdate: T(10)},
		).
		GoPodderRecord(
			Play(ep1).Pos(50).Total(600).At(T(1)).WithGUID("guid-ep1"),
			Play(ep2).Pos(50).Total(600).At(T(1)),
		).
		ClientGET().
		ExpectGoPodderWrite(
			Play(ep1).Pos(300).Total(600).At(T(10)).WithGUID("guid-ep1"),
			Play(ep2).Pos(300).Total(600).At(T(10)),
		).
		Run()
}

// 図: diagrams/GET-09.svg
func TestGET_WritesBackForNonPlayRecords(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-09", "goPodder の記録が download・delete でも、ABS に進捗があれば書き戻す").
		ABSEpisode(ep1, ep2).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 200, Duration: 600, LastUpdate: T(5)},
			Progress{Ep: ep2, CurrentTime: 300, Duration: 600, LastUpdate: T(5)},
		).
		GoPodderRecord(
			Of(ep1, "download").At(T(1)),
			Of(ep2, "delete").At(T(9)),
		).
		ClientGET().
		ExpectGoPodderWrite(
			Play(ep1).Pos(200).Total(600).At(T(5)),
			Play(ep2).Pos(300).Total(600).At(T(5)),
		).
		Run()
}

// 図: diagrams/GET-10.svg
func TestGET_IgnoresNonPlayRecordWithoutProgress(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-10", "goPodder の記録が download で ABS に進捗がなければ、何もしない（ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep2, CurrentTime: 300, Duration: 600, LastUpdate: T(5)}).
		GoPodderRecord(Of(ep1, "download").At(T(1)), Of(ep2, "download").At(T(1))).
		ClientGET().
		ExpectNoPATCH().
		ExpectGoPodderWrite(Play(ep2).Pos(300).Total(600).At(T(5))).
		Run()
}

// 図: diagrams/GET-11.svg
func TestGET_IgnoresPlayRecordWithoutTimestamp(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-11", "時刻のない play の記録は、突き合わせに使わない（ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(5)},
			Progress{Ep: ep2, CurrentTime: 300, Duration: 600, LastUpdate: T(5)},
		).
		GoPodderRecord(Play(ep1).Pos(100).Total(600), Play(ep2).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCH().
		ExpectGoPodderWrite(Play(ep2).Pos(300).Total(600).At(T(5))).
		Run()
}

// 図: diagrams/GET-12.svg
func TestGET_ReflectsWhenABSHasNoProgress(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-12", "ABS に進捗がなく play の記録があれば、ABS へ反映する").
		ABSEpisode(ep1).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/GET-13.svg
func TestGET_ProcessesRecordsOldestFirst(t *testing.T) {
	ep1 := NewEpisode("ep1")
	newer := Play(ep1).Pos(200).Total(600).At(T(2)).URL("https://cdn2.example.com/show/ep1.mp3").WithGUID("guid-ep1")
	older := Play(ep1).Pos(100).Total(600).At(T(1))
	New(t, "GET-13", "同じエピソードに URL 違いの記録が 2 つあれば、時刻の古い順に処理し、新しい方が最後に残る").
		ABSEpisode(ep1).
		GoPodderRecord(newer, older).
		ClientGET().
		ExpectLastPATCH(ep1, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/GET-14.svg
func TestGET_RelaysWhenHistoryFetchFails(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-14", "goPodder の記録を取得できなければ、ABS には何もせず、元の GET を中継する").
		ABSEpisode(ep1).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		GoPodderFails(History, 500).
		ClientGET().
		ExpectHistoryFetched().
		ExpectNoABSRequests().
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-15.svg
func TestGET_RelaysWhenProgressFetchFails(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-15", "ABS の進捗を取得できなければ、どちらにも書き込まず、元の GET を中継する").
		ABSEpisode(ep1).
		ABSFails(ProgressGET, 500).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectABSAttempted(ProgressGET).
		ExpectNoPATCH().
		ExpectNoGoPodderWrite().
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-16.svg
func TestGET_RelaysWhenWriteBackFails(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-16", "goPodder への書き戻しが失敗しても、元の GET を中継する").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		GoPodderFails(EpisodesPOST, 500).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(300).Total(600).At(T(10))).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-17.svg
func TestGET_FetchesHistoryWithSinceZero(t *testing.T) {
	New(t, "GET-17", "記録の取得は since=0 で行い、元の GET のクエリは変えずに中継する").
		ABSEpisode(NewEpisode("ep1")).
		ClientGETSince(123).
		ExpectHistoryFetched().
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-18.svg
func TestGET_BridgeRequestsCarryClientAuth(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-18", "ブリッジが goPodder へ送るリクエストにも、クライアントの Authorization・Cookie を付ける").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectBridgeRequestsCarryClientAuth().
		Run()
}

// 図: diagrams/GET-19.svg
func TestGET_RelaysWhenHistoryFetchTimesOut(t *testing.T) {
	New(t, "GET-19", "記録の取得が GOPODDER_TIMEOUT_SECONDS 以内に返らなくても、元の GET を中継する").
		Env("GOPODDER_TIMEOUT_SECONDS", "1").
		ABSEpisode(NewEpisode("ep1")).
		GoPodderHangs(History).
		ClientGET().
		ExpectHistoryFetched().
		ExpectResponseWithin(5 * time.Second).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/GET-20.svg
func TestGET_ReflectsActionUnmatchedAtPOSTAfterReindex(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-20", "POST のときに照合できなかった記録も、索引の作り直しのあとの GET で ABS へ反映する").
		Env("ABS_INDEX_SCHEDULE", "@every 1s").
		ABSEpisode(ep1).
		ClientPOST(Play(ep2).Pos(100).Total(600).At(T(1))).
		ABSAddEpisodeNow(ep2).
		WaitIndexRebuild().
		ClientGET().
		ExpectPATCH(ep2, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/GET-21.svg
func TestGET_UsesNewestPlayAmongDuplicates(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "GET-21", "同じ組に play の記録が複数あれば、時刻が新しいもので判定する").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 200, Duration: 600, LastUpdate: T(2)}).
		GoPodderDuplicateRecords(
			Play(ep1).Pos(100).Total(600).At(T(1)),
			Play(ep1).Pos(300).Total(600).At(T(3)),
		).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 300, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/GET-22.svg
func TestGET_IgnoresPlayRecordsWithInvalidPosition(t *testing.T) {
	ep1, ep2, ep3, ep4 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3"), NewEpisode("ep4")
	New(t, "GET-22", "位置が負・数値でない（文字列の数値も）play の記録は、突き合わせに使わない（ep3 は対照）").
		ABSEpisode(ep1, ep2, ep3, ep4).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 100, Duration: 600, LastUpdate: T(1)},
			Progress{Ep: ep2, CurrentTime: 100, Duration: 600, LastUpdate: T(1)},
			Progress{Ep: ep3, CurrentTime: 100, Duration: 600, LastUpdate: T(1)},
			Progress{Ep: ep4, CurrentTime: 100, Duration: 600, LastUpdate: T(1)},
		).
		GoPodderRecord(
			Play(ep1).Pos(-5).At(T(5)),
			Play(ep2).Raw("position", "abc").At(T(5)),
			Play(ep3).Pos(400).Total(600).At(T(5)),
			Play(ep4).Raw("position", "400").Total(600).At(T(5)),
		).
		ClientGET().
		ExpectNoPATCHFor(ep1, ep2, ep4).
		ExpectPATCH(ep3, B("currentTime", 400, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/GET-23.svg
func TestGET_IgnoresEpisodesUnknownToGoPodder(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "GET-23", "goPodder に記録がないエピソードは、ABS に進捗があっても書き戻さない（ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 200, Duration: 600, LastUpdate: T(5)},
			Progress{Ep: ep2, CurrentTime: 200, Duration: 600, LastUpdate: T(5)},
		).
		GoPodderRecord(Play(ep2).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep2).Pos(200).Total(600).At(T(5))).
		Run()
}
