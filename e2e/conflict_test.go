package e2e

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/e2e/internal/scenario"
)

// 比べ方（判定表）の各行を、POST（記録の受信）と GET（同期の突き合わせ）の両方で確かめる。

// 図: diagrams/CNF-01-POST.svg
func TestConflictPOST_NoProgressReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-01-POST", "ABS に進捗がなければ反映する（POST）").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-01-GET.svg
func TestConflictGET_NoProgressReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-01-GET", "ABS に進捗がなければ反映する（GET）").
		ABSEpisode(ep1).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-02-POST.svg
func TestConflictPOST_NewerGoPodderReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-02-POST", "記録の方が新しく位置が違えば、ABS へ反映する（POST）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 50, Duration: 600, LastUpdate: T(1)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(2))).
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-02-GET.svg
func TestConflictGET_NewerGoPodderReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-02-GET", "記録の方が新しく位置が違えば、ABS へ反映する（GET）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 50, Duration: 600, LastUpdate: T(1)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(2))).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-03-POST.svg
func TestConflictPOST_NewerABSDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-03-POST", "ABS の方が新しく位置が違っても、POST では何もしない（ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(5)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(1)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-03-GET.svg
func TestConflictGET_NewerABSWritesBack(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-03-GET", "ABS の方が新しく位置が違えば、goPodder へ書き戻す（GET）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(5)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCH().
		ExpectGoPodderWrite(Play(ep1).Pos(300).Total(600).At(T(5))).
		Run()
}

// 図: diagrams/CNF-04-POST.svg
func TestConflictPOST_BothFinishedDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-04-POST", "どちらも聴き終わっていれば、時刻が違っても何もしない（POST、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 600, Duration: 600, Finished: true, LastUpdate: T(1)}).
		ClientPOST(Play(ep1).Pos(600).Total(600).At(T(5)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-04-GET.svg
func TestConflictGET_BothFinishedDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-04-GET", "どちらも聴き終わっていれば、時刻が違っても何もしない（GET、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 600, Duration: 600, Finished: true, LastUpdate: T(5)}).
		GoPodderRecord(Play(ep1).Pos(600).Total(600).At(T(1)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-05-POST.svg
func TestConflictPOST_WithinOneSecondDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-05-POST", "どちらも聴き終わっておらず位置の差が 1 秒以内なら、何もしない（POST、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 100.8, Duration: 600, LastUpdate: T(1)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(5)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-05-GET.svg
func TestConflictGET_WithinOneSecondDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-05-GET", "どちらも聴き終わっておらず位置の差が 1 秒以内なら、何もしない（GET、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 100.8, Duration: 600, LastUpdate: T(1)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(5)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-06-POST.svg
func TestConflictPOST_FinishedDiffersReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-06-POST", "片方だけ聴き終わっていれば、位置が同じでも違う状態とみなす（POST）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 600, Duration: 600, LastUpdate: T(1)}).
		ClientPOST(Play(ep1).Pos(600).Total(600).At(T(5))).
		ExpectPATCH(ep1, B("currentTime", 600, "duration", 600, "isFinished", true)).
		Run()
}

// 図: diagrams/CNF-06-GET.svg
func TestConflictGET_FinishedDiffersReflects(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "CNF-06-GET", "片方だけ聴き終わっていれば、位置が同じでも違う状態とみなす（GET）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 600, Duration: 600, LastUpdate: T(1)}).
		GoPodderRecord(Play(ep1).Pos(600).Total(600).At(T(5))).
		ClientGET().
		ExpectPATCH(ep1, B("currentTime", 600, "duration", 600, "isFinished", true)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-07-POST.svg
func TestConflictPOST_EqualTimeDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-07-POST", "時刻が等しければ、位置が違っても何もしない（POST、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(3)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(3)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-07-GET.svg
func TestConflictGET_EqualTimeDoesNothing(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-07-GET", "時刻が等しければ、位置が違っても何もしない（GET、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(3)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(3)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}

// 図: diagrams/CNF-08-POST.svg
func TestConflictPOST_ComparesInSeconds(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-08-POST", "時刻は秒で比べる（ABS が 0.9 秒だけ新しくても等しい）（POST、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(3).Add(900 * time.Millisecond)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(3)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/CNF-08-GET.svg
func TestConflictGET_ComparesInSeconds(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	New(t, "CNF-08-GET", "時刻は秒で比べる（ABS が 0.9 秒だけ新しくても等しい）（GET、ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(3).Add(900 * time.Millisecond)}).
		GoPodderRecord(Play(ep1).Pos(100).Total(600).At(T(3)), Play(ep2).Pos(200).Total(600).At(T(1))).
		ClientGET().
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		ExpectNoGoPodderWrite().
		Run()
}
