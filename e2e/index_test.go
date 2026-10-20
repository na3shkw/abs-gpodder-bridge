package e2e

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// 図: diagrams/IDX-01.svg
func TestIndex_BuiltAtStartup(t *testing.T) {
	New(t, "IDX-01", "起動したら、batch/get で番組の詳細を取って索引を作る").
		ABSEpisode(NewEpisode("ep1"), NewEpisode("ep2")).
		ExpectIndexBuiltAtStartup().
		Run()
}

// 図: diagrams/IDX-02.svg
func TestIndex_UnmatchedBeforeBuilt(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "IDX-02", "索引を作り終える前に届いた記録は照合できないが、作り終えたあとの記録は反映する").
		ABSEpisode(ep1).
		ABSDelays(BatchGet, 3*time.Second).
		NoIndexWait().
		ClientPOST(Play(ep1).Pos(120).At(T(1))).
		WaitIndexBuilt().
		ClientPOST(Play(ep1).Pos(240).At(T(2))).
		ExpectPATCH(ep1, B("currentTime", 240)).
		Run()
}

// 図: diagrams/IDX-03.svg
func TestIndex_RebuiltOnSchedule(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "IDX-03", "ABS_INDEX_SCHEDULE の時刻に作り直し、追加されたエピソードを照合できるようになる").
		Env("ABS_INDEX_SCHEDULE", "@every 1s").
		ABSEpisode(ep1).
		ABSAddEpisodeNow(ep2).
		WaitIndexRebuild().
		ClientPOST(Play(ep2).Pos(120).At(T(1))).
		ExpectPATCH(ep2, B("currentTime", 120)).
		Run()
}

// 図: diagrams/IDX-04.svg
func TestIndex_NotRebuiltOnUnmatched(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "IDX-04", "照合できない記録が来ても、索引を作り直さない（ep1 は対照）").
		ABSEpisode(ep1).
		ClientPOST(Play(NewEpisode("ep9")).Pos(120).At(T(1)), Play(ep1).Pos(60).At(T(1))).
		Wait(2*time.Second).
		ExpectPATCH(ep1, B("currentTime", 60)).
		ExpectNoIndexRebuild().
		Run()
}

// 図: diagrams/IDX-05.svg
func TestIndex_KeepsPreviousOnFailure(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "IDX-05", "索引の作り直しに失敗したら、前の索引を使い続ける").
		Env("ABS_INDEX_SCHEDULE", "@every 1s").
		ABSEpisode(ep1).
		ABSFailsAfterStartup(Libraries, 500).
		WaitIndexAttempt().
		ClientPOST(Play(ep1).Pos(120).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120)).
		Run()
}

// 図: diagrams/IDX-06.svg
func TestIndex_WorksWithoutFeedsWhenFeedsFail(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	ep2 := NewEpisode("ep2")
	New(t, "IDX-06", "/api/feeds だけが失敗したら、フィードの対応づけなしの索引で照合する").
		AdminToken().
		ABSEpisode(f1, ep2).
		ABSFeed(f1).
		ABSFails(Feeds, 500).
		ClientPOST(
			Play(f1).Pos(100).Total(600).At(T(1)),
			Play(ep2).Pos(200).Total(600).At(T(1)),
		).
		ExpectNoPATCHFor(f1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/IDX-07.svg
func TestIndex_SkipsNonPodcastLibraries(t *testing.T) {
	New(t, "IDX-07", "ポッドキャスト以外（本）のライブラリの番組一覧は取らない").
		ABSEpisode(NewEpisode("ep1")).
		ABSBookLibrary("lib-books", "book-1").
		ExpectIndexBuiltAtStartup().
		ExpectABSNeverRequested("/api/libraries/lib-books/items").
		Run()
}

// 図: diagrams/IDX-08.svg
func TestIndex_ListsItemsWithoutLimit(t *testing.T) {
	New(t, "IDX-08", "番組一覧は limit を付けずに（全件）取る").
		ABSEpisode(NewEpisode("ep1")).
		ExpectNoLimitOnItems().
		Run()
}

// 図: diagrams/IDX-09.svg
func TestIndex_AdminTokenOnlyForFeeds(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	New(t, "IDX-09", "管理者のトークンは /api/feeds だけに使う").
		AdminToken().
		ABSEpisode(f1).
		ABSFeed(f1).
		ClientPOST(Play(f1).Pos(100).Total(600).At(T(1))).
		ExpectTokenUsage().
		Run()
}

// 図: diagrams/IDX-10.svg
func TestIndex_RemovedEpisodeNoLongerMatches(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "IDX-10", "毎回全体を作り直すので、ABS から消えたエピソードは照合されなくなる").
		Env("ABS_INDEX_SCHEDULE", "@every 1s").
		ABSEpisode(ep1, ep2).
		ABSRemoveEpisodeNow(ep1).
		WaitIndexRebuild().
		ClientPOST(Play(ep1).Pos(120).At(T(1))).
		ExpectNoPATCH().
		Run()
}

// 図: diagrams/IDX-11.svg
func TestIndex_DoesNotPollProgress(t *testing.T) {
	New(t, "IDX-11", "何もしていない間は、ABS の進捗を取りに行かない").
		Env("ABS_INDEX_SCHEDULE", "@every 1s").
		ABSEpisode(NewEpisode("ep1")).
		Wait(3 * time.Second).
		ExpectABSAttempted(BatchGet).
		ExpectNoProgressFetch().
		Run()
}
