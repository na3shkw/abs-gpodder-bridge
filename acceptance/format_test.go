package acceptance

import (
	"testing"
	"time"

	. "github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/scenario"
)

// 図: diagrams/FMT-01.svg
func TestFormat_AcceptsTimestampVariants(t *testing.T) {
	ep1, ep2, ep3 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3")
	New(t, "FMT-01", "時刻の Z・小数秒・時差を正しく読む（ABS の 09:06 と比べる）").
		ABSEpisode(ep1, ep2, ep3).
		ABSProgress(
			Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(6)},
			Progress{Ep: ep2, CurrentTime: 300, Duration: 600, LastUpdate: T(6)},
			Progress{Ep: ep3, CurrentTime: 300, Duration: 600, LastUpdate: T(6)},
		).
		ClientPOST(
			Play(ep1).Pos(100).Total(600).Raw("timestamp", "2026-01-01T09:07:00Z"),
			Play(ep2).Pos(100).Total(600).Raw("timestamp", "2026-01-01T09:05:59.900"),
			Play(ep3).Pos(100).Total(600).Raw("timestamp", "2026-01-01T18:05:00+09:00"),
		).
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		ExpectNoPATCHFor(ep2, ep3).
		Run()
}

// 図: diagrams/FMT-02.svg
func TestFormat_WritesTimestampWithoutZone(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "FMT-02", "書き戻す時刻は UTC の YYYY-MM-DDTHH:MM:SS（ミリ秒は切り捨て、Z なし）").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10).Add(123 * time.Millisecond)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(300).Total(600).At(T(10))).
		Run()
}

// 図: diagrams/FMT-04.svg
func TestFormat_ReadsTimestampAsUTCRegardlessOfTZ(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "FMT-04", "TZ を設定しても、受け取る時刻は UTC として読む（ABS の 09:06 と比べる）").
		Env("TZ", "Asia/Tokyo").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(6)}).
		ClientPOST(Play(ep1).Pos(100).Total(600).At(T(7))).
		ExpectPATCH(ep1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/FMT-05.svg
func TestFormat_WritesTimestampInUTCRegardlessOfTZ(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "FMT-05", "TZ を設定しても、書き戻す時刻は UTC").
		Env("TZ", "Asia/Tokyo").
		ABSEpisode(ep1).
		ABSProgress(Progress{Ep: ep1, CurrentTime: 300, Duration: 600, LastUpdate: T(10)}).
		GoPodderRecord(Play(ep1).Pos(50).Total(600).At(T(1))).
		ClientGET().
		ExpectGoPodderWrite(Play(ep1).Pos(300).Total(600).At(T(10))).
		Run()
}

// 図: diagrams/FMT-03.svg
func TestFormat_AcceptsDecimalNumbers(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "FMT-03", "位置・長さが小数でも受け付ける").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120.5).Total(600.25).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120.5, "duration", 600.25)).
		Run()
}
