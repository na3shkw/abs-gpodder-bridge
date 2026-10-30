package scenario

import (
	"fmt"
	"strings"
)

// グループの表示名（ID の接頭辞 → 見出し）。並びもこの順。
var groups = []struct{ prefix, title string }{
	{"CFG", "起動・設定"},
	{"HLT", "ヘルスチェック"},
	{"PRX", "中継"},
	{"PST", "POST（記録の受信）"},
	{"GET", "GET（同期の突き合わせ）"},
	{"MCH", "照合"},
	{"IDX", "索引"},
	{"CNF", "比べ方"},
	{"FMT", "データ形式"},
}

// indexHTML は図の一覧ページを作る。テストへのリンクはこのページからの相対パス。
func indexHTML(entries []*entry) string {
	var b strings.Builder
	b.WriteString(indexHead)
	fmt.Fprintf(&b, "<p class=\"lead\">テスト 1 件につき 1 枚の図。全 %d 件。図はテストの記述（DSL）から生成している。</p>\n", len(entries))
	b.WriteString("<div class=\"legend\"><span><b class=\"ok\">✓</b> 起きるべきこと</span><span><b class=\"ng\">✗</b> 起きてはならないこと</span><span>時刻は 09:00 起点の例、位置・長さは秒</span></div>\n")
	// 左のメニュー（グループの見出しへのリンク）
	b.WriteString("<nav class=\"side\" aria-label=\"グループ\"><ul>\n")
	for _, g := range groups {
		for _, e := range entries {
			if strings.HasPrefix(e.id, g.prefix+"-") {
				fmt.Fprintf(&b, "<li><a href=\"#g-%s\">%s</a></li>\n", esc(g.prefix), esc(g.title))
				break
			}
		}
	}
	b.WriteString("</ul></nav>\n")
	used := map[string]bool{}
	for _, g := range groups {
		var es []*entry
		for _, e := range entries {
			if strings.HasPrefix(e.id, g.prefix+"-") {
				es = append(es, e)
				used[e.id] = true
			}
		}
		if len(es) > 0 {
			writeGroup(&b, g.prefix, g.title, es)
		}
	}
	var rest []*entry
	for _, e := range entries {
		if !used[e.id] {
			rest = append(rest, e)
		}
	}
	if len(rest) > 0 {
		writeGroup(&b, "other", "その他", rest)
	}
	b.WriteString(indexTail)
	return b.String()
}

func writeGroup(b *strings.Builder, key, title string, es []*entry) {
	fmt.Fprintf(b, "<details class=\"sec\" id=\"g-%s\" data-key=\"%s\" open><summary><h2>%s</h2><span class=\"count\"></span></summary>\n", esc(key), esc(key), esc(title))
	for _, e := range es {
		fmt.Fprintf(b, `<article class="card" id="%s">
<header><label><input type="checkbox" data-id="%s"> <span class="id">%s</span></label><span class="title">%s</span></header>
<p class="src"><a href="../%s#L%d">%s:%d</a> <code>%s</code></p>
<img src="%s.svg" alt="%s の図" loading="lazy">
</article>
`, esc(e.id), esc(e.id), esc(e.id), esc(e.title), esc(e.file), e.line, esc(e.file), e.line, esc(e.testName), esc(e.id), esc(e.id))
	}
	b.WriteString("</details>\n")
}

const indexHead = `<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>e2e テストシナリオ図解</title>
<style>
:root { --bg:#fbfaf7; --fg:#22252a; --muted:#5f6670; --line:#d9d6cf; --card:#ffffff; --accent:#2f6fbd; --ok:#3f8a52; --ng:#c0606e; --code-bg:#f1efe9; }
@media (prefers-color-scheme: dark) {
  :root:not([data-theme="light"]) { --bg:#17191c; --fg:#e6e4df; --muted:#a2a7ae; --line:#3a3e44; --card:#1f2226; --accent:#7fb0ee; --ok:#78b884; --ng:#e08a96; --code-bg:#2a2d32; }
}
:root[data-theme="dark"] { --bg:#17191c; --fg:#e6e4df; --muted:#a2a7ae; --line:#3a3e44; --card:#1f2226; --accent:#7fb0ee; --ok:#78b884; --ng:#e08a96; --code-bg:#2a2d32; }
* { box-sizing: border-box; }
body { margin: 0; background: var(--bg); color: var(--fg); font-family: "Hiragino Sans","Noto Sans JP","Yu Gothic UI",system-ui,sans-serif; line-height: 1.6; font-size: 15px; }
main { max-width: 980px; margin: 0 auto; padding: 0 16px 80px; }
h1 { font-size: 1.5rem; margin: 24px 0 4px; }
.lead, .legend { color: var(--muted); font-size: 0.9rem; }
.legend { display: flex; flex-wrap: wrap; gap: 4px 16px; }
.legend .ok { color: var(--ok); } .legend .ng { color: var(--ng); }
code { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 0.85em; background: var(--code-bg); padding: 1px 5px; border-radius: 4px; }
a { color: var(--accent); }
.bar { position: sticky; top: 0; z-index: 5; background: var(--bg); display: flex; flex-wrap: wrap; gap: 8px 14px; align-items: center; padding: 8px 0; border-bottom: 1px solid var(--line); font-size: 0.88rem; }
.bar button { font: inherit; font-size: 0.82rem; color: var(--fg); background: var(--card); border: 1px solid var(--line); border-radius: 6px; padding: 2px 10px; cursor: pointer; }
.bar .total { font-weight: 700; font-variant-numeric: tabular-nums; }
details.sec { margin-top: 24px; }
details.sec > summary { cursor: pointer; list-style: none; display: flex; align-items: baseline; gap: 10px; border-bottom: 1px solid var(--line); padding-bottom: 4px; }
details.sec > summary::-webkit-details-marker { display: none; }
details.sec > summary::before { content: "▸"; color: var(--muted); }
details.sec[open] > summary::before { content: "▾"; }
details.sec h2 { font-size: 1.15rem; margin: 0; }
.count { font-size: 0.82rem; color: var(--muted); font-variant-numeric: tabular-nums; }
.count.done { color: var(--ok); font-weight: 700; }
.card { background: var(--card); border: 1px solid var(--line); border-radius: 10px; padding: 12px 14px; margin: 14px 0; }
.card.checked { border-color: var(--ok); }
.card header { display: flex; flex-wrap: wrap; align-items: baseline; gap: 4px 12px; }
.card label { cursor: pointer; display: inline-flex; align-items: center; gap: 6px; }
.card .id { font-family: ui-monospace, Menlo, Consolas, monospace; color: var(--accent); font-size: 0.9rem; }
.card.checked .id { color: var(--ok); }
.card .title { font-weight: 700; }
.card .src { margin: 2px 0 8px; font-size: 0.82rem; color: var(--muted); }
.card img { width: 100%; height: auto; display: block; border-radius: 6px; }
input[type=checkbox] { width: 16px; height: 16px; accent-color: var(--ok); cursor: pointer; }
nav.side { position: fixed; top: 50%; transform: translateY(-50%); left: max(12px, calc(50vw - 490px - 230px)); width: max-content; z-index: 6;
  padding: 8px 0; font-size: 0.88rem; white-space: nowrap; }
nav.side ul { list-style: none; margin: 0; padding: 0; }
nav.side a { display: block; padding: 5px 14px; color: var(--fg); text-decoration: none; border-left: 3px solid transparent; }
nav.side a:hover { background: var(--code-bg); }
nav.side a.current { border-left-color: var(--accent); color: var(--accent); font-weight: 700; }
details.sec { scroll-margin-top: 56px; }
@media (max-width: 1460px) { main { margin-left: 240px; } }
@media (max-width: 760px) { nav.side { display: none; } main { margin-left: auto; } }
</style>
</head>
<body>
<main>
<h1>e2e テストシナリオ図解</h1>
<div class="bar"><span>チェック <span class="total"></span></span><button type="button" data-act="open">すべて開く</button><button type="button" data-act="close">すべて閉じる</button><button type="button" data-act="reset">チェックをすべて外す</button></div>
`

const indexTail = `</main>
<script>
(function () {
  var P = 'e2e-diagrams:';
  function load(k, d) { try { var v = localStorage.getItem(P + k); return v === null ? d : JSON.parse(v); } catch (e) { return d; } }
  function save(k, v) { try { localStorage.setItem(P + k, JSON.stringify(v)); } catch (e) {} }
  var checked = load('checked', {}), open = load('open', {});
  var boxes = Array.prototype.slice.call(document.querySelectorAll('input[data-id]'));
  var secs = Array.prototype.slice.call(document.querySelectorAll('details.sec'));
  function paint(cb) { cb.closest('.card').classList.toggle('checked', cb.checked); }
  function refresh() {
    var done = boxes.filter(function (b) { return b.checked; }).length;
    document.querySelector('.bar .total').textContent = done + ' / ' + boxes.length;
    secs.forEach(function (s) {
      var bs = boxes.filter(function (b) { return s.contains(b); });
      var d = bs.filter(function (b) { return b.checked; }).length;
      var c = s.querySelector('.count');
      c.textContent = d + ' / ' + bs.length;
      c.classList.toggle('done', d === bs.length);
    });
  }
  boxes.forEach(function (cb) {
    cb.checked = !!checked[cb.dataset.id];
    paint(cb);
    cb.addEventListener('change', function () {
      if (cb.checked) checked[cb.dataset.id] = true; else delete checked[cb.dataset.id];
      save('checked', checked); paint(cb); refresh();
    });
  });
  secs.forEach(function (s) {
    var k = s.dataset.key;
    if (open[k] !== undefined) s.open = open[k];
    s.addEventListener('toggle', function () { open[k] = s.open; save('open', open); });
  });
  document.querySelector('.bar').addEventListener('click', function (e) {
    var a = e.target.dataset && e.target.dataset.act;
    if (a === 'open' || a === 'close') secs.forEach(function (s) { s.open = a === 'open'; });
    if (a === 'reset' && confirm('チェックをすべて外しますか？')) {
      checked = {}; save('checked', checked);
      boxes.forEach(function (b) { b.checked = false; paint(b); });
    }
    refresh();
  });
  if (location.hash) { var t = document.getElementById(location.hash.slice(1)); if (t) t.closest('details').open = true; }
  // 左のメニュー：閉じている見出しは開いてから飛ぶ。表示中のグループを強調する
  var links = Array.prototype.slice.call(document.querySelectorAll('nav.side a'));
  links.forEach(function (a) {
    a.addEventListener('click', function () {
      var t = document.getElementById(a.getAttribute('href').slice(1));
      if (t) { t.open = true; open[t.dataset.key] = true; save('open', open); }
    });
  });
  function markCurrent() {
    var cur = null;
    secs.forEach(function (s) { if (s.getBoundingClientRect().top < window.innerHeight / 3) cur = s; });
    links.forEach(function (a) { a.classList.toggle('current', !!cur && a.getAttribute('href') === '#' + cur.id); });
  }
  window.addEventListener('scroll', markCurrent, { passive: true });
  markCurrent();
  refresh();
})();
</script>
</body>
</html>
`
