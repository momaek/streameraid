// Package svg draws laid out diagrams as SVG, in pure Go.
package svg

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	"github.com/momaek/streameraid/flow"
	"github.com/momaek/streameraid/geom"
	"github.com/momaek/streameraid/layout"
	"github.com/momaek/streameraid/render/css"
)

// Theme holds the colors a diagram is drawn in, as CSS colors.
type Theme struct {
	Background    string // "" leaves it transparent
	NodeFill      string
	NodeStroke    string
	Text          string
	TextOnLight   string // for nodes styled with a light fill
	TextOnDark    string // for nodes styled with a dark fill
	Edge          string
	EdgeLabelBg   string
	ClusterFill   string
	ClusterStroke string
	ClusterText   string
	NoteFill      string // notes of sequence diagrams
	NoteStroke    string
	NoteText      string
	FontFamily    string
	// Series are the colors of a mind map's branches and a pie's slices,
	// in order.
	Series []string
}

// SeriesLight and SeriesDark are the default series colors, for light and
// dark backgrounds: the same eight hues, stepped for each.
var (
	SeriesLight = []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#6250d6", "#e34948"}
	SeriesDark  = []string{"#3987e5", "#d95926", "#199e70", "#c98500", "#d55181", "#0ca30c", "#9085e9", "#e66767"}
)

// SeriesColor returns the color of series n, from 1.
func (th Theme) SeriesColor(n int) string {
	if len(th.Series) == 0 || n < 1 {
		return th.NodeStroke
	}
	return th.Series[(n-1)%len(th.Series)]
}

// background returns the color behind the diagram, for what is filled to
// hide the lines under it.
func (th Theme) background() string {
	if th.Background != "" {
		return th.Background
	}
	return th.EdgeLabelBg
}

// DefaultFonts is the font list SVGs ask for: the system's sans-serif
// fonts, with CJK ones.
const DefaultFonts = `-apple-system, BlinkMacSystemFont, "Segoe UI", "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif`

// Light is the theme for light backgrounds.
func Light() Theme {
	return Theme{
		NodeFill: "#f3f4ff", NodeStroke: "#7b83d8", Text: "#1f2328",
		TextOnLight: "#1f2328", TextOnDark: "#ffffff",
		Edge: "#5b6470", EdgeLabelBg: "#ffffff",
		ClusterFill: "#f7f8fa", ClusterStroke: "#d0d7de", ClusterText: "#57606a",
		NoteFill: "#fff8c5", NoteStroke: "#d4a72c", NoteText: "#1f2328",
		FontFamily: DefaultFonts, Series: SeriesLight,
	}
}

// Dark is the theme for dark backgrounds.
func Dark() Theme {
	return Theme{
		NodeFill: "#262a3f", NodeStroke: "#8d95f2", Text: "#e6edf3",
		TextOnLight: "#1f2328", TextOnDark: "#ffffff",
		Edge: "#9ba3ae", EdgeLabelBg: "#1b1f24",
		ClusterFill: "#161b22", ClusterStroke: "#3a414b", ClusterText: "#9ba3ae",
		NoteFill: "#3b3516", NoteStroke: "#9e8a2e", NoteText: "#e6edf3",
		FontFamily: DefaultFonts, Series: SeriesDark,
	}
}

// Flow draws a laid out flowchart. fontSize is the size the layout was
// made with.
func Flow(g *flow.Graph, f *layout.Flow, th Theme, fontSize float64) string {
	var b strings.Builder
	w, h := f.Width, f.Height
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s" font-size="%s">`,
		num(w), num(h), num(w), num(h), attr(th.FontFamily), num(fontSize))
	b.WriteByte('\n')
	if th.Background != "" {
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`+"\n", attr(th.Background))
	}
	clusterColors := func(sg *flow.Subgraph) (fill, stroke, text string, st flow.Style) {
		st = g.SubgraphStyle(sg)
		fill, stroke, text = pick(st.Fill, th.ClusterFill), pick(st.Stroke, th.ClusterStroke), th.ClusterText
		if st.Color != "" {
			text = st.Color
		} else if st.Fill != "" {
			text = textOn(st.Fill, th)
		}
		return
	}
	for _, c := range f.Clusters {
		fill, stroke, _, st := clusterColors(g.Subgraphs[c.ID])
		fmt.Fprintf(&b, `<rect class="cluster" data-id="%s" x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s" stroke="%s"%s/>`+"\n",
			attr(c.ID), num(c.Rect.X), num(c.Rect.Y), num(c.Rect.W), num(c.Rect.H), num(0.4*fontSize), attr(fill), attr(stroke), strokeExtras(st, 1))
	}
	for _, r := range f.Edges {
		e := g.Edges[r.Index]
		if e.Line == flow.Invisible {
			continue
		}
		color := pick(e.Style.Stroke, th.Edge)
		if e.Series > 0 && e.Style.Stroke == "" {
			color = th.SeriesColor(e.Series)
		}
		width := layout.EdgeWidth(e, fontSize)
		body, markers := layout.EdgeShapes(e, r, fontSize)
		dash := ""
		switch {
		case e.Style.Dash != "":
			dash = fmt.Sprintf(` stroke-dasharray="%s"`, attr(e.Style.Dash))
		case e.Line == flow.Dotted:
			dash = fmt.Sprintf(` stroke-dasharray="%s %s"`, num(width*2.5), num(width*2.5))
		}
		fmt.Fprintf(&b, `<g class="edge" data-id="%s"><path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round"%s/>`,
			attr(e.ID), pathData(body), attr(color), num(width), dash)
		for _, m := range markers {
			if m.Hollow {
				fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="%s" stroke-linejoin="round"/>`, pathData(m.Shape), attr(th.background()), attr(color), num(width))
			} else if m.Filled {
				fmt.Fprintf(&b, `<path d="%s" fill="%s"/>`, pathData(m.Shape), attr(color))
			} else {
				fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="%s" stroke-linecap="round"/>`, pathData(m.Shape), attr(color), num(width*1.2))
			}
		}
		if r.HasLabel {
			lb := geom.RectCentered(r.LabelAt, r.Label.W+0.5*fontSize, r.Label.H+0.2*fontSize)
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s" opacity="0.92"/>`,
				num(lb.X), num(lb.Y), num(lb.W), num(lb.H), num(0.25*fontSize), attr(th.EdgeLabelBg))
			writeText(&b, r.Label, r.LabelAt, pick(e.Style.Color, th.Text), "")
		}
		writeText(&b, r.StartLabel, r.StartAt, pick(e.Style.Color, th.Text), "")
		writeText(&b, r.EndLabel, r.EndAt, pick(e.Style.Color, th.Text), "")
		b.WriteString("</g>\n")
	}
	// Titles go over the edges, on their box's fill, so that edges entering
	// a box do not cross them out.
	for _, c := range f.Clusters {
		if len(c.Title.Lines) == 0 {
			continue
		}
		fill, _, text, _ := clusterColors(g.Subgraphs[c.ID])
		bg := geom.RectCentered(c.TitleAt, c.Title.W+0.4*fontSize, c.Title.H)
		fmt.Fprintf(&b, `<g class="cluster-title" data-id="%s"><rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s"/>`,
			attr(c.ID), num(bg.X), num(bg.Y), num(bg.W), num(bg.H), num(0.2*fontSize), attr(fill))
		writeText(&b, c.Title, c.TitleAt, text, "")
		b.WriteString("</g>\n")
	}
	for _, nb := range f.Nodes {
		n := g.Nodes[nb.ID]
		st := g.NodeStyle(n)
		fill, stroke, text := nodeColors(n, st, th)
		sw := 0.09 * fontSize
		fmt.Fprintf(&b, `<g class="node" data-id="%s"><path d="%s" fill="%s" stroke="%s"%s/>`,
			attr(nb.ID), pathData(nb.Outline), attr(fill), attr(stroke), strokeExtras(st, sw))
		if len(nb.Lines.Ops) > 0 {
			if layout.FilledDecorations(nb.Shape) {
				fmt.Fprintf(&b, `<path d="%s" fill="%s"/>`, pathData(nb.Lines), attr(stroke))
			} else {
				fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s"%s/>`, pathData(nb.Lines), attr(stroke), strokeExtras(st, sw))
			}
		}
		weight := ""
		if st.FontWeight != "" {
			weight = fmt.Sprintf(` font-weight="%s"`, attr(st.FontWeight))
		}
		writeText(&b, nb.Label, layout.LabelCenter(nb.Shape, nb.Rect), text, weight)
		if nb.Table != nil {
			for _, c := range nb.Table.Cells {
				extra := weight
				if c.Text.Style.Bold {
					extra = ` font-weight="bold"`
				}
				if c.Text.Style.Size != fontSize {
					extra += fmt.Sprintf(` font-size="%s"`, num(c.Text.Style.Size))
				}
				if c.Text.Style.Italic {
					extra += ` font-style="italic"`
				}
				if c.Text.Style.Underline {
					extra += ` text-decoration="underline"`
				}
				at := nb.Table.CellAt(c, nb.Rect)
				if c.Left {
					writeTextFrom(&b, c.Text, geom.Point{X: at.X - c.Text.W/2, Y: at.Y}, text, extra)
					continue
				}
				writeText(&b, c.Text, at, text, extra)
			}
		}
		b.WriteString("</g>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

// nodeColors returns the fill, stroke and text colors of a node: those
// its style sets, else those of its shape or series, else the theme's.
func nodeColors(n *flow.Node, st flow.Style, th Theme) (fill, stroke, text string) {
	fill, stroke, text = th.NodeFill, th.NodeStroke, th.Text
	switch {
	case layout.FilledShape(n.Shape):
		fill, stroke = th.Edge, th.Edge
	case n.Shape == flow.End:
		fill, stroke = th.background(), th.Edge
	case n.Shape == flow.Note:
		fill, stroke, text = th.NoteFill, th.NoteStroke, th.NoteText
	case n.Series > 0:
		c := th.SeriesColor(n.Series)
		fill, stroke = css.Mix(th.background(), c, 0.22), c
	}
	fill, stroke = pick(st.Fill, fill), pick(st.Stroke, stroke)
	if st.Color != "" {
		text = st.Color
	} else if st.Fill != "" {
		text = textOn(st.Fill, th)
	}
	return
}

func strokeExtras(st flow.Style, width float64) string {
	s := ""
	if st.StrokeWidth > 0 {
		width = st.StrokeWidth
	}
	s += fmt.Sprintf(` stroke-width="%s"`, num(width))
	if st.Dash != "" {
		s += fmt.Sprintf(` stroke-dasharray="%s"`, attr(st.Dash))
	}
	return s
}

// writeText writes a text's lines centered on c.
func writeText(b *strings.Builder, tb layout.TextBox, c geom.Point, color, extra string) {
	if len(tb.Lines) == 0 {
		return
	}
	top := c.Y - tb.H/2
	fmt.Fprintf(b, `<text fill="%s" text-anchor="middle"%s>`, attr(color), extra)
	for i, line := range tb.Lines {
		// The baseline sits about 0.35 em below the middle of the line.
		y := top + (float64(i)+0.5)*tb.LineHeight + 0.35*tb.Style.Size
		fmt.Fprintf(b, `<tspan x="%s" y="%s">%s</tspan>`, num(c.X), num(y), html.EscapeString(line))
	}
	b.WriteString("</text>")
}

// writeTextFrom writes a text's lines aligned left on left.X, centered
// on its Y.
func writeTextFrom(b *strings.Builder, tb layout.TextBox, left geom.Point, color, extra string) {
	if len(tb.Lines) == 0 {
		return
	}
	top := left.Y - tb.H/2
	fmt.Fprintf(b, `<text fill="%s"%s>`, attr(color), extra)
	for i, line := range tb.Lines {
		y := top + (float64(i)+0.5)*tb.LineHeight + 0.35*tb.Style.Size
		fmt.Fprintf(b, `<tspan x="%s" y="%s">%s</tspan>`, num(left.X), num(y), html.EscapeString(line))
	}
	b.WriteString("</text>")
}

// textOn picks a text color readable on a fill.
func textOn(fill string, th Theme) string {
	c, ok := css.ParseColor(fill)
	if !ok || c.A < 128 {
		return th.Text
	}
	if c.Luminance() > 0.4 {
		return th.TextOnLight
	}
	return th.TextOnDark
}

func pick(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// attr escapes s for an attribute value.
func attr(s string) string { return html.EscapeString(s) }

func num(f float64) string {
	if math.Abs(f) < 1e-9 {
		return "0"
	}
	// Snap to a millionth first: a value on the boundary between two
	// hundredths, as 216.265, comes out a hair above it on one CPU and a
	// hair below on another (arm64 fuses multiplies and adds, amd64 does
	// not), and would round to different hundredths.
	snapped := math.Round(f*1e6) / 1e4
	return strconv.FormatFloat(math.Round(snapped)/100, 'f', -1, 64)
}

func pathData(p geom.Path) string {
	var b strings.Builder
	pt := func(q geom.Point) {
		b.WriteString(num(q.X))
		b.WriteByte(',')
		b.WriteString(num(q.Y))
	}
	for _, op := range p.Ops {
		switch op.Kind {
		case geom.MoveTo:
			b.WriteByte('M')
			pt(op.Pts[0])
		case geom.LineTo:
			b.WriteByte('L')
			pt(op.Pts[0])
		case geom.CubicTo:
			b.WriteByte('C')
			pt(op.Pts[0])
			b.WriteByte(' ')
			pt(op.Pts[1])
			b.WriteByte(' ')
			pt(op.Pts[2])
		case geom.Close:
			b.WriteByte('Z')
		}
	}
	return b.String()
}

// Pie draws a laid out pie chart. fontSize is the size the layout was made
// with.
func Pie(lay *layout.Pie, th Theme, fontSize float64) string {
	var b strings.Builder
	w, h := lay.Width, lay.Height
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s" font-size="%s">`,
		num(w), num(h), num(w), num(h), attr(th.FontFamily), num(fontSize))
	b.WriteByte('\n')
	if th.Background != "" {
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s"/>`+"\n", attr(th.Background))
	}
	writeText(&b, lay.Title, lay.TitleAt, th.Text, ` font-weight="bold"`)
	if len(lay.Title.Lines) > 0 {
		b.WriteByte('\n')
	}
	empty := true
	for _, s := range lay.Slices {
		if s.End > s.Start {
			empty = false
		}
	}
	if empty {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="%s"/>`+"\n",
			num(lay.Center.X), num(lay.Center.Y), num(lay.Radius), attr(th.ClusterStroke))
	}
	for _, s := range lay.Slices {
		color := th.SeriesColor(s.Series)
		fmt.Fprintf(&b, `<g class="slice" data-id="%s">`, attr(s.ID))
		if s.End > s.Start {
			// A gap of the background between slices.
			fmt.Fprintf(&b, `<path d="%s" fill="%s" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`,
				pathData(layout.Wedge(lay.Center, lay.Radius, s.Start, s.End)), attr(color), attr(th.background()))
		}
		fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" fill="%s"/>`,
			num(s.Swatch.X), num(s.Swatch.Y), num(s.Swatch.W), num(s.Swatch.H), num(0.2*fontSize), attr(color))
		writeText(&b, s.Label, s.LabelAt, th.Text, "")
		writeText(&b, s.Share, s.ShareAt, th.ClusterText, "")
		b.WriteString("</g>\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}
