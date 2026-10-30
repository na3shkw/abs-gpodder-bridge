package scenario

import (
	"fmt"
	"html"
	"strings"
)

// Lane は図の縦の列（登場するもの）。
type Lane int

const (
	Client Lane = iota
	Bridge
	GoPodder
	ABS
)

var laneNames = [...]string{"クライアント", "ブリッジ", "goPodder", "ABS"}
var laneClass = [...]string{"client", "bridge", "gp", "abs"}

// 図の要素。テストの情報（名前・ID・ファイル）は含めない。
type note struct {
	lane  Lane
	lines []string
}

type message struct {
	from, to Lane
	label    string
	detail   []string
	expect   bool // Assert なら true
	negated  bool // 「送られない」
}

type check struct {
	lane    Lane
	text    string
	negated bool
}

// step は Act の中の、矢印にならない手順（待つ・ABS を変える など）。
type step struct {
	lane Lane
	text string
}

// stepBox は Act の中の、1 つのレーンだけで行う複数行の動作（設定を変えた起動など）。
type stepBox struct {
	lane  Lane
	lines []string
}

type diagram struct {
	given []note
	when  []any // message または step（Act）
	then  []any // message または check（Assert）
}

func (d *diagram) addGiven(l Lane, line string) {
	for i := range d.given {
		if d.given[i].lane == l {
			d.given[i].lines = append(d.given[i].lines, line)
			return
		}
	}
	d.given = append(d.given, note{lane: l, lines: []string{line}})
}

const (
	svgW     = 900
	headerH  = 40
	lineH    = 17
	bandGap  = 16
	noteW    = 210
	arrowGap = 30
)

var laneX = [...]int{100, 330, 560, 790}

// SVG は図を SVG 文書として返す。同じ入力からは同じ出力になる（差分の確認に使うため）。
func (d *diagram) SVG() string {
	var body strings.Builder
	y := 20 + headerH + bandGap

	band := func(title string) {
		fmt.Fprintf(&body, `<line class="sep" x1="8" y1="%d" x2="%d" y2="%d"/>`+"\n", y, svgW-8, y)
		fmt.Fprintf(&body, `<text class="band" x="10" y="%d">%s</text>`+"\n", y+14, title)
		y += 24
	}

	if len(d.given) > 0 {
		band("Arrange")
		// 箱の位置と幅を決め、横に重なる箱は下の段にずらす
		type placed struct {
			n         note
			x, w, row int
		}
		var ps []placed
		for _, n := range d.given {
			w := noteW
			for _, l := range n.lines {
				if tw := textWidth(l) + 20; tw > w {
					w = tw
				}
			}
			x := max(8, min(laneX[n.lane]-w/2, svgW-8-w))
			row := 0
			for {
				ok := true
				for _, p := range ps {
					if p.row == row && x < p.x+p.w+8 && p.x < x+w+8 {
						ok = false
					}
				}
				if ok {
					break
				}
				row++
			}
			ps = append(ps, placed{n: n, x: x, w: w, row: row})
		}
		rows := 0
		for _, p := range ps {
			rows = max(rows, p.row+1)
		}
		for r := 0; r < rows; r++ {
			maxLines := 0
			for _, p := range ps {
				if p.row == r {
					maxLines = max(maxLines, len(p.n.lines))
				}
			}
			h := maxLines*lineH + 14
			for _, p := range ps {
				if p.row != r {
					continue
				}
				fmt.Fprintf(&body, `<rect class="note %s" x="%d" y="%d" width="%d" height="%d" rx="6"/>`+"\n", laneClass[p.n.lane], p.x, y, p.w, h)
				for i, l := range p.n.lines {
					fmt.Fprintf(&body, `<text class="mono" x="%d" y="%d">%s</text>`+"\n", p.x+10, y+20+i*lineH, esc(l))
				}
			}
			y += h + 8
		}
		y += bandGap - 8
	}

	drawMessage := func(m message) {
		x1, x2 := laneX[m.from], laneX[m.to]
		y += 18
		mid := (x1 + x2) / 2
		cls := "arrow"
		label := m.label
		if m.expect {
			cls = "arrow expect"
			label = "✓ " + label
		}
		if m.negated {
			cls = "arrow negated"
			label = "✗ " + label
		}
		fmt.Fprintf(&body, `<text class="label" x="%d" y="%d" text-anchor="middle">%s</text>`+"\n", mid, y-6, esc(label))
		fmt.Fprintf(&body, `<line class="%s" x1="%d" y1="%d" x2="%d" y2="%d" marker-end="url(#%s)"/>`+"\n", cls, x1, y, x2, y, markerFor(m))
		for i, l := range m.detail {
			fmt.Fprintf(&body, `<text class="mono detail" x="%d" y="%d" text-anchor="middle">%s</text>`+"\n", mid, y+16+i*lineH, esc(l))
		}
		y += len(m.detail)*lineH + arrowGap - 10
	}

	drawPill := func(lane Lane, text, cls string) {
		w := noteW + 20
		if tw := textWidth(text) + 24; tw > w {
			w = tw
		}
		x := max(8, min(laneX[lane]-w/2, svgW-8-w))
		fmt.Fprintf(&body, `<rect class="%s" x="%d" y="%d" width="%d" height="26" rx="13"/>`+"\n", cls, x, y+4, w)
		fmt.Fprintf(&body, `<text class="checktext" x="%d" y="%d" text-anchor="middle">%s</text>`+"\n", x+w/2, y+22, esc(text))
		y += 38
	}

	if len(d.when) > 0 {
		band("Act")
		for _, it := range d.when {
			switch v := it.(type) {
			case message:
				drawMessage(v)
			case step:
				drawPill(v.lane, v.text, "step")
			case stepBox:
				w := noteW
				for _, l := range v.lines {
					w = max(w, textWidth(l)+20)
				}
				x := max(8, min(laneX[v.lane]-w/2, svgW-8-w))
				h := len(v.lines)*lineH + 14
				fmt.Fprintf(&body, `<rect class="note %s" x="%d" y="%d" width="%d" height="%d" rx="6"/>`+"\n", laneClass[v.lane], x, y+4, w, h)
				for i, l := range v.lines {
					fmt.Fprintf(&body, `<text class="mono" x="%d" y="%d">%s</text>`+"\n", x+10, y+24+i*lineH, esc(l))
				}
				y += h + 16
			}
		}
		y += bandGap
	}

	if len(d.then) > 0 {
		band("Assert")
		for _, it := range d.then {
			switch v := it.(type) {
			case message:
				drawMessage(v)
			case check:
				if v.negated {
					drawPill(v.lane, "✗ "+v.text, "check negated")
				} else {
					drawPill(v.lane, "✓ "+v.text, "check")
				}
			}
		}
	}
	y += 10

	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" font-family="'Hiragino Sans','Noto Sans JP','Yu Gothic UI',system-ui,sans-serif">`+"\n", svgW, y)
	out.WriteString(svgStyle)
	out.WriteString(svgDefs)
	for i, name := range laneNames {
		x := laneX[i]
		fmt.Fprintf(&out, `<line class="life" x1="%d" y1="%d" x2="%d" y2="%d"/>`+"\n", x, 20+headerH, x, y-6)
		fmt.Fprintf(&out, `<rect class="head %s" x="%d" y="20" width="150" height="%d" rx="8"/>`+"\n", laneClass[i], x-75, headerH)
		fmt.Fprintf(&out, `<text class="headtext" x="%d" y="%d" text-anchor="middle">%s</text>`+"\n", x, 20+headerH/2+5, name)
	}
	out.WriteString(body.String())
	out.WriteString("</svg>\n")
	return out.String()
}

func markerFor(m message) string {
	switch {
	case m.negated:
		return "mn"
	case m.expect:
		return "me"
	}
	return "m"
}

func esc(s string) string { return html.EscapeString(s) }

// textWidth は、12px の等幅フォントで描いたときのおおよその幅を返す。
func textWidth(s string) int {
	w := 0.0
	for _, r := range s {
		if r < 0x80 {
			w += 7.3
		} else {
			w += 12
		}
	}
	return int(w + 0.5)
}

const svgStyle = `<style>
svg { --bg:#ffffff; --fg:#22252a; --muted:#5f6670; --line:#d9d6cf; --accent:#2f6fbd; --ok:#3f8a52; --ng:#c0606e;
  --client:#eef3fb; --client-s:#6b8fc7; --bridge:#eef7ef; --bridge-s:#5d9b68; --gp:#fbf1e6; --gp-s:#c58a45; --abs:#f3eefa; --abs-s:#8a6cc0; }
@media (prefers-color-scheme: dark) {
  svg { --bg:#1f2226; --fg:#e6e4df; --muted:#a2a7ae; --line:#3a3e44; --accent:#7fb0ee; --ok:#78b884; --ng:#e08a96;
    --client:#1d2633; --client-s:#7fa3dc; --bridge:#1c2a20; --bridge-s:#78b884; --gp:#2e2519; --gp-s:#d9a262; --abs:#261f33; --abs-s:#a98ddc; }
}
rect.bg { fill: var(--bg); }
text { fill: var(--fg); font-size: 13px; paint-order: stroke; stroke: var(--bg); stroke-width: 4px; stroke-linejoin: round; }
.mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12px; }
.detail { fill: var(--muted); }
.band { font-size: 11px; font-weight: 700; fill: var(--muted); }
.headtext { font-weight: 700; }
.sep { stroke: var(--line); stroke-width: 1; }
.life { stroke: var(--line); stroke-width: 1.2; stroke-dasharray: 3 4; }
.head { stroke-width: 1.5; }
.client { fill: var(--client); stroke: var(--client-s); } .bridge { fill: var(--bridge); stroke: var(--bridge-s); }
.gp { fill: var(--gp); stroke: var(--gp-s); } .abs { fill: var(--abs); stroke: var(--abs-s); }
.note { stroke-width: 1; }
.arrow { stroke: var(--muted); stroke-width: 1.5; }
.arrow.expect { stroke: var(--ok); stroke-width: 2; }
.arrow.negated { stroke: var(--ng); stroke-width: 2; stroke-dasharray: 6 4; }
.check { fill: var(--bg); stroke: var(--ok); stroke-width: 1.5; }
.step { fill: var(--bg); stroke: var(--muted); stroke-width: 1; }
.check.negated { stroke: var(--ng); stroke-dasharray: 5 3; }
.checktext { font-size: 12px; }
</style>
`

const svgDefs = `<defs>
<marker id="m" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" style="fill:var(--muted)"/></marker>
<marker id="me" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" style="fill:var(--ok)"/></marker>
<marker id="mn" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" style="fill:var(--ng)"/></marker>
</defs>
<rect class="bg" width="100%" height="100%"/>
`
