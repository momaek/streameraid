package mermaid

import (
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/measure"
	"github.com/momaek/streameraid/render/css"
	"github.com/momaek/streameraid/render/svg"
)

// palette is the colors a diagram is drawn in, taken from the theme: nodes
// in a wash of the accent color, so that diagrams follow the theme and its
// light or dark mode.
type palette struct {
	bg, nodeFill, nodeStroke, text, textOnLight, textOnDark ui.Color
	edge, labelBg, clusterFill, clusterStroke, clusterText  ui.Color
	noteFill, noteStroke, noteText                          ui.Color // notes of sequence diagrams
	accent                                                  ui.Color // what stands out under the pointer
	series                                                  []ui.Color
}

// seriesColor returns the color of series n, from 1.
func (pal palette) seriesColor(n int) ui.Color {
	if n < 1 || len(pal.series) == 0 {
		return pal.nodeStroke
	}
	return pal.series[(n-1)%len(pal.series)]
}

func newPalette(th *ui.Theme) palette {
	bg := th.Background
	if bg.A == 0 {
		bg = ui.RGB(255, 255, 255)
		if th.Dark {
			bg = ui.RGB(0x1b, 0x1f, 0x24)
		}
	}
	acc := th.Accent
	if acc.A == 0 {
		acc = ui.RGB(0x63, 0x66, 0xf1)
	}
	// Notes are the yellow of sticky notes, dimmed in the dark.
	noteFill, noteStroke := ui.RGB(0xff, 0xf8, 0xc5), ui.RGB(0xd4, 0xa7, 0x2c)
	if th.Dark {
		noteFill, noteStroke = ui.RGB(0x3b, 0x35, 0x16), ui.RGB(0x9e, 0x8a, 0x2e)
	}
	series := svg.SeriesLight
	if th.Dark {
		series = svg.SeriesDark
	}
	var seriesColors []ui.Color
	for _, h := range series {
		c, _ := css.ParseColor(h)
		seriesColors = append(seriesColors, ui.RGB(c.R, c.G, c.B))
	}
	return palette{
		series:   seriesColors,
		noteFill: noteFill, noteStroke: noteStroke, noteText: th.Text, accent: acc,
		bg:          bg,
		nodeFill:    mix(bg, acc, 0.10),
		nodeStroke:  mix(bg, acc, 0.62),
		text:        th.Text,
		textOnLight: ui.RGB(0x1f, 0x23, 0x28),
		textOnDark:  ui.RGB(255, 255, 255),
		edge:        mix(bg, th.Text, 0.62),
		labelBg:     bg,
		clusterFill: mix(bg, th.Text, 0.035),
		// Borders are drawn a little stronger than the theme's own, to read
		// as edges of boxes rather than of controls.
		clusterStroke: mix(bg, th.Text, 0.2),
		clusterText:   th.TextMuted,
	}
}

func mix(a, b ui.Color, t float32) ui.Color {
	f := func(x, y uint8) uint8 { return uint8(float32(x)*(1-t) + float32(y)*t + 0.5) }
	return ui.RGB(f(a.R, b.R), f(a.G, b.G), f(a.B, b.B))
}

// styled returns the CSS color s, or def when s is empty or unreadable.
func (pal palette) styled(s string, def ui.Color) ui.Color {
	if c, ok := css.ParseColor(s); ok {
		return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255)
	}
	return def
}

// textOn picks a text color readable on a fill a style set.
func (pal palette) textOn(fill string) ui.Color {
	c, ok := css.ParseColor(fill)
	if !ok || c.A < 128 {
		return pal.text
	}
	if c.Luminance() > 0.4 {
		return pal.textOnLight
	}
	return pal.textOnDark
}

func (pal palette) node(g *flow.Graph, n *flow.Node) (fill, stroke, text ui.Color) {
	st := g.NodeStyle(n)
	fill, stroke, text = pal.nodeFill, pal.nodeStroke, pal.text
	switch {
	case layout.FilledShape(n.Shape):
		fill, stroke = pal.edge, pal.edge
	case n.Shape == flow.End:
		fill, stroke = pal.bg, pal.edge
	case n.Shape == flow.Note:
		fill, stroke, text = pal.noteFill, pal.noteStroke, pal.noteText
	case n.Series > 0:
		c := pal.seriesColor(n.Series)
		fill, stroke = mix(pal.bg, c, 0.22), c
	}
	fill, stroke = pal.styled(st.Fill, fill), pal.styled(st.Stroke, stroke)
	if st.Color != "" {
		text = pal.styled(st.Color, text)
	} else if st.Fill != "" {
		text = pal.textOn(st.Fill)
	}
	return
}

func (pal palette) cluster(g *flow.Graph, s *flow.Subgraph) (fill, stroke, text ui.Color) {
	st := g.SubgraphStyle(s)
	fill, stroke, text = pal.styled(st.Fill, pal.clusterFill), pal.styled(st.Stroke, pal.clusterStroke), pal.clusterText
	if st.Color != "" {
		text = pal.styled(st.Color, text)
	} else if st.Fill != "" {
		text = pal.textOn(st.Fill)
	}
	return
}

// toPath converts a path, moved by (dx, dy).
func toPath(p geom.Path, dx, dy float32) *ui.Path {
	out := &ui.Path{}
	pt := func(q geom.Point) (float32, float32) { return float32(q.X) + dx, float32(q.Y) + dy }
	for _, op := range p.Ops {
		switch op.Kind {
		case geom.MoveTo:
			out.MoveTo(pt(op.Pts[0]))
		case geom.LineTo:
			out.LineTo(pt(op.Pts[0]))
		case geom.CubicTo:
			x1, y1 := pt(op.Pts[0])
			x2, y2 := pt(op.Pts[1])
			x3, y3 := pt(op.Pts[2])
			out.CubeTo(x1, y1, x2, y2, x3, y3)
		case geom.Close:
			out.Close()
		}
	}
	return out
}

func toRect(r geom.Rect, dx, dy float32) ui.Rect {
	return ui.Rect{X: float32(r.X) + dx, Y: float32(r.Y) + dy, W: float32(r.W), H: float32(r.H)}
}

// fade returns c at alpha times its opacity.
func fade(c ui.Color, alpha float64) ui.Color {
	if alpha >= 1 {
		return c
	}
	return ui.RGBA(c.R, c.G, c.B, float32(c.A)/255*float32(max(alpha, 0)))
}

// paint draws everything but the labels, as frame f shows them: subgraphs'
// boxes, edges, the backgrounds of edge labels and subgraph titles, and
// nodes' shapes.
//
// hover is the node under the pointer, or "": its edges are drawn in the
// accent color and the others faded, to show what it connects to.
func paint(p *ui.Painter, r ui.Rect, g *flow.Graph, lay *layout.Flow, f *frame, pal palette, fs float64, hover string) {
	dx, dy := r.X, r.Y
	rad := float32(0.4 * fs)
	for _, cb := range lay.Clusters {
		cf := f.clusters[cb.ID]
		sg := g.Subgraphs[cb.ID]
		fill, stroke, _ := pal.cluster(g, sg)
		box := toRect(cf.rect, dx, dy)
		p.Fill(box, fade(fill, cf.alpha), rad)
		if g.SubgraphStyle(sg).Dash != "" {
			// A dashed box: a region of a state, or a style asking for it.
			var outline geom.Path
			outline.RoundRect(cf.rect, float64(rad))
			path := toPath(outline, dx, dy)
			path.Dash(0, 4, 4)
			p.StrokePath(path, 1, fade(stroke, cf.alpha))
		} else {
			p.Stroke(box, fade(stroke, cf.alpha), rad, 1)
		}
	}
	for _, er := range lay.Edges {
		e := g.Edges[er.Index]
		if e.Line == flow.Invisible {
			continue
		}
		ef := f.edges[er.ID]
		color := pal.edgeColor(e)
		w := float32(layout.EdgeWidth(e, fs))
		if hover != "" {
			if e.From == hover || e.To == hover {
				color, w = pal.accent, w*1.4
			} else {
				color = fade(color, 0.3)
			}
		}
		if ef.moving {
			// Between two routes: through the points on the way.
			er.Path = polyline(ef.pts)
		}
		body, markers := layout.EdgeShapes(e, er, fs)
		path := toPath(body, dx, dy)
		if ef.drawn < 1 {
			path = path.Trim(0, float32(ef.drawn))
		}
		if e.Line == flow.Dotted {
			path.Dash(0, w*2.5, w*2.5)
		}
		p.StrokePath(path, w, color)
		for _, m := range markers {
			// A marker at the target comes as the line reaches it; one at
			// the source is there from the start.
			c := color
			if m.AtEnd {
				c = fade(color, (ef.drawn-0.8)/0.2)
			}
			switch {
			case m.Hollow:
				shape := toPath(m.Shape, dx, dy)
				if m.Filled {
					p.FillPath(shape, c)
				} else {
					p.FillPath(shape, fade(pal.bg, (ef.drawn-0.8)/0.2))
				}
				p.StrokePath(shape, w, c)
			case m.Filled:
				p.FillPath(toPath(m.Shape, dx, dy), c)
			default:
				p.StrokePath(toPath(m.Shape, dx, dy), w*1.2, c)
			}
		}
		if er.HasLabel {
			lb := geom.RectCentered(ef.labelAt, er.Label.W+0.5*fs, er.Label.H+0.2*fs)
			p.Fill(toRect(lb, dx, dy), fade(pal.labelBg, ef.drawn), float32(0.25*fs))
		}
	}
	for _, cb := range lay.Clusters {
		if len(cb.Title.Lines) == 0 {
			continue
		}
		cf := f.clusters[cb.ID]
		fill, _, _ := pal.cluster(g, g.Subgraphs[cb.ID])
		bg := geom.RectCentered(cf.titleAt, cb.Title.W+0.4*fs, cb.Title.H)
		p.Fill(toRect(bg, dx, dy), fade(fill, cf.alpha), float32(0.2*fs))
	}
	for _, nb := range lay.Nodes {
		nf := f.nodes[nb.ID]
		n := g.Nodes[nb.ID]
		fill, stroke, _ := pal.node(g, n)
		st := g.NodeStyle(n)
		w := float32(0.09 * fs)
		if st.StrokeWidth > 0 {
			w = float32(st.StrokeWidth)
		}
		shape, lines := nb.Outline, nb.Lines
		if nf.rect != nb.Rect || nf.alpha < 1 {
			// On the way: a new node grows a little as it fades in.
			rect := grow(nf.rect, 0.92+0.08*nf.alpha)
			shape = layout.Outline(nb.Shape, rect)
			lines = layout.NodeLines(nb, rect, fs)
		}
		if nb.ID == hover {
			stroke, w = pal.accent, w*1.6
		}
		outline := toPath(shape, dx, dy)
		p.FillPath(outline, fade(fill, nf.alpha))
		p.StrokePath(outline, w, fade(stroke, nf.alpha))
		if len(lines.Ops) > 0 {
			if layout.FilledDecorations(nb.Shape) {
				p.FillPath(toPath(lines, dx, dy), fade(stroke, nf.alpha))
			} else {
				p.StrokePath(toPath(lines, dx, dy), w, fade(stroke, nf.alpha))
			}
		}
	}
}

// edgeColor returns the color of an edge's line: its style's, its
// series', or the theme's.
func (pal palette) edgeColor(e *flow.Edge) ui.Color {
	if e.Series > 0 && e.Style.Stroke == "" {
		return pal.seriesColor(e.Series)
	}
	return pal.styled(e.Style.Stroke, pal.edge)
}

// labelText places a label's lines centered on c, as text elements, at
// alpha times their opacity. z is the diagram's zoom: c and the label's
// size are those of the layout, before zooming.
func labelText(c *ui.Context, tb layout.TextBox, center geom.Point, color ui.Color, alpha, z float64) {
	if len(tb.Lines) == 0 {
		return
	}
	k := float32(z)
	size := float32(tb.Style.Size)
	weight := 400
	if tb.Style.Bold {
		weight = 700
	}
	m := ui.Font{Family: tb.Style.Family, Size: size * k, Weight: weight}.Metrics()
	shift := (float32(tb.LineHeight)*k - (m.Ascent + m.Descent + m.LineGap)) / 2
	top := float32(center.Y-tb.H/2) * k
	for i, line := range tb.Lines {
		w := float32(sharedMeasurer.lineWidth(line, tb.Style)) * k
		t := ui.Text(c, line).Absolute().
			Left(float32(center.X)*k - w/2).
			Top(top + float32(i)*float32(tb.LineHeight)*k + shift).
			FontSize(size * k).FontWeight(weight).TextColor(color).NoWrap()
		if tb.Style.Italic {
			t.Italic()
		}
		if tb.Style.Underline {
			t.Underline()
		}
		if alpha < 1 {
			t.Opacity(float32(max(alpha, 0)))
		}
	}
}

// uiMeasurer measures text with the fonts MyGo draws it with, a character
// at a time, so that what it measures can be cached.
type uiMeasurer struct {
	mu     sync.Mutex
	widths map[runeKey]float64
}

type runeKey struct {
	r      rune
	family string
	size   float64
	bold   bool
}

var sharedMeasurer = &uiMeasurer{widths: map[runeKey]float64{}}

func (m *uiMeasurer) width(r rune, st layout.TextStyle) float64 {
	k := runeKey{r, st.Family, st.Size, st.Bold}
	m.mu.Lock()
	w, ok := m.widths[k]
	m.mu.Unlock()
	if ok {
		return w
	}
	weight := 400
	if st.Bold {
		weight = 700
	}
	for _, gl := range ui.Shape(string(r), ui.Font{Family: st.Family, Size: float32(st.Size), Weight: weight}) {
		w = max(w, float64(gl.X+gl.Advance))
	}
	m.mu.Lock()
	m.widths[k] = w
	m.mu.Unlock()
	return w
}

// Measure measures text as layout.Measurer does.
func (m *uiMeasurer) Measure(text string, st layout.TextStyle, maxWidth float64) layout.TextBox {
	return measure.Wrap(text, st, maxWidth, func(r rune) float64 { return m.width(r, st) })
}

func (m *uiMeasurer) lineWidth(line string, st layout.TextStyle) float64 {
	w := 0.0
	for _, r := range line {
		w += m.width(r, st)
	}
	return w
}
