package mermaid

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/layout/sequence"
)

// The parts of a sequence diagram in a frame: boxes (heads, notes,
// activations, blocks, groups) are under nodes, lines (lifelines,
// messages) under edges, each kind with its own prefix.

// frameOfSequence returns the frame showing all of a sequence diagram's
// layout.
func frameOfSequence(lay *layout.Sequence) *frame {
	f := &frame{w: lay.Width, h: lay.Height, nodes: map[string]nodeFrame{}, clusters: map[string]clusterFrame{}, edges: map[string]edgeFrame{}}
	box := func(key string, r geom.Rect) { f.nodes[key] = nodeFrame{rect: r, alpha: 1} }
	for _, g := range lay.Groups {
		box("g:"+g.ID, g.Rect)
	}
	for _, p := range lay.Participants {
		box("p:"+p.ID, p.Rect)
		var life geom.Path
		life.MoveTo(p.LifeTop).LineTo(p.LifeBottom)
		f.edges["l:"+p.ID] = edgeFrame{pts: sample(life, 2), drawn: 1}
	}
	for _, b := range lay.Blocks {
		if b.ID != "" {
			box("b:"+b.ID, b.Rect)
		}
	}
	for _, a := range lay.Activations {
		box("a:"+a.ID, a.Rect)
	}
	for _, n := range lay.Notes {
		box("n:"+n.ID, n.Rect)
	}
	for _, m := range lay.Messages {
		f.edges["m:"+m.ID] = edgeFrame{pts: sample(m.Path, edgeSamples), labelAt: m.LabelAt, drawn: 1}
	}
	return f
}

// offset returns where a point placed relative to a box is, now that the
// box is at now rather than at was.
func offset(pt geom.Point, was, now geom.Rect) geom.Point {
	return geom.Point{X: pt.X - was.X + now.X, Y: pt.Y - was.Y + now.Y}
}

// moved returns how far the end of a line (its start when start is set) is
// from where it will be.
func moved(f, to edgeFrame, start bool) (dx, dy float32) {
	if len(f.pts) == 0 || len(to.pts) != len(f.pts) {
		return 0, 0
	}
	i := len(f.pts) - 1
	if start {
		i = 0
	}
	return float32(f.pts[i].X - to.pts[i].X), float32(f.pts[i].Y - to.pts[i].Y)
}

// paintSequence draws a sequence diagram as frame f shows it, without its
// labels. to is the frame the diagram is on its way to.
func paintSequence(p *ui.Painter, r ui.Rect, lay *layout.Sequence, f, to *frame, pal palette, fs float64) {
	dx, dy := r.X, r.Y
	em := float32(fs)
	line := 0.09 * em
	for _, g := range lay.Groups {
		gf := f.nodes["g:"+g.ID]
		fill := pal.styled(g.Color, pal.clusterFill)
		box := toRect(gf.rect, dx, dy)
		p.Fill(box, fade(fill, gf.alpha), 0.4*em)
		p.Stroke(box, fade(pal.clusterStroke, gf.alpha), 0.4*em, 1)
	}
	for _, b := range lay.Blocks {
		if b.Kind == "rect" && b.ID != "" {
			bf := f.nodes["b:"+b.ID]
			p.Fill(toRect(bf.rect, dx, dy), fade(pal.styled(b.Fill, pal.clusterFill), bf.alpha), 0.3*em)
		}
	}
	for _, pt := range lay.Participants {
		lf := f.edges["l:"+pt.ID]
		if len(lf.pts) < 2 {
			continue
		}
		a, b := lf.pts[0], lf.pts[len(lf.pts)-1]
		b = a.Lerp(b, lf.drawn)
		p.Line(float32(a.X)+dx, float32(a.Y)+dy, float32(b.X)+dx, float32(b.Y)+dy, line, pal.clusterStroke)
		if pt.Destroyed {
			c, d := float32(b.X)+dx, float32(b.Y)+dy
			q := 0.45 * em
			cross := (&ui.Path{}).MoveTo(c-q, d-q).LineTo(c+q, d+q).MoveTo(c-q, d+q).LineTo(c+q, d-q)
			p.StrokePath(cross, 2*line, fade(pal.edge, lf.drawn))
		}
	}
	for _, a := range lay.Activations {
		af := f.nodes["a:"+a.ID]
		box := toRect(af.rect, dx, dy)
		p.Fill(box, fade(pal.nodeFill, af.alpha), 0.1*em)
		p.Stroke(box, fade(pal.nodeStroke, af.alpha), 0.1*em, line)
	}
	for _, b := range lay.Blocks {
		if b.Kind == "rect" || b.ID == "" {
			continue
		}
		bf := f.nodes["b:"+b.ID]
		edge := fade(pal.edge, bf.alpha)
		p.Stroke(toRect(bf.rect, dx, dy), edge, 0.3*em, line)
		// The tab keeps its size at the corner of the frame.
		x0, y0 := float32(bf.rect.X)+dx, float32(bf.rect.Y)+dy
		w, h, cut := float32(b.Tab.W), float32(b.Tab.H), 0.45*em
		tab := (&ui.Path{}).MoveTo(x0, y0).LineTo(x0+w, y0).LineTo(x0+w, y0+h-cut).LineTo(x0+w-cut, y0+h).LineTo(x0, y0+h).Close()
		p.FillPath(tab, fade(pal.clusterFill, bf.alpha))
		p.StrokePath(tab, line, edge)
		for _, s := range b.Sections {
			y := float32(s.Y-b.Rect.Y) + y0
			if y > y0+float32(bf.rect.H) {
				continue // the frame has not grown that far yet
			}
			div := (&ui.Path{}).MoveTo(x0, y).LineTo(x0+float32(bf.rect.W), y)
			p.StrokePath(div.Dash(0, 0.3*em, 0.3*em), line, edge)
		}
	}
	for _, m := range lay.Messages {
		key := "m:" + m.ID
		mf := f.edges[key]
		body := m.Path
		if mf.moving {
			body = polyline(mf.pts)
		}
		path := toPath(body, dx, dy)
		if mf.drawn < 1 {
			path = path.Trim(0, float32(mf.drawn))
		}
		if m.Dotted {
			path.Dash(0, 0.3*em, 0.3*em)
		}
		p.StrokePath(path, 0.11*em, pal.edge)
		for i, h := range m.Heads {
			// The head at the end comes as the line reaches it, and both
			// follow the line while it moves.
			ex, ey := moved(mf, to.edges[key], i > 0)
			c := pal.edge
			if i == 0 {
				c = fade(c, (mf.drawn-0.8)/0.2)
			}
			shape := toPath(h.Shape, dx+ex, dy+ey)
			if h.Filled {
				p.FillPath(shape, c)
			} else {
				p.StrokePath(shape, 0.11*em, c)
			}
		}
		if len(m.Label.Lines) > 0 {
			bg := geom.RectCentered(mf.labelAt, m.Label.W+0.4*fs, m.Label.H)
			p.Fill(toRect(bg, dx, dy), fade(pal.labelBg, 0.85*mf.drawn), 0.2*em)
		}
		if m.Number != 0 {
			sx, sy := moved(mf, to.edges[key], true)
			badge := (&ui.Path{}).Circle(float32(m.NumberAt.X)+dx+sx, float32(m.NumberAt.Y)+dy+sy, float32(m.NumberRadius))
			p.FillPath(badge, fade(pal.edge, mf.drawn))
		}
	}
	for _, n := range lay.Notes {
		nf := f.nodes["n:"+n.ID]
		box := toRect(grow(nf.rect, 0.94+0.06*nf.alpha), dx, dy)
		p.Fill(box, fade(pal.noteFill, nf.alpha), 0.2*em)
		p.Stroke(box, fade(pal.noteStroke, nf.alpha), 0.2*em, line)
	}
	for _, pt := range lay.Participants {
		pf := f.nodes["p:"+pt.ID]
		if pt.Actor {
			fig := toPath(sequence.Actor(geom.Point{X: pf.rect.X + pf.rect.W/2, Y: pf.rect.Y}, fs), dx, dy)
			p.FillPath(fig, fade(pal.nodeFill, pf.alpha))
			p.StrokePath(fig, 0.12*em, fade(pal.nodeStroke, pf.alpha))
			continue
		}
		box := toRect(grow(pf.rect, 0.94+0.06*pf.alpha), dx, dy)
		p.Fill(box, fade(pal.nodeFill, pf.alpha), 0.25*em)
		p.Stroke(box, fade(pal.nodeStroke, pf.alpha), 0.25*em, line)
	}
}

// sequenceLabels places the texts of a sequence diagram as frame f shows
// it.
func sequenceLabels(c *ui.Context, lay *layout.Sequence, f *frame, pal palette, fs, z float64) {
	labelText := func(c *ui.Context, tb layout.TextBox, at geom.Point, color ui.Color, alpha float64) {
		labelText(c, tb, at, color, alpha, z)
	}
	for _, g := range lay.Groups {
		gf := f.nodes["g:"+g.ID]
		text := pal.clusterText
		if g.Color != "" {
			text = pal.textOn(g.Color)
		}
		labelText(c, g.Title, offset(g.TitleAt, g.Rect, gf.rect), text, gf.alpha)
	}
	for _, b := range lay.Blocks {
		if b.Kind == "rect" || b.ID == "" {
			continue
		}
		bf := f.nodes["b:"+b.ID]
		labelText(c, b.KindLabel, offset(b.KindAt, b.Rect, bf.rect), pal.text, bf.alpha)
		labelText(c, b.Title, offset(b.TitleAt, b.Rect, bf.rect), pal.text, bf.alpha)
		for _, s := range b.Sections {
			at := offset(s.TitleAt, b.Rect, bf.rect)
			if at.Y+s.Title.H/2 > bf.rect.Bottom() {
				continue // the frame has not grown that far yet
			}
			labelText(c, s.Title, at, pal.text, bf.alpha)
		}
	}
	for _, m := range lay.Messages {
		mf := f.edges["m:"+m.ID]
		labelText(c, m.Label, mf.labelAt, pal.text, mf.drawn)
		if m.Number != 0 {
			st := layout.TextStyle{Size: 0.78 * fs, Bold: true}
			tb := sharedMeasurer.Measure(strconv.Itoa(m.Number), st, 0)
			at := m.NumberAt
			if len(mf.pts) > 0 && mf.moving {
				at = geom.Point{X: mf.pts[0].X, Y: mf.pts[0].Y}
			}
			labelText(c, tb, at, pal.labelBg, mf.drawn)
		}
	}
	for _, n := range lay.Notes {
		nf := f.nodes["n:"+n.ID]
		labelText(c, n.Label, nf.rect.Center(), pal.noteText, nf.alpha)
	}
	for _, p := range lay.Participants {
		pf := f.nodes["p:"+p.ID]
		at := pf.rect.Center()
		if p.Actor {
			at.Y = pf.rect.Bottom() - p.Label.H/2
		}
		labelText(c, p.Label, at, pal.text, pf.alpha)
	}
	if len(lay.Title.Lines) > 0 {
		labelText(c, lay.Title, lay.TitleAt, pal.text, 1)
	}
}
