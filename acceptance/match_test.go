package acceptance

import (
	"testing"

	. "github.com/na3shkw/abs-gpodder-bridge/acceptance/internal/scenario"
)

// 図: diagrams/MCH-01.svg
func TestMatch_ByABSFeedURL(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	New(t, "MCH-01", "ABS のフィード URL の記録は、/api/feeds の対応づけでエピソードを見つける").
		AdminToken().
		ABSEpisode(f1).
		ABSFeed(f1).
		ClientPOST(Play(f1).Pos(100).Total(600).At(T(1))).
		ExpectPATCH(f1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/MCH-02.svg
func TestMatch_FeedURLIgnoresSchemeHostPort(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	New(t, "MCH-02", "フィード URL は、スキーム・ホスト・ポートが違っても同じエピソードとみなす").
		AdminToken().
		ABSEpisode(f1).
		ABSFeed(f1).
		ClientPOST(Play(f1).Pos(100).Total(600).At(T(1)).URL("http://192.168.1.10:13378/feed/show/item/fe-ep1/media.mp3")).
		ExpectPATCH(f1, B("currentTime", 100, "duration", 600)).
		Run()
}

// 図: diagrams/MCH-03.svg
func TestMatch_SkipsFeedStepWithoutAdminToken(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	ep2 := NewEpisode("ep2")
	New(t, "MCH-03", "ABS_ADMIN_TOKEN がなければ /api/feeds を呼ばず、元の配信 URL と GUID で照合する").
		ABSEpisode(f1, ep2).
		ABSFeed(f1).
		ClientPOST(
			Play(f1).Pos(100).Total(600).At(T(1)),
			Play(ep2).Pos(200).Total(600).At(T(1)),
		).
		ExpectABSNeverRequested("/api/feeds").
		ExpectNoPATCHFor(f1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/MCH-04.svg
func TestMatch_SkipsFeedStepWhenFeedsForbidden(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	ep2 := NewEpisode("ep2")
	New(t, "MCH-04", "/api/feeds が 403 なら、元の配信 URL と GUID で照合する").
		AdminTokenRejected().
		ABSEpisode(f1, ep2).
		ABSFeed(f1).
		ClientPOST(
			Play(f1).Pos(100).Total(600).At(T(1)),
			Play(ep2).Pos(200).Total(600).At(T(1)),
		).
		ExpectNoPATCHFor(f1).
		ExpectPATCH(ep2, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/MCH-05.svg
func TestMatch_FeedMappingLimitedToUserIndex(t *testing.T) {
	secret := NewEpisode("ep9").ViaABSFeed(ABSHost, "secret")
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	New(t, "MCH-05", "フィードのエピソードでも、ユーザーの索引にないものとは対応づけない（ep1 は対照）").
		AdminToken().
		ABSHiddenEpisode(secret).
		ABSEpisode(f1).
		ABSFeed(secret, f1).
		ClientPOST(
			Play(secret).Pos(100).Total(600).At(T(1)),
			Play(f1).Pos(200).Total(600).At(T(1)),
		).
		ExpectNoPATCHFor(secret).
		ExpectPATCH(f1, B("currentTime", 200, "duration", 600)).
		Run()
}

// 図: diagrams/MCH-06.svg
func TestMatch_ByOriginalEnclosureURL(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "MCH-06", "元の配信 URL が一致すれば、そのエピソードとみなす").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).At(T(1))).
		ExpectPATCH(ep1, B("currentTime", 120)).
		Run()
}

// 図: diagrams/MCH-07.svg
func TestMatch_NormalizesURL(t *testing.T) {
	ep1, ep2, ep3, ep4 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3"), NewEpisode("ep4")
	ep4.EnclosureURL = "http://cdn.example.com/show/ep4.mp3"
	New(t, "MCH-07", "スキーム・ホストの大小文字、既定のポート、#以降の違いは無視して比べる").
		ABSEpisode(ep1, ep2, ep3, ep4).
		ClientPOST(
			Play(ep1).Pos(10).At(T(1)).URL("HTTPS://CDN.EXAMPLE.COM/show/ep1.mp3"),
			Play(ep2).Pos(20).At(T(1)).URL("https://cdn.example.com:443/show/ep2.mp3"),
			Play(ep3).Pos(30).At(T(1)).URL("https://cdn.example.com/show/ep3.mp3#t=30"),
			Play(ep4).Pos(40).At(T(1)).URL("http://cdn.example.com:80/show/ep4.mp3"),
		).
		ExpectPATCH(ep1, B("currentTime", 10)).
		ExpectPATCH(ep2, B("currentTime", 20)).
		ExpectPATCH(ep3, B("currentTime", 30)).
		ExpectPATCH(ep4, B("currentTime", 40)).
		Run()
}

// 図: diagrams/MCH-08.svg
func TestMatch_PathAndQueryAreCaseSensitive(t *testing.T) {
	ep1, ep2, ep3 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3")
	ep1.EnclosureURL += "?token=abc"
	ep2.EnclosureURL += "?token=abc"
	ep3.EnclosureURL += "?token=abc"
	New(t, "MCH-08", "パスとクエリは大小文字も含めて比べる（違えば一致しない、ep3 は対照）").
		ABSEpisode(ep1, ep2, ep3).
		ClientPOST(
			Play(ep1).Pos(10).At(T(1)).URL("https://cdn.example.com/Show/EP1.mp3?token=abc"),
			Play(ep2).Pos(20).At(T(1)).URL("https://cdn.example.com/show/ep2.mp3?token=ABC"),
			Play(ep3).Pos(30).At(T(1)).URL("https://cdn.example.com/show/ep3.mp3?token=abc"),
		).
		ExpectNoPATCHFor(ep1, ep2).
		ExpectPATCH(ep3, B("currentTime", 30)).
		Run()
}

// 図: diagrams/MCH-09.svg
func TestMatch_ByGUID(t *testing.T) {
	ep1 := NewEpisode("ep1")
	New(t, "MCH-09", "URL が違っても GUID が一致すれば、そのエピソードとみなす").
		ABSEpisode(ep1).
		ClientPOST(Play(ep1).Pos(120).At(T(1)).URL("https://other.example.com/ep1.mp3").WithGUID("guid-ep1")).
		ExpectPATCH(ep1, B("currentTime", 120)).
		Run()
}

// 図: diagrams/MCH-10.svg
func TestMatch_NullNeverMatches(t *testing.T) {
	ep1, ep2 := NewEpisode("ep1"), NewEpisode("ep2")
	ep1.EnclosureURL, ep1.GUID = "", ""
	New(t, "MCH-10", "ABS 側の元の配信 URL・GUID が null なら、その手順では一致しない（ep2 は対照）").
		ABSEpisode(ep1, ep2).
		ClientPOST(
			Play(ep1).Pos(10).At(T(1)).URL("https://cdn.example.com/show/ep1.mp3"),
			Play(ep1).Pos(20).At(T(2)).URL("https://cdn.example.com/show/x.mp3").Raw("guid", ""),
			Play(ep2).Pos(30).At(T(1)),
		).
		ExpectNoPATCHFor(ep1).
		ExpectPATCH(ep2, B("currentTime", 30)).
		Run()
}

// 図: diagrams/MCH-11.svg
func TestMatch_AmbiguousURLIsNotMatched(t *testing.T) {
	ep1, ep2, ep3 := NewEpisode("ep1"), NewEpisode("ep2"), NewEpisode("ep3")
	ep2.LibraryID = "lib-archive"
	ep2.EnclosureURL = ep1.EnclosureURL
	New(t, "MCH-11", "同じ URL に ABS のエピソードが 2 つあれば照合できない（GUID でも探さない、ep3 は対照）").
		ABSEpisode(ep1, ep2, ep3).
		ClientPOST(
			Play(ep1).Pos(120).Total(600).At(T(1)).WithGUID("guid-ep1"),
			Play(ep3).Pos(240).Total(600).At(T(1)),
		).
		ExpectNoPATCHFor(ep1, ep2).
		ExpectPATCH(ep3, B("currentTime", 240, "duration", 600)).
		ExpectResponseFromGoPodder().
		Run()
}

// 図: diagrams/MCH-12.svg
func TestMatch_FeedURLStepComesFirst(t *testing.T) {
	f1 := NewEpisode("ep1").ViaABSFeed(ABSHost, "show")
	ep2 := NewEpisode("ep2")
	ep2.EnclosureURL = f1.ClientURL
	New(t, "MCH-12", "フィード URL と元の配信 URL が別のエピソードを指すなら、フィード URL の方を採る").
		AdminToken().
		ABSEpisode(f1, ep2).
		ABSFeed(f1).
		ClientPOST(Play(f1).Pos(100).Total(600).At(T(1))).
		ExpectPATCH(f1, B("currentTime", 100, "duration", 600)).
		ExpectNoPATCHFor(ep2).
		Run()
}
